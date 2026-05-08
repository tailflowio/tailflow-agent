package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/tailflow/tailflow/internal/action"
	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/event"
	tfotel "github.com/tailflow/tailflow/internal/otel"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/runtime"
)

func runCmd(noColorFlag *bool, otelEndpoint, otelServiceName *string) *cobra.Command {
	var (
		params []string
		data   string
	)

	cmd := &cobra.Command{
		Use:   "run <workflow.yaml>",
		Short: "Execute a workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			noColor := detectNoColor(*noColorFlag)
			otelCfg := resolveOTelConfig(otelEndpoint, otelServiceName)

			return executeRun(args[0], params, data, noColor, otelCfg)
		},
	}
	cmd.Flags().StringArrayVarP(&params, "param", "p", nil, "Parameters (key=value)")
	cmd.Flags().StringVarP(&data, "data", "d", "", "Trigger body as JSON (for trigger-based workflows)")

	return cmd
}

type runResources struct {
	bus      *event.Bus
	tickDone chan struct{}
	wg       sync.WaitGroup
}

func (rr *runResources) shutdown() {
	rr.bus.Close()
	rr.wg.Wait()

	if rr.tickDone != nil {
		close(rr.tickDone)
	}
}

func executeRun(path string, rawParams []string, data string, noColor bool, otelCfg tfotel.Config) error {
	otelCfg.Sync = true

	otelResult, err := tfotel.Setup(context.Background(), otelCfg)
	if err != nil {
		return fmt.Errorf("otel setup: %w", err)
	}
	defer shutdownOTel(otelResult)

	wf, parseErr := parser.Parse(path)
	if parseErr != nil {
		return parseErr
	}

	renderer := &cliRenderer{
		noColor: noColor,
		isTTY:   isTerminal(),
		wfName:  wf.Name,
	}

	buildErr := renderer.buildTree(wf)
	if buildErr != nil {
		return buildErr
	}

	bus := event.NewBus()
	defer bus.Close()

	tracer := tfotel.NewTracer(otelResult.TracerProvider)

	bm, bmErr := tfotel.NewBusinessMetrics(otelResult.MeterProvider)
	if bmErr != nil {
		return fmt.Errorf("otel business metrics: %w", bmErr)
	}

	logger := tfotel.NewSlogLogger(slog.LevelError+1, otelResult)

	exec, services, closeFn, setupErr := setupActionRegistry(wf, bus, logger, tracer, bm)
	if setupErr != nil {
		return setupErr
	}

	defer closeFn()

	res := setupRunResources(bus, renderer)
	opts := buildExecutionOptions(renderer, wf, path, data, services)

	renderer.printTree()

	return runAndReport(renderer, exec, wf, parseParams(rawParams), opts, res)
}

func setupActionRegistry(
	wf *parser.Workflow, bus *event.Bus, logger *slog.Logger,
	tracer *tfotel.Tracer, bm *tfotel.BusinessMetrics,
) (*engine.Executor, *runtime.ActionServices, func(), error) {
	reg := action.NewRegistry()
	action.RegisterBuiltins(reg)

	dbPool := runtime.NewMemoryDBPool()

	services := &runtime.ActionServices{
		DBPool:     dbPool,
		TxRegistry: runtime.NewMemoryTxRegistry(logger),
		Locker:     runtime.NewMemoryLocker(),
		KVStore:    runtime.NewMemoryKVStore(),
	}

	reg.SetAllowlist(cliAllowedActions(reg.Names()))

	for _, step := range wf.Steps {
		_, err := reg.Create(step.Action)
		if err != nil {
			closeErr := dbPool.Close()
			if closeErr != nil {
				logger.Warn("failed to close db pool", "error", closeErr)
			}

			return nil, nil, nil, fmt.Errorf("step %q uses %q which requires 'tailflow serve'", step.ID, step.Action)
		}
	}

	exec := engine.NewExecutor(reg, bus, logger, wf.Sensitive, tracer, bm)

	return exec, services, func() {
		err := dbPool.Close()
		if err != nil {
			logger.Warn("failed to close db pool", "error", err)
		}
	}, nil
}

func setupRunResources(bus *event.Bus, renderer *cliRenderer) *runResources {
	res := &runResources{bus: bus}

	ch := bus.Subscribe(eventBusBuffer)

	res.wg.Go(func() {
		for ev := range ch {
			renderer.handleEvent(ev)
		}
	})

	if renderer.isTTY {
		res.tickDone = make(chan struct{})

		go func() {
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					renderer.tick()
				case <-res.tickDone:
					return
				}
			}
		}()
	}

	return res
}

func buildExecutionOptions(
	renderer *cliRenderer, wf *parser.Workflow, path, data string, services *runtime.ActionServices,
) []engine.ExecuteOptions {
	baseOpts := engine.ExecuteOptions{Services: services}

	if wf.Trigger != nil {
		if data == "" {
			fmt.Printf("\n  %s %s\n",
				renderer.c("33", "Note:"),
				renderer.c("33", "this workflow has a trigger. Use --data/-d to provide a JSON body."),
			)
			fmt.Printf("  %s\n",
				renderer.c("33", fmt.Sprintf("  Example: tailflow run %s -d '{\"key\":\"value\"}'", path)),
			)
		}

		baseOpts.TriggerData = buildCLITriggerData(wf, data)
	}

	return []engine.ExecuteOptions{baseOpts}
}

func runAndReport(
	renderer *cliRenderer, exec *engine.Executor,
	wf *parser.Workflow, params map[string]any,
	opts []engine.ExecuteOptions, res *runResources,
) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	start := time.Now()

	result, err := exec.Execute(ctx, wf, params, opts...)
	if err != nil {
		res.shutdown()

		return fmt.Errorf("execution failed: %w", err)
	}

	elapsed := time.Since(start)

	res.shutdown()

	renderer.printSummary(result, elapsed)
	fmt.Println()

	if result.Status != runtime.StatusSuccess && result.Status != runtime.StatusCompletedWithErrors {
		cancel()
		os.Exit(1) //nolint:gocritic // cancel() called explicitly above
	}

	return nil
}

func buildCLITriggerData(wf *parser.Workflow, data string) map[string]any {
	triggerData := map[string]any{
		"method":  "CLI",
		"path":    "",
		"headers": map[string]string{},
		"query":   map[string][]string{},
		"body":    nil,
	}

	if wf.Trigger.HTTP != nil {
		triggerData["method"] = wf.Trigger.HTTP.Method
		triggerData["path"] = wf.Trigger.HTTP.Path
	} else if wf.Trigger.Webhook != nil {
		triggerData["method"] = "POST"
		triggerData["path"] = wf.Trigger.Webhook.Path
	}

	if data != "" {
		var body any

		err := json.Unmarshal([]byte(data), &body)
		if err == nil {
			triggerData["body"] = body
		} else {
			triggerData["body"] = data
		}
	}

	return triggerData
}
