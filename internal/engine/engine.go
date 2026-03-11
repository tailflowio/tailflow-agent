package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"regexp"
	goruntime "runtime"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tailflow/tailflow/internal/action"
	"github.com/tailflow/tailflow/internal/event"
	tfotel "github.com/tailflow/tailflow/internal/otel"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/runtime"
)

type Executor struct {
	registry        *action.Registry
	bus             *event.Bus
	eval            *runtime.ExprEvaluator
	logger          *slog.Logger
	sensitive       *SensitiveRegistry
	tracer          *tfotel.Tracer
	businessMetrics *tfotel.BusinessMetrics
}

func NewExecutor(
	registry *action.Registry, bus *event.Bus, logger *slog.Logger,
	sensitiveKeys []string, tracer *tfotel.Tracer, businessMetrics *tfotel.BusinessMetrics,
) *Executor {
	if tracer == nil {
		tracer = tfotel.NewTracer(nil)
	}

	return &Executor{
		registry:        registry,
		bus:             bus,
		eval:            runtime.NewExprEvaluator(),
		logger:          logger,
		sensitive:       NewSensitiveRegistry(sensitiveKeys),
		tracer:          tracer,
		businessMetrics: businessMetrics,
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
	ExecutionID  string // Pre-generated execution ID (if empty, one is generated)
	TriggerData  map[string]any
	Services     *runtime.ActionServices // Server-side services (nil in CLI mode)
	TestCaseName string                  // test mode: inject mocks for this case
}

func (e *Executor) Execute(
	ctx context.Context, wf *parser.Workflow, params map[string]any, opts ...ExecuteOptions,
) (*ExecuteResult, error) {
	executionID := resolveExecutionID(opts)
	startedAt := time.Now()

	resolvedParams, err := e.resolveParams(wf, params)
	if err != nil {
		return nil, err
	}

	resolvedEnv, err := e.resolveEnv(wf, resolvedParams)
	if err != nil {
		return nil, err
	}

	execCtx := e.buildExecutionContext(executionID, wf.Name, resolvedParams, resolvedEnv, opts)

	if len(opts) > 0 && opts[0].TestCaseName != "" {
		execCtx.TestCaseName = opts[0].TestCaseName
	}

	triggerType := ""
	if len(opts) > 0 && opts[0].TriggerData != nil {
		triggerType, _ = opts[0].TriggerData["method"].(string)
	}

	ctx, finishWorkflow := e.tracer.StartWorkflow(ctx, executionID, wf.Name, triggerType)
	e.businessMetrics.RecordWorkflowStarted(ctx, wf.Name)

	e.bus.Publish(event.NewEvent(event.WorkflowStarted, executionID, "", fmt.Sprintf("workflow %q started", wf.Name)))

	dag, err := BuildDAG(wf.Steps)
	if err != nil {
		finishWorkflow("failed", err)

		return nil, fmt.Errorf("build DAG: %w", err)
	}

	execErr := e.executeDAG(ctx, dag, execCtx)

	if len(wf.OnError) > 0 && (execErr != nil || execCtx.HasFailedSteps()) {
		e.executeOnError(ctx, parser.Step{OnError: wf.OnError}, execCtx)
	}

	result := e.buildExecuteResult(ctx, wf.Name, execCtx, execErr, executionID, startedAt)
	finishWorkflow(result.Status, result.Error)
	e.businessMetrics.RecordWorkflowCompleted(ctx, wf.Name, result.Status, tfotel.DurationMs(startedAt))

	return result, nil
}

func resolveExecutionID(opts []ExecuteOptions) string {
	if len(opts) > 0 && opts[0].ExecutionID != "" {
		return opts[0].ExecutionID
	}

	return uuid.New().String()
}

func (e *Executor) buildExecutionContext(
	executionID, wfName string, resolvedParams map[string]any,
	resolvedEnv map[string]string, opts []ExecuteOptions,
) *runtime.ExecutionContext {
	execCtx := runtime.NewExecutionContext(executionID, wfName, resolvedParams, resolvedEnv)
	if len(opts) > 0 && opts[0].TriggerData != nil {
		execCtx.TriggerData = opts[0].TriggerData
	}

	if len(opts) > 0 && opts[0].Services != nil {
		execCtx.Services = opts[0].Services
	}

	return execCtx
}

func (e *Executor) buildExecuteResult(
	ctx context.Context, wfName string, execCtx *runtime.ExecutionContext,
	execErr error, executionID string, startedAt time.Time,
) *ExecuteResult {
	status, hasErrors := e.determineStatus(ctx, execErr, execCtx)
	finishedAt := time.Now()
	e.publishWorkflowCompleted(executionID, wfName, status, finishedAt)

	return &ExecuteResult{
		ExecutionID: executionID,
		Status:      status,
		HasErrors:   hasErrors,
		Steps:       copyStepResults(execCtx.Steps),
		Error:       execErr,
		StartedAt:   startedAt,
		FinishedAt:  finishedAt,
	}
}

func (e *Executor) publishWorkflowCompleted(executionID, wfName, status string, ts time.Time) {
	e.bus.Publish(event.Event{
		Type:        event.WorkflowCompleted,
		Timestamp:   ts,
		ExecutionID: executionID,
		Message:     fmt.Sprintf("workflow %q %s", wfName, status),
		Data:        map[string]any{"status": status},
	})
}

func copyStepResults(steps map[string]*runtime.StepResult) map[string]*runtime.StepResult {
	stepsCopy := make(map[string]*runtime.StepResult, len(steps))
	for k, v := range steps {
		cp := *v
		stepsCopy[k] = &cp
	}

	return stepsCopy
}

func (e *Executor) resolveParams(wf *parser.Workflow, params map[string]any) (map[string]any, error) {
	resolved := make(map[string]any)

	for _, p := range wf.Params {
		v, ok := params[p.Name]

		switch {
		case ok:
			resolved[p.Name] = v
		case p.Default != nil:
			resolved[p.Name] = p.Default
		case p.Required:
			return nil, fmt.Errorf("required parameter %q not provided", p.Name)
		}
	}

	for _, p := range wf.Params {
		if p.Pattern == "" {
			continue
		}

		v, ok := resolved[p.Name]
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

	for k, v := range params {
		_, exists := resolved[k]
		if !exists {
			resolved[k] = v
		}
	}

	return resolved, nil
}

func (e *Executor) resolveEnv(wf *parser.Workflow, resolvedParams map[string]any) (map[string]string, error) {
	resolved := make(map[string]string, len(wf.Env))

	for k, v := range wf.Env {
		expanded := os.ExpandEnv(v)
		ctxMap := map[string]any{"params": resolvedParams}

		val, err := e.eval.ResolveTemplate(expanded, ctxMap)
		if err != nil {
			return nil, fmt.Errorf("resolve env %q: %w", k, err)
		}

		resolved[k] = val
	}

	return resolved, nil
}

func (e *Executor) determineStatus(ctx context.Context, execErr error, execCtx *runtime.ExecutionContext) (string, bool) {
	if execErr != nil && ctx.Err() != nil {
		return runtime.StatusCancelled, false
	}

	if execErr != nil {
		return runtime.StatusFailed, false
	}

	if execCtx.HasFailedSteps() {
		return runtime.StatusCompletedWithErrors, true
	}

	return runtime.StatusSuccess, false
}

type loopInfo struct {
	body    []string
	bodySet map[string]bool
}

// dagState holds shared mutable state for a DAG execution.
type dagState struct {
	mu             sync.Mutex
	wg             sync.WaitGroup
	firstErr       error
	ready          chan *DAGNode
	done           chan struct{}
	completed      int
	total          int
	inDegree       map[string]int
	gotoLoops      map[string]loopInfo
	gotoIterations map[string]int
	sem            chan struct{}
}

func newDAGState(dag *DAG) *dagState {
	inDegree := make(map[string]int, len(dag.Nodes))
	for id, node := range dag.Nodes {
		inDegree[id] = len(node.Parents)
	}

	gotoLoops := make(map[string]loopInfo)

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

	ready := make(chan *DAGNode, len(dag.Nodes))
	for _, root := range dag.Roots {
		ready <- root
	}

	return &dagState{
		ready:          ready,
		done:           make(chan struct{}),
		total:          len(dag.Nodes),
		inDegree:       inDegree,
		gotoLoops:      gotoLoops,
		gotoIterations: make(map[string]int),
		sem:            make(chan struct{}, goruntime.NumCPU()),
	}
}

func (e *Executor) executeDAG(ctx context.Context, dag *DAG, execCtx *runtime.ExecutionContext) error {
	ds := newDAGState(dag)

	go e.dagLoop(ctx, dag, execCtx, ds)

	<-ds.done
	ds.wg.Wait()

	return ds.firstErr
}

func (e *Executor) dagLoop(ctx context.Context, dag *DAG, execCtx *runtime.ExecutionContext, ds *dagState) {
	for node := range ds.ready {
		ds.mu.Lock()
		hasError := ds.firstErr != nil
		ds.mu.Unlock()

		if hasError {
			e.skipNode(node, execCtx, ds)

			continue
		}

		ds.wg.Add(1)

		ds.sem <- struct{}{}

		go e.runDAGNode(ctx, node, execCtx, dag, ds)
	}
}

func (e *Executor) skipNode(node *DAGNode, execCtx *runtime.ExecutionContext, ds *dagState) {
	execCtx.SetStepResult(node.Step.ID, &runtime.StepResult{Status: runtime.StatusSkipped})
	e.bus.Publish(event.NewEvent(event.StepSkipped, execCtx.ExecutionID, node.Step.ID, "skipped due to prior failure"))

	ds.mu.Lock()
	ds.completed++
	allDone := ds.completed >= ds.total
	ds.mu.Unlock()

	if allDone {
		close(ds.done)

		return
	}

	e.enqueueChildren(node.Children, ds)
}

func (e *Executor) runDAGNode(ctx context.Context, n *DAGNode, execCtx *runtime.ExecutionContext, dag *DAG, ds *dagState) {
	defer ds.wg.Done()
	defer func() { <-ds.sem }()

	err := e.executeNode(ctx, n, execCtx)

	ds.mu.Lock()
	if err != nil && ds.firstErr == nil {
		ds.firstErr = fmt.Errorf("step %q: %w", n.Step.ID, err)
	}

	ds.completed++

	if err == nil && n.Step.Goto != nil {
		readyNodes, triggered := e.handleGoto(n, execCtx, dag, ds.gotoLoops, ds.gotoIterations, ds.inDegree, &ds.completed)
		if triggered {
			ds.mu.Unlock()

			for _, rn := range readyNodes {
				ds.ready <- rn
			}

			return
		}
	}

	allDone := ds.completed >= ds.total
	ds.mu.Unlock()

	if allDone {
		close(ds.done)

		return
	}

	e.enqueueChildren(n.Children, ds)
}

func (e *Executor) enqueueChildren(children []*DAGNode, ds *dagState) {
	for _, child := range children {
		ds.mu.Lock()
		ds.inDegree[child.Step.ID]--
		shouldEnqueue := ds.inDegree[child.Step.ID] <= 0
		ds.mu.Unlock()

		if shouldEnqueue {
			ds.ready <- child
		}
	}
}

func (e *Executor) executeNode(ctx context.Context, node *DAGNode, execCtx *runtime.ExecutionContext) error {
	step := node.Step
	logger := e.logger.With("step", step.ID)

	skipped, err := e.evaluateWhenCondition(step, execCtx, logger)
	if skipped || err != nil {
		return err
	}

	if execCtx.TestCaseName != "" {
		tc := findTestCase(step, execCtx.TestCaseName)
		if tc != nil && (tc.Output != nil || tc.Error != nil) {
			mockErr := e.applyTestMock(step, tc, execCtx, logger)
			if mockErr == nil && tc.Expect != nil {
				expectErr := e.checkTestExpect(step, tc.Expect, execCtx)
				if expectErr != nil {
					return expectErr
				}
			}

			return mockErr
		}
	}

	e.bus.Publish(event.NewEvent(event.StepStarted, execCtx.ExecutionID, step.ID, fmt.Sprintf("step %q started", step.ID)))
	logger.Info("started", "action", step.Action)

	stepStartedAt := time.Now()

	resolvedConfig, err := e.resolveStepConfig(step, execCtx)
	if err != nil {
		_, finishStep := e.tracer.StartStep(ctx, step.ID, step.Action, nil)
		finishStep("failed", err, nil, nil)

		return err
	}

	ctx, finishStep := e.tracer.StartStep(ctx, step.ID, step.Action, resolvedConfig)

	act, err := e.registry.Create(step.Action)
	if err != nil {
		finishStep("failed", err, nil, nil)

		return err
	}

	e.emitStepInput(step, resolvedConfig, execCtx)

	emitLog := e.newLogEmitter(step.ID, execCtx)
	actCtx := e.createActionContext(ctx, step, resolvedConfig, execCtx, logger, emitLog)

	err = act.Validate(actCtx)
	if err != nil {
		finishStep("failed", fmt.Errorf("validate: %w", err), nil, nil)

		return fmt.Errorf("validate: %w", err)
	}

	output, execErr := e.executeWithRetry(ctx, step, act, actCtx, execCtx, logger)
	if execErr != nil {
		maskedInput := e.sensitive.MaskMap(e.prepareStepInput(step, resolvedConfig, execCtx))
		finishStep("failed", execErr, maskedInput, nil)
		e.businessMetrics.RecordStepCompleted(
			ctx, step.ID, step.Action, "failed", tfotel.DurationMs(stepStartedAt),
		)
		e.businessMetrics.RecordStepError(ctx, step.ID, step.Action, stepErrorCode(execErr))

		return e.handleStepError(ctx, step, output, execErr, execCtx, &stepStartedAt, logger)
	}

	maskedInput := e.sensitive.MaskMap(e.prepareStepInput(step, resolvedConfig, execCtx))
	maskedOutput := e.sensitive.MaskAny(output)
	finishStep("success", nil, maskedInput, maskedOutput)
	e.businessMetrics.RecordStepCompleted(
		ctx, step.ID, step.Action, "success", tfotel.DurationMs(stepStartedAt),
	)

	e.recordStepSuccess(step, output, execCtx, &stepStartedAt, logger)

	if execCtx.TestCaseName != "" {
		tc := findTestCase(step, execCtx.TestCaseName)
		if tc != nil && tc.Expect != nil {
			expectErr := e.checkTestExpect(step, tc.Expect, execCtx)
			if expectErr != nil {
				return expectErr
			}
		}
	}

	return nil
}

func (e *Executor) evaluateWhenCondition(
	step parser.Step, execCtx *runtime.ExecutionContext, logger *slog.Logger,
) (skipped bool, err error) {
	if step.When == "" {
		return false, nil
	}

	result, err := e.eval.EvalBool(step.When, execCtx.ToMap())
	if err != nil {
		return false, fmt.Errorf("eval when condition: %w", err)
	}

	if !result {
		execCtx.SetStepResult(step.ID, &runtime.StepResult{Status: runtime.StatusSkipped})
		e.bus.Publish(event.NewEvent(event.StepSkipped, execCtx.ExecutionID, step.ID, "condition not met"))
		logger.Info("skipped (when condition false)")

		return true, nil
	}

	return false, nil
}

func (e *Executor) resolveStepConfig(step parser.Step, execCtx *runtime.ExecutionContext) (map[string]any, error) {
	configToResolve, rawActionConfig, rawActionsArray := e.extractLoopConfig(step)

	resolvedConfig, err := e.eval.ResolveConfig(configToResolve, execCtx.ToMap())
	if err != nil {
		return nil, fmt.Errorf("resolve config: %w", err)
	}

	if rawActionConfig != nil {
		resolvedConfig["action_config"] = rawActionConfig
	}

	if rawActionsArray != nil {
		resolvedConfig["actions"] = rawActionsArray
	}

	return resolvedConfig, nil
}

func (e *Executor) extractLoopConfig(
	step parser.Step,
) (configToResolve map[string]any, rawActionConfig map[string]any, rawActionsArray []any) {
	configToResolve = step.Config

	if step.Action != "loop" {
		return configToResolve, nil, nil
	}

	needsCopy := false

	ac, ok := step.Config["action_config"]
	if ok {
		rawActionConfig, _ = ac.(map[string]any)
		needsCopy = true
	}

	aa, ok := step.Config["actions"]
	if ok {
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

	return configToResolve, rawActionConfig, rawActionsArray
}

func (e *Executor) emitStepInput(step parser.Step, resolvedConfig map[string]any, execCtx *runtime.ExecutionContext) {
	inputData := e.prepareStepInput(step, resolvedConfig, execCtx)

	e.bus.Publish(e.sensitive.MaskEvent(event.Event{
		Type:        event.StepInput,
		Timestamp:   time.Now(),
		ExecutionID: execCtx.ExecutionID,
		StepID:      step.ID,
		Message:     fmt.Sprintf("step %q input", step.ID),
		Data:        inputData,
	}))
}

func (e *Executor) prepareStepInput(step parser.Step, resolvedConfig map[string]any, execCtx *runtime.ExecutionContext) map[string]any {
	inputData := map[string]any{}

	filteredConfig := make(map[string]any, len(resolvedConfig))

	for k, v := range resolvedConfig {
		if k != "script" {
			filteredConfig[k] = v
		}
	}

	if len(filteredConfig) > 0 {
		inputData["config"] = filteredConfig
	}

	if len(execCtx.TriggerData) > 0 {
		inputData["trigger"] = execCtx.TriggerData
	}

	deps := make(map[string]any)

	for _, dep := range step.DependsOn {
		r, ok := execCtx.GetStepResult(dep)
		if ok && r.Output != nil {
			deps[dep] = r.Output
		}
	}

	if len(deps) > 0 {
		inputData["deps"] = deps
	}

	return inputData
}

func (e *Executor) newLogEmitter(stepID string, execCtx *runtime.ExecutionContext) func(string) {
	return func(msg string) {
		e.bus.Publish(event.Event{
			Type:        event.StepLog,
			Timestamp:   time.Now(),
			ExecutionID: execCtx.ExecutionID,
			StepID:      stepID,
			Message:     msg,
			Data:        map[string]any{"stream": true},
		})
	}
}

func (e *Executor) newPrintEmitter(stepID string, execCtx *runtime.ExecutionContext) func(string) {
	return func(msg string) {
		e.bus.Publish(event.Event{
			Type:        event.StepLog,
			Timestamp:   time.Now(),
			ExecutionID: execCtx.ExecutionID,
			StepID:      stepID,
			Message:     msg,
		})
	}
}

func (e *Executor) createActionContext(
	ctx context.Context, step parser.Step, resolvedConfig map[string]any,
	execCtx *runtime.ExecutionContext, logger *slog.Logger, emitLog func(string),
) *action.ActionContext {
	return &action.ActionContext{
		Context:   ctx,
		Config:    resolvedConfig,
		ExecCtx:   execCtx,
		StepID:    step.ID,
		Logger:    logger,
		Services:  execCtx.Services,
		EmitLog:   emitLog,
		EmitPrint: e.newPrintEmitter(step.ID, execCtx),
		RunAction: e.newRunAction(ctx, step, execCtx, logger, emitLog),
	}
}

func (e *Executor) newRunAction(
	ctx context.Context, step parser.Step,
	execCtx *runtime.ExecutionContext, logger *slog.Logger, emitLog func(string),
) func(string, map[string]any, map[string]any) (any, error) {
	return func(actionName string, rawConfig map[string]any, loopVars map[string]any) (any, error) {
		ctxMap := execCtx.ToMap()
		ctxMap["loop"] = loopVars

		resolved, resolveErr := e.eval.ResolveConfig(rawConfig, ctxMap)
		if resolveErr != nil {
			return nil, fmt.Errorf("resolve loop action config: %w", resolveErr)
		}

		subAct, createErr := e.registry.Create(actionName)
		if createErr != nil {
			return nil, fmt.Errorf("create loop action %q: %w", actionName, createErr)
		}

		subCtx := &action.ActionContext{
			Context:   ctx,
			Config:    resolved,
			ExecCtx:   execCtx,
			StepID:    step.ID,
			Logger:    logger,
			Services:  execCtx.Services,
			EmitLog:   emitLog,
			EmitPrint: e.newPrintEmitter(step.ID, execCtx),
		}

		validateErr := subAct.Validate(subCtx)
		if validateErr != nil {
			return nil, fmt.Errorf("validate loop action %q: %w", actionName, validateErr)
		}

		return subAct.Execute(subCtx)
	}
}

func (e *Executor) executeWithRetry(
	ctx context.Context, step parser.Step, act action.Action,
	actCtx *action.ActionContext, execCtx *runtime.ExecutionContext, logger *slog.Logger,
) (any, error) {
	maxAttempts := 1

	var retryDelay time.Duration

	if step.Retry != nil {
		maxAttempts = step.Retry.MaxAttempts

		var delayErr error

		retryDelay, delayErr = step.Retry.ParsedDelay()
		if delayErr != nil {
			logger.WarnContext(ctx, "invalid retry delay, using default",
				"delay", step.Retry.Delay,
				"error", delayErr,
			)

			retryDelay = time.Second
		}
	}

	var output any
	var execErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			err := e.waitForRetry(ctx, step, attempt, maxAttempts, retryDelay, execCtx, logger)
			if err != nil {
				return nil, err
			}
		}

		retryCtx := ctx
		var finishRetry func(error)

		if maxAttempts > 1 {
			retryCtx, finishRetry = e.tracer.StartRetry(ctx, attempt, maxAttempts)
			e.businessMetrics.RecordStepRetry(ctx, step.ID, step.Action)
		}

		timeoutCtx, timeoutCancel := e.applyTimeout(retryCtx, step)
		actCtx.Context = timeoutCtx

		output, execErr = act.Execute(actCtx)

		timeoutCancel()

		if finishRetry != nil {
			finishRetry(execErr)
		}

		if execErr == nil {
			break
		}
	}

	return output, execErr
}

func (e *Executor) waitForRetry(
	ctx context.Context, step parser.Step, attempt, maxAttempts int,
	retryDelay time.Duration, execCtx *runtime.ExecutionContext, logger *slog.Logger,
) error {
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
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *Executor) applyTimeout(ctx context.Context, step parser.Step) (context.Context, context.CancelFunc) {
	if step.Timeout == "" {
		return ctx, func() {}
	}

	dur, err := time.ParseDuration(step.Timeout)
	if err == nil {
		return context.WithTimeout(ctx, dur)
	}

	return ctx, func() {}
}

func (e *Executor) handleStepError(
	ctx context.Context, step parser.Step, output any, execErr error,
	execCtx *runtime.ExecutionContext, stepStartedAt *time.Time, logger *slog.Logger,
) error {
	if step.ErrorPolicy == "ignore" {
		return e.handleIgnoredError(step, output, execErr, execCtx, stepStartedAt, logger)
	}

	failedAt := time.Now()
	execCtx.SetStepResult(step.ID, &runtime.StepResult{
		Status:     runtime.StatusFailed,
		Output:     output,
		Error:      stepError(step.ID, execErr),
		StartedAt:  stepStartedAt,
		FinishedAt: &failedAt,
	})

	e.bus.Publish(event.Event{
		Type:        event.StepFailed,
		Timestamp:   time.Now(),
		ExecutionID: execCtx.ExecutionID,
		StepID:      step.ID,
		Message:     execErr.Error(),
	})

	if len(step.OnError) > 0 {
		e.executeOnError(ctx, step, execCtx)
	}

	if step.ErrorPolicy == "continue" {
		logger.Warn("continuing despite error (error_policy=continue)")

		return nil
	}

	return execErr
}

func (e *Executor) handleIgnoredError(
	step parser.Step, output any, execErr error,
	execCtx *runtime.ExecutionContext, stepStartedAt *time.Time, logger *slog.Logger,
) error {
	suppressedOutput := map[string]any{"_suppressed_error": execErr.Error()}

	outMap, ok := output.(map[string]any)
	if ok {
		maps.Copy(suppressedOutput, outMap)
	}

	now := time.Now()
	execCtx.SetStepResult(step.ID, &runtime.StepResult{
		Status:     runtime.StatusSuccess,
		Output:     suppressedOutput,
		StartedAt:  stepStartedAt,
		FinishedAt: &now,
	})
	logger.Warn("error ignored (error_policy=ignore)", "error", execErr)

	e.bus.Publish(e.sensitive.MaskEvent(event.Event{
		Type:        event.StepCompleted,
		Timestamp:   time.Now(),
		ExecutionID: execCtx.ExecutionID,
		StepID:      step.ID,
		Message:     fmt.Sprintf("step %q completed (error ignored)", step.ID),
		Data:        map[string]any{"output": suppressedOutput},
	}))

	return nil
}

func (e *Executor) recordStepSuccess(
	step parser.Step, output any, execCtx *runtime.ExecutionContext,
	stepStartedAt *time.Time, logger *slog.Logger,
) {
	completedAt := time.Now()

	execCtx.SetStepResult(step.ID, &runtime.StepResult{
		Status:     runtime.StatusSuccess,
		Output:     output,
		StartedAt:  stepStartedAt,
		FinishedAt: &completedAt,
	})

	e.emitLogActionOutput(step, output, execCtx)

	e.bus.Publish(e.sensitive.MaskEvent(event.Event{
		Type:        event.StepOutput,
		Timestamp:   time.Now(),
		ExecutionID: execCtx.ExecutionID,
		StepID:      step.ID,
		Message:     fmt.Sprintf("step %q output", step.ID),
		Data:        map[string]any{"output": output},
	}))

	e.bus.Publish(e.sensitive.MaskEvent(event.Event{
		Type:        event.StepCompleted,
		Timestamp:   time.Now(),
		ExecutionID: execCtx.ExecutionID,
		StepID:      step.ID,
		Message:     fmt.Sprintf("step %q completed", step.ID),
		Data:        map[string]any{"output": output},
	}))
	logger.Info("completed")
}

func (e *Executor) emitLogActionOutput(step parser.Step, output any, execCtx *runtime.ExecutionContext) {
	outMap, ok := output.(map[string]any)
	if !ok {
		return
	}

	var logMsg, logLevel string

	switch step.Action {
	case "log":
		logMsg, _ = outMap["message"].(string)
		logLevel, _ = outMap["level"].(string)
	default:
		return
	}

	stream, _ := step.Config["stream"].(bool)

	for _, line := range strings.Split(strings.TrimRight(logMsg, "\n"), "\n") {
		data := map[string]any{"level": logLevel}
		if stream {
			data["stream"] = true
		}

		e.bus.Publish(event.Event{
			Type:        event.StepLog,
			Timestamp:   time.Now(),
			ExecutionID: execCtx.ExecutionID,
			StepID:      step.ID,
			Message:     line,
			Data:        data,
		})
	}
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

			continue
		}

		execCtx.SetStepResult(errStep.ID, &runtime.StepResult{Status: runtime.StatusSuccess, Output: output})
	}
}

