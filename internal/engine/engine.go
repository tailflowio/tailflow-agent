package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	goruntime "runtime"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tailflow/tailflow/internal/action"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/runtime"
)

type Executor struct {
	registry *action.Registry
	bus      *event.Bus
	eval     *runtime.ExprEvaluator
	logger   *slog.Logger
}

func NewExecutor(registry *action.Registry, bus *event.Bus, logger *slog.Logger) *Executor {
	return &Executor{
		registry: registry,
		bus:      bus,
		eval:     runtime.NewExprEvaluator(),
		logger:   logger,
	}
}

type ExecuteResult struct {
	ExecutionID string
	Status      string // see runtime.Status* constants
	HasErrors   bool   // true when some steps failed but error_policy allowed continuation
	Steps       map[string]*runtime.StepResult
	Error       error
	StartedAt   time.Time
	FinishedAt  time.Time
}

type ExecuteOptions struct {
	ExecutionID string // Pre-generated execution ID (if empty, one is generated)
	TriggerData map[string]any
	Services    *runtime.ActionServices // Server-side services (nil in CLI mode)
}

func (e *Executor) Execute(
	ctx context.Context, wf *parser.Workflow, params map[string]any, opts ...ExecuteOptions,
) (*ExecuteResult, error) {
	executionID := uuid.New().String()
	if len(opts) > 0 && opts[0].ExecutionID != "" {
		executionID = opts[0].ExecutionID
	}

	startedAt := time.Now()

	// Resolve params with defaults
	resolvedParams := make(map[string]any)

	for _, p := range wf.Params {
		if v, ok := params[p.Name]; ok {
			resolvedParams[p.Name] = v
		} else if p.Default != nil {
			resolvedParams[p.Name] = p.Default
		} else if p.Required {
			return nil, fmt.Errorf("required parameter %q not provided", p.Name)
		}
	}
	// Validate param patterns
	for _, p := range wf.Params {
		if p.Pattern == "" {
			continue
		}
		v, ok := resolvedParams[p.Name]
		if !ok {
			continue
		}
		s, ok := v.(string)
		if !ok {
			continue
		}
		re, err := regexp.Compile("^(?:" + p.Pattern + ")$")
		if err != nil {
			return nil, fmt.Errorf("invalid pattern for parameter %q: %w", p.Name, err)
		}
		if !re.MatchString(s) {
			return nil, fmt.Errorf("parameter %q value %q does not match pattern %q", p.Name, s, p.Pattern)
		}
	}

	// Include any extra params
	for k, v := range params {
		if _, exists := resolvedParams[k]; !exists {
			resolvedParams[k] = v
		}
	}

	// Resolve env vars
	resolvedEnv := make(map[string]string, len(wf.Env))

	for k, v := range wf.Env {
		expanded := os.ExpandEnv(v)
		ctxMap := map[string]any{"params": resolvedParams}

		resolved, err := e.eval.ResolveTemplate(expanded, ctxMap)
		if err != nil {
			return nil, fmt.Errorf("resolve env %q: %w", k, err)
		}

		resolvedEnv[k] = resolved
	}

	execCtx := runtime.NewExecutionContext(executionID, wf.Name, resolvedParams, resolvedEnv)

	// Inject trigger data if provided
	if len(opts) > 0 && opts[0].TriggerData != nil {
		execCtx.TriggerData = opts[0].TriggerData
	}

	// Inject services if provided
	if len(opts) > 0 && opts[0].Services != nil {
		execCtx.Services = opts[0].Services
	}

	e.bus.Publish(event.NewEvent(event.WorkflowStarted, executionID, "", fmt.Sprintf("workflow %q started", wf.Name)))

	// Build DAG
	dag, err := BuildDAG(wf.Steps)
	if err != nil {
		return nil, fmt.Errorf("build DAG: %w", err)
	}

	// Execute DAG
	execErr := e.executeDAG(ctx, dag, execCtx)

	// Workflow-level on_error: runs if any error occurred
	if len(wf.OnError) > 0 && (execErr != nil || hasFailedSteps(execCtx)) {
		e.executeOnError(ctx, parser.Step{OnError: wf.OnError}, execCtx)
	}

	status := runtime.StatusSuccess
	hasErrors := false

	if execErr != nil {
		if ctx.Err() != nil {
			status = runtime.StatusCancelled
		} else {
			status = runtime.StatusFailed
		}
	} else {
		// Check if any steps failed with continue policy (no propagated error)
		for _, sr := range execCtx.Steps {
			if sr.Status == runtime.StatusFailed {
				hasErrors = true
				status = runtime.StatusCompletedWithErrors

				break
			}
		}
	}

	finishedAt := time.Now()
	e.bus.Publish(event.Event{
		Type:        event.WorkflowCompleted,
		Timestamp:   finishedAt,
		ExecutionID: executionID,
		Message:     fmt.Sprintf("workflow %q %s", wf.Name, status),
		Data:        map[string]any{"status": status},
	})

	result := &ExecuteResult{
		ExecutionID: executionID,
		Status:      status,
		HasErrors:   hasErrors,
		Steps:       execCtx.Steps,
		Error:       execErr,
		StartedAt:   startedAt,
		FinishedAt:  finishedAt,
	}

	return result, nil
}