func stepErrorCode(err error) string {
	if err == nil {
		return ""
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}

	if errors.Is(err, context.Canceled) {
		return "canceled"
	}

	return "action_failed"
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

func findTestCase(step parser.Step, caseName string) *parser.TestCase {
	for i := range step.Testing {
		if step.Testing[i].Name == caseName {
			return &step.Testing[i]
		}
	}

	return nil
}

func (e *Executor) applyTestMock(step parser.Step, tc *parser.TestCase, execCtx *runtime.ExecutionContext, logger *slog.Logger) error {
	now := time.Now()

	if tc.Error != nil {
		code := tc.Error.Code
		if code == "" {
			code = "action_failed"
		}

		execCtx.SetStepResult(step.ID, &runtime.StepResult{
			Status:     runtime.StatusFailed,
			Error:      &runtime.StepError{Message: tc.Error.Message, Code: code, StepID: step.ID},
			StartedAt:  &now,
			FinishedAt: &now,
		})

		e.bus.Publish(event.Event{
			Type: event.StepFailed, ExecutionID: execCtx.ExecutionID,
			StepID: step.ID, Message: tc.Error.Message, Timestamp: now,
		})

		if step.ErrorPolicy == "continue" || step.ErrorPolicy == "ignore" {
			return nil
		}

		return errors.New(tc.Error.Message)
	}

	e.recordStepSuccess(step, tc.Output, execCtx, &now, logger)

	return nil
}

func (e *Executor) CheckTestExpectExposed(step parser.Step, expect *parser.TestCaseExpect, execCtx *runtime.ExecutionContext) error {
	return e.checkTestExpect(step, expect, execCtx)
}

func (e *Executor) checkTestExpect(step parser.Step, expect *parser.TestCaseExpect, execCtx *runtime.ExecutionContext) error {
	result, ok := execCtx.GetStepResult(step.ID)
	if !ok {
		return fmt.Errorf("test expect: step %q has no result", step.ID)
	}

	if expect.Status != "" && result.Status != expect.Status {
		return fmt.Errorf("test expect: step %q status = %q, want %q", step.ID, result.Status, expect.Status)
	}

	if expect.Output != nil {
		if err := partialMatch(step.ID, "output", expect.Output, result.Output); err != nil {
			return err
		}
	}

	if expect.Error != nil {
		if result.Error == nil {
			return fmt.Errorf("test expect: step %q expected error but got none", step.ID)
		}

		if expect.Error.Message != "" && result.Error.Message != expect.Error.Message {
			return fmt.Errorf("test expect: step %q error message = %q, want %q", step.ID, result.Error.Message, expect.Error.Message)
		}

		if expect.Error.Code != "" && result.Error.Code != expect.Error.Code {
			return fmt.Errorf("test expect: step %q error code = %q, want %q", step.ID, result.Error.Code, expect.Error.Code)
		}
	}

	return nil
}

func partialMatch(stepID, path string, expected, actual any) error {
	switch exp := expected.(type) {
	case map[string]any:
		actMap, ok := actual.(map[string]any)
		if !ok {
			return fmt.Errorf("test expect: step %q %s expected map, got %T", stepID, path, actual)
		}

		for k, v := range exp {
			av, ok := actMap[k]
			if !ok {
				return fmt.Errorf("test expect: step %q %s.%s missing", stepID, path, k)
			}

			if err := partialMatch(stepID, path+"."+k, v, av); err != nil {
				return err
			}
		}

	case []any:
		actSlice, ok := actual.([]any)
		if !ok {
			return fmt.Errorf("test expect: step %q %s expected slice, got %T", stepID, path, actual)
		}

		if len(exp) != len(actSlice) {
			return fmt.Errorf("test expect: step %q %s length = %d, want %d", stepID, path, len(actSlice), len(exp))
		}

		for i := range exp {
			if err := partialMatch(stepID, fmt.Sprintf("%s[%d]", path, i), exp[i], actSlice[i]); err != nil {
				return err
			}
		}

	default:
		if !scalarEqual(expected, actual) {
			return fmt.Errorf("test expect: step %q %s = %v, want %v", stepID, path, actual, expected)
		}
	}

	return nil
}

func scalarEqual(a, b any) bool {
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}