type loopInfo struct {
	body    []string
	bodySet map[string]bool
}

func (e *Executor) executeDAG(ctx context.Context, dag *DAG, execCtx *runtime.ExecutionContext) error {
	// Track in-degree for each node
	inDegree := make(map[string]int, len(dag.Nodes))
	for id, node := range dag.Nodes {
		inDegree[id] = len(node.Parents)
	}

	// Precompute goto loop info
	gotoLoops := map[string]loopInfo{}
	gotoIterations := map[string]int{}

	for id, node := range dag.Nodes {
		if node.Step.Goto != nil {
			body := FindLoopBody(dag, node.Step.Goto.Target, id)
			bodySet := make(map[string]bool, len(body))

			for _, bid := range body {
				bodySet[bid] = true
			}

			gotoLoops[id] = loopInfo{body, bodySet}
		}
	}

	// Semaphore for concurrency control
	sem := make(chan struct{}, goruntime.NumCPU())

	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error

	// ready channel for nodes that are ready to execute
	ready := make(chan *DAGNode, len(dag.Nodes))

	// Queue root nodes
	for _, root := range dag.Roots {
		ready <- root
	}

	done := make(chan struct{})
	completed := 0
	total := len(dag.Nodes)

	go func() {
		for node := range ready {
			mu.Lock()
			if firstErr != nil {
				mu.Unlock()
				// Mark as skipped
				execCtx.SetStepResult(node.Step.ID, &runtime.StepResult{Status: runtime.StatusSkipped})
				e.bus.Publish(event.NewEvent(event.StepSkipped, execCtx.ExecutionID, node.Step.ID, "skipped due to prior failure"))
				mu.Lock()

				completed++
				if completed >= total {
					mu.Unlock()
					close(done)

					return
				}
				mu.Unlock()
				// Propagate skip to children
				for _, child := range node.Children {
					mu.Lock()
					inDegree[child.Step.ID]--
					shouldEnqueue := inDegree[child.Step.ID] <= 0
					mu.Unlock()

					if shouldEnqueue {
						ready <- child
					}
				}

				continue
			}
			mu.Unlock()

			wg.Add(1)
			sem <- struct{}{} // Acquire semaphore

			go func(n *DAGNode) {
				defer wg.Done()
				defer func() { <-sem }() // Release semaphore

				err := e.executeNode(ctx, n, execCtx)

				mu.Lock()
				if err != nil && firstErr == nil {
					firstErr = fmt.Errorf("step %q: %w", n.Step.ID, err)
				}

				completed++

				// Check goto condition after successful execution
				if err == nil && n.Step.Goto != nil {
					li, hasLoop := gotoLoops[n.Step.ID]
					if hasLoop {
						maxIter := n.Step.Goto.MaxIterations
						shouldGoto := false
						gotoResult, evalErr := e.eval.EvalBool(n.Step.Goto.When, execCtx.ToMap())

						if evalErr == nil && gotoResult && gotoIterations[n.Step.ID] < maxIter {
							shouldGoto = true
						}

						if shouldGoto {
							gotoIterations[n.Step.ID]++
							iter := gotoIterations[n.Step.ID]

							// Reset loop body: clear results and recompute in-degrees
							for _, bid := range li.body {
								inDegree[bid] = loopInDegree(dag, li.bodySet, bid)
								completed--

								execCtx.ClearStepResult(bid)
							}
							// Enqueue nodes in the body with inDegree == 0
							var readyNodes []*DAGNode

							for _, bid := range li.body {
								if inDegree[bid] == 0 {
									readyNodes = append(readyNodes, dag.Nodes[bid])
								}
							}
							mu.Unlock()

							// Emit step.goto event
							e.bus.Publish(event.Event{
								Type:        event.StepGoto,
								Timestamp:   time.Now(),
								ExecutionID: execCtx.ExecutionID,
								StepID:      n.Step.ID,
								Message:     fmt.Sprintf("goto %s (iteration %d/%d)", n.Step.Goto.Target, iter+1, maxIter),
								Data: map[string]any{
									"target":         n.Step.Goto.Target,
									"iteration":      iter + 1,
									"max_iterations": maxIter,
									"body":           li.body,
								},
							})

							for _, rn := range readyNodes {
								ready <- rn
							}

							return // Do NOT propagate to children
						}
					}
				}

				allDone := completed >= total
				mu.Unlock()

				if allDone {
					close(done)
					return
				}

				// Decrement in-degree of children and enqueue if ready
				for _, child := range n.Children {
					mu.Lock()
					inDegree[child.Step.ID]--
					shouldEnqueue := inDegree[child.Step.ID] <= 0
					mu.Unlock()

					if shouldEnqueue {
						ready <- child
					}
				}
			}(node)
		}
	}()

	<-done
	wg.Wait()

	return firstErr
}

func (e *Executor) executeNode(ctx context.Context, node *DAGNode, execCtx *runtime.ExecutionContext) error {
	step := node.Step
	logger := e.logger.With("step", step.ID)

	// Evaluate "when" condition
	if step.When != "" {
		result, err := e.eval.EvalBool(step.When, execCtx.ToMap())
		if err != nil {
			return fmt.Errorf("eval when condition: %w", err)
		}

		if !result {
			execCtx.SetStepResult(step.ID, &runtime.StepResult{Status: runtime.StatusSkipped})
			e.bus.Publish(event.NewEvent(event.StepSkipped, execCtx.ExecutionID, step.ID, "condition not met"))
			logger.Info("skipped (when condition false)")

			return nil
		}
	}

	e.bus.Publish(event.NewEvent(event.StepStarted, execCtx.ExecutionID, step.ID, fmt.Sprintf("step %q started", step.ID)))
	logger.Info("started", "action", step.Action)

	stepStartedAt := time.Now()

	// For loop actions, defer resolution of action_config and actions (they contain
	// {{ loop.* }} templates that can only be resolved at iteration time inside RunAction).
	var rawActionConfig map[string]any
	var rawActionsArray []any

	configToResolve := step.Config

	if step.Action == "loop" {
		needsCopy := false

		if ac, ok := step.Config["action_config"]; ok {
			rawActionConfig, _ = ac.(map[string]any)
			needsCopy = true
		}

		if aa, ok := step.Config["actions"]; ok {
			rawActionsArray, _ = aa.([]any)
			needsCopy = true
		}

		if needsCopy {
			configToResolve = make(map[string]any, len(step.Config))

			for k, v := range step.Config {
				if k != "action_config" && k != "actions" {
					configToResolve[k] = v
				}
			}
		}
	}

	// Resolve config templates
	resolvedConfig, err := e.eval.ResolveConfig(configToResolve, execCtx.ToMap())
	if err != nil {
		return fmt.Errorf("resolve config: %w", err)
	}

	// Re-inject raw action_config / actions so the loop action can pass them to RunAction
	if rawActionConfig != nil {
		resolvedConfig["action_config"] = rawActionConfig
	}

	if rawActionsArray != nil {
		resolvedConfig["actions"] = rawActionsArray
	}

	// Create action
	act, err := e.registry.Create(step.Action)
	if err != nil {
		return err
	}

	// Emit step.input with useful data (not raw scripts)
	inputData := map[string]any{}
	// Resolved config minus script source code
	filteredConfig := make(map[string]any, len(resolvedConfig))

	for k, v := range resolvedConfig {
		if k != "script" {
			filteredConfig[k] = v
		}
	}

	if len(filteredConfig) > 0 {
		inputData["config"] = filteredConfig
	}
	// Trigger data if available
	if len(execCtx.TriggerData) > 0 {
		inputData["trigger"] = execCtx.TriggerData
	}
	// Outputs from dependency steps
	deps := make(map[string]any)

	for _, dep := range step.DependsOn {
		if r, ok := execCtx.GetStepResult(dep); ok && r.Output != nil {
			deps[dep] = r.Output
		}
	}

	if len(deps) > 0 {
		inputData["deps"] = deps
	}

	e.bus.Publish(event.Event{
		Type:        event.StepInput,
		Timestamp:   time.Now(),
		ExecutionID: execCtx.ExecutionID,
		StepID:      step.ID,
		Message:     fmt.Sprintf("step %q input", step.ID),
		Data:        inputData,
	})

	emitLog := func(msg string) {
		e.bus.Publish(event.Event{
			Type:        event.StepLog,
			Timestamp:   time.Now(),
			ExecutionID: execCtx.ExecutionID,
			StepID:      step.ID,
			Message:     msg,
			Data:        map[string]any{"stream": true},
		})
	}

	actCtx := &action.ActionContext{
		Context:  ctx,
		Config:   resolvedConfig,
		ExecCtx:  execCtx,
		StepID:   step.ID,
		Logger:   logger,
		Services: execCtx.Services,
		EmitLog:  emitLog,
		RunAction: func(actionName string, rawConfig map[string]any, loopVars map[string]any) (any, error) {
			// Build merged context: execCtx.ToMap() + "loop" key
			ctxMap := execCtx.ToMap()
			ctxMap["loop"] = loopVars

			// Resolve config templates with merged context
			resolved, err := e.eval.ResolveConfig(rawConfig, ctxMap)
			if err != nil {
				return nil, fmt.Errorf("resolve loop action config: %w", err)
			}

			// Create action instance
			subAct, err := e.registry.Create(actionName)
			if err != nil {
				return nil, fmt.Errorf("create loop action %q: %w", actionName, err)
			}

			// Build sub-ActionContext
			subCtx := &action.ActionContext{
				Context:  ctx,
				Config:   resolved,
				ExecCtx:  execCtx,
				StepID:   step.ID,
				Logger:   logger,
				Services: execCtx.Services,
				EmitLog:  emitLog,
			}

			err = subAct.Validate(subCtx)
			if err != nil {
				return nil, fmt.Errorf("validate loop action %q: %w", actionName, err)
			}

			// Execute
			return subAct.Execute(subCtx)
		},
	}

	err = act.Validate(actCtx)
	if err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	// Execute with retry
	maxAttempts := 1
	var retryDelay time.Duration

	if step.Retry != nil {
		maxAttempts = step.Retry.MaxAttempts
		retryDelay, _ = step.Retry.ParsedDelay()
	}

	var output any
	var execErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			logger.Info("retrying", "attempt", attempt, "max", maxAttempts)
			e.bus.Publish(event.Event{
				Type:        event.StepStarted,
				Timestamp:   time.Now(),
				ExecutionID: execCtx.ExecutionID,
				StepID:      step.ID,
				Message:     fmt.Sprintf("retry attempt %d/%d", attempt, maxAttempts),
				Data:        map[string]any{"attempt": attempt},
			})

			select {
			case <-time.After(retryDelay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		// Handle timeout
		execCtxInner := ctx

		if step.Timeout != "" {
			dur, err := time.ParseDuration(step.Timeout)
			if err == nil {
				var cancel context.CancelFunc
				execCtxInner, cancel = context.WithTimeout(ctx, dur)
				defer cancel()
			}
		}

		actCtx.Context = execCtxInner

		output, execErr = act.Execute(actCtx)
		if execErr == nil {
			break
		}
	}

	if execErr != nil {
		policy := step.ErrorPolicy // "", "stop", "continue", "ignore"

		if policy == "ignore" {
			// Suppress error: mark as success with suppressed error in output
			suppressedOutput := map[string]any{"_suppressed_error": execErr.Error()}

			if outMap, ok := output.(map[string]any); ok {
				for k, v := range outMap {
					suppressedOutput[k] = v
				}
			}

			now := time.Now()
			execCtx.SetStepResult(step.ID, &runtime.StepResult{
				Status:     runtime.StatusSuccess,
				Output:     suppressedOutput,
				StartedAt:  &stepStartedAt,
				FinishedAt: &now,
			})
			logger.Warn("error ignored (error_policy=ignore)", "error", execErr)
			e.bus.Publish(event.Event{
				Type:        event.StepCompleted,
				Timestamp:   time.Now(),
				ExecutionID: execCtx.ExecutionID,
				StepID:      step.ID,
				Message:     fmt.Sprintf("step %q completed (error ignored)", step.ID),
				Data:        map[string]any{"output": suppressedOutput},
			})

			return nil
		}

		failedAt := time.Now()
		execCtx.SetStepResult(step.ID, &runtime.StepResult{
			Status:     runtime.StatusFailed,
			Output:     output,
			Error:      stepError(step.ID, execErr),
			StartedAt:  &stepStartedAt,
			FinishedAt: &failedAt,
		})
		e.bus.Publish(event.Event{
			Type:        event.StepFailed,
			Timestamp:   time.Now(),
			ExecutionID: execCtx.ExecutionID,
			StepID:      step.ID,
			Message:     execErr.Error(),
		})
		logger.Error("failed", "error", execErr)

		// Execute on_error steps
		if len(step.OnError) > 0 {
			e.executeOnError(ctx, step, execCtx)
		}

		if policy == "continue" {
			// Step failed but dependants can still execute
			logger.Warn("continuing despite error (error_policy=continue)")
			return nil
		}

		// Default: "stop" or empty — propagate error
		return execErr
	}

	completedAt := time.Now()

	execCtx.SetStepResult(step.ID, &runtime.StepResult{
		Status:     runtime.StatusSuccess,
		Output:     output,
		StartedAt:  &stepStartedAt,
		FinishedAt: &completedAt,
	})

	// Emit step.log for log actions so the message appears in the event stream
	if step.Action == "log" {
		if outMap, ok := output.(map[string]any); ok {
			logMsg, _ := outMap["message"].(string)
			logLevel, _ := outMap["level"].(string)
			e.bus.Publish(event.Event{
				Type:        event.StepLog,
				Timestamp:   time.Now(),
				ExecutionID: execCtx.ExecutionID,
				StepID:      step.ID,
				Message:     logMsg,
				Data:        map[string]any{"level": logLevel},
			})
		}
	}

	// Emit step.output with result data
	e.bus.Publish(event.Event{
		Type:        event.StepOutput,
		Timestamp:   time.Now(),
		ExecutionID: execCtx.ExecutionID,
		StepID:      step.ID,
		Message:     fmt.Sprintf("step %q output", step.ID),
		Data:        map[string]any{"output": output},
	})

	e.bus.Publish(event.Event{
		Type:        event.StepCompleted,
		Timestamp:   time.Now(),
		ExecutionID: execCtx.ExecutionID,
		StepID:      step.ID,
		Message:     fmt.Sprintf("step %q completed", step.ID),
		Data:        map[string]any{"output": output},
	})
	logger.Info("completed")

	return nil
}

func (e *Executor) executeOnError(ctx context.Context, step parser.Step, execCtx *runtime.ExecutionContext) {
	for _, errStep := range step.OnError {
		logger := e.logger.With("step", errStep.ID, "on_error_of", step.ID)
		logger.Info("executing on_error step")

		resolvedConfig, err := e.eval.ResolveConfig(errStep.Config, execCtx.ToMap())
		if err != nil {
			logger.Error("failed to resolve on_error config", "error", err)
			continue
		}

		act, err := e.registry.Create(errStep.Action)
		if err != nil {
			logger.Error("failed to create on_error action", "error", err)
			continue
		}

		actCtx := &action.ActionContext{
			Context:  ctx,
			Config:   resolvedConfig,
			ExecCtx:  execCtx,
			StepID:   errStep.ID,
			Logger:   logger,
			Services: execCtx.Services,
		}

		output, err := act.Execute(actCtx)
		if err != nil {
			logger.Error("on_error step failed", "error", err)
			execCtx.SetStepResult(errStep.ID, &runtime.StepResult{Status: runtime.StatusFailed, Error: stepError(errStep.ID, err)})
		} else {
			execCtx.SetStepResult(errStep.ID, &runtime.StepResult{Status: runtime.StatusSuccess, Output: output})
		}
	}
}

func stepError(stepID string, err error) *runtime.StepError {
	code := "action_failed"
	if errors.Is(err, context.DeadlineExceeded) {
		code = "timeout"
	} else if errors.Is(err, context.Canceled) {
		code = "cancelled"
	}
	return &runtime.StepError{Message: err.Error(), Code: code, StepID: stepID}
}

func hasFailedSteps(execCtx *runtime.ExecutionContext) bool {
	for _, sr := range execCtx.Steps {
		if sr.Status == runtime.StatusFailed {
			return true
		}
	}
	return false
}
