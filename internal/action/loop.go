package action

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type LoopAction struct{}

func NewLoopAction() Action { return &LoopAction{} }

func (a *LoopAction) Validate(ctx *ActionContext) error {
	_, ok := ctx.Config["items"]
	if !ok {
		return errors.New("loop action requires 'items' in config")
	}

	return nil
}

func (a *LoopAction) Execute(ctx *ActionContext) (any, error) {
	items, ok := toAnySlice(ctx.Config["items"])
	if !ok {
		return nil, errors.New("loop action: 'items' must be an array")
	}

	lp := parseLoopParams(ctx)

	rawActions, ok := ctx.Config["actions"]
	if ok {
		pipeline, err := parsePipelineActions(rawActions)
		if err != nil {
			return nil, fmt.Errorf("loop action: %w", err)
		}

		if ctx.RunAction == nil {
			return nil, errors.New("loop action: RunAction callback not available")
		}

		return a.executePipeline(ctx, items, lp, pipeline)
	}

	actionName, _ := ctx.Config["action"].(string)
	if actionName == "" {
		return a.executeLegacy(ctx, items, lp.varName, lp.indexName)
	}

	if ctx.RunAction == nil {
		return nil, errors.New("loop action: RunAction callback not available")
	}

	actionConfig, _ := ctx.Config["action_config"].(map[string]any)

	return a.executeSingleAction(ctx, items, lp, actionName, actionConfig)
}

type loopParams struct {
	varName     string
	indexName   string
	concurrency int
}

func parseLoopParams(ctx *ActionContext) loopParams {
	lp := loopParams{
		varName:     "item",
		indexName:   "index",
		concurrency: 1,
	}

	as, ok := ctx.Config["as"]
	if ok {
		lp.varName = fmt.Sprintf("%v", as)
	}

	idx, ok := ctx.Config["index"]
	if ok {
		lp.indexName = fmt.Sprintf("%v", idx)
	}

	c, ok := toInt(ctx.Config["concurrency"])
	if ok && c > 0 {
		lp.concurrency = c
	}

	return lp
}

func (a *LoopAction) executeSingleAction(
	ctx *ActionContext,
	items []any,
	lp loopParams,
	actionName string,
	actionConfig map[string]any,
) (any, error) {
	sem := make(chan struct{}, lp.concurrency)
	results := make([]any, len(items))
	errs := make([]error, len(items))
	var wg sync.WaitGroup

	for i, item := range items {
		sem <- struct{}{}

		wg.Add(1)

		go func(idx int, it any) {
			defer wg.Done()
			defer func() { <-sem }()

			loopVars := map[string]any{
				lp.varName:   it,
				lp.indexName: idx,
			}

			if ctx.EmitLog != nil {
				ctx.EmitLog(fmt.Sprintf("[%d/%d]", idx+1, len(items)))
			}

			output, err := ctx.RunAction(actionName, deepCopyMap(actionConfig), loopVars)
			results[idx] = output
			errs[idx] = err
		}(i, item)
	}

	wg.Wait()

	setLastIterationVars(ctx, items, lp.varName, lp.indexName)

	return buildLoopOutput(ctx, items, results, errs)
}

type pipelineStep struct {
	Action string
	Config map[string]any
}

func parsePipelineActions(raw any) ([]pipelineStep, error) {
	arr, ok := raw.([]any)
	if !ok {
		return nil, errors.New("'actions' must be an array")
	}

	if len(arr) == 0 {
		return nil, errors.New("'actions' array must not be empty")
	}

	steps := make([]pipelineStep, 0, len(arr))

	for i, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("actions[%d]: must be an object", i)
		}

		actionName, ok := m["action"].(string)
		if !ok || actionName == "" {
			return nil, fmt.Errorf("actions[%d]: missing or invalid 'action' field", i)
		}

		config, _ := m["config"].(map[string]any)

		steps = append(steps, pipelineStep{
			Action: actionName,
			Config: config,
		})
	}

	return steps, nil
}

type actionResult struct {
	Action     string `json:"action"`
	Output     any    `json:"output"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"duration_ms"`
}

type iterationResult struct {
	Actions []actionResult `json:"actions"`
}

func (a *LoopAction) executePipeline(
	ctx *ActionContext,
	items []any,
	lp loopParams,
	pipeline []pipelineStep,
) (any, error) {
	sem := make(chan struct{}, lp.concurrency)
	results := make([]iterationResult, len(items))
	errs := make([]error, len(items))
	var wg sync.WaitGroup

	for i, item := range items {
		sem <- struct{}{}

		wg.Add(1)

		go func(idx int, it any) {
			defer wg.Done()
			defer func() { <-sem }()

			loopVars := map[string]any{
				lp.varName:   it,
				lp.indexName: idx,
			}

			actionResults, err := runPipelineSteps(ctx, pipeline, loopVars, idx, len(items))
			results[idx] = iterationResult{Actions: actionResults}
			errs[idx] = err
		}(i, item)
	}

	wg.Wait()

	setLastIterationVars(ctx, items, lp.varName, lp.indexName)

	return buildLoopOutput(ctx, items, results, errs)
}

func runPipelineSteps(
	ctx *ActionContext,
	pipeline []pipelineStep,
	loopVars map[string]any,
	idx, total int,
) ([]actionResult, error) {
	results := make([]actionResult, 0, len(pipeline))
	label := loopItemLabel(loopVars)

	for si, ps := range pipeline {
		emitStepStart(ctx, idx, total, si, len(pipeline), ps.Action, label)

		start := time.Now()
		output, err := ctx.RunAction(ps.Action, deepCopyMap(ps.Config), loopVars)
		durationMs := time.Since(start).Milliseconds()

		if err != nil {
			return appendFailedStep(ctx, results, ps, output, durationMs, err, idx, total, si, len(pipeline), label)
		}

		emitStepSuccess(ctx, idx, total, si, len(pipeline), ps.Action, label, durationMs)

		loopVars["prev"] = output

		results = append(results, actionResult{
			Action:     ps.Action,
			Output:     output,
			DurationMs: durationMs,
		})
	}

	return results, nil
}

func appendFailedStep(
	ctx *ActionContext,
	results []actionResult,
	ps pipelineStep,
	output any,
	durationMs int64,
	err error,
	idx, total, si, pipelineLen int,
	label string,
) ([]actionResult, error) {
	if ctx.EmitLog != nil {
		hint := lastStdoutLine(output)

		switch {
		case label != "" && hint != "":
			ctx.EmitLog(fmt.Sprintf("[%d/%d] %s FAILED: %s", idx+1, total, label, hint))
		case label != "":
			ctx.EmitLog(fmt.Sprintf("[%d/%d] %s FAILED (%dms)", idx+1, total, label, durationMs))
		default:
			ctx.EmitLog(fmt.Sprintf("[%d/%d] step %d/%d %s FAILED (%dms): %s",
				idx+1, total, si+1, pipelineLen, ps.Action, durationMs, err.Error()))
		}
	}

	results = append(results, actionResult{
		Action:     ps.Action,
		Output:     output,
		Error:      err.Error(),
		DurationMs: durationMs,
	})

	return results, fmt.Errorf("action %q (step %d/%d): %w", ps.Action, si+1, pipelineLen, err)
}

func setLastIterationVars(ctx *ActionContext, items []any, varName, indexName string) {
	if len(items) == 0 {
		return
	}

	ctx.ExecCtx.SetVariable(varName, items[len(items)-1])
	ctx.ExecCtx.SetVariable(indexName, len(items)-1)
}

func buildLoopOutput[T any](ctx *ActionContext, items []any, results []T, errs []error) (any, error) {
	errorPolicy, _ := ctx.Config["error_policy"].(string)

	var loopErrors []any

	for i, err := range errs {
		if err != nil {
			entry := map[string]any{
				"index":   i,
				"item":    items[i],
				"message": err.Error(),
			}

			detail := extractResultDetail(results[i])
			if detail != "" {
				entry["detail"] = detail
			}

			loopErrors = append(loopErrors, entry)
		}
	}

	if len(loopErrors) > 0 && errorPolicy != "continue" {
		return nil, fmt.Errorf("loop iteration failed: %w", errs[firstErrIdx(errs)])
	}

	output := map[string]any{
		"iterations": len(items),
		"items":      items,
		"results":    results,
	}
	if len(loopErrors) > 0 {
		output["errors"] = loopErrors
		output["failed"] = len(loopErrors)
		output["succeeded"] = len(items) - len(loopErrors)
	}

	return output, nil
}

func (a *LoopAction) executeLegacy(ctx *ActionContext, items []any, varName, indexName string) (any, error) {
	results := make([]any, 0, len(items))

	for i, item := range items {
		ctx.ExecCtx.SetVariable(varName, item)
		ctx.ExecCtx.SetVariable(indexName, i)

		results = append(results, item)
	}

	return map[string]any{
		"iterations": len(items),
		"items":      results,
	}, nil
}

func loopItemLabel(loopVars map[string]any) string {
	for k, v := range loopVars {
		if k == "index" || k == "prev" {
			continue
		}

		m, ok := v.(map[string]any)
		if ok {
			for _, field := range []string{"path", "name", "id", "title"} {
				s, found := m[field].(string)
				if found && s != "" {
					return s
				}
			}
		}

		s, ok := v.(string)
		if ok {
			return s
		}
	}

	return ""
}

func lastStdoutLine(output any) string {
	outMap, ok := output.(map[string]any)
	if !ok {
		return ""
	}

	stdout, _ := outMap["stdout"].(string)
	stdout = strings.TrimSpace(stdout)

	if stdout == "" {
		return ""
	}

	lines := strings.Split(stdout, "\n")

	return strings.TrimSpace(lines[len(lines)-1])
}

func extractResultDetail[T any](result T) string {
	ir, ok := any(result).(iterationResult)
	if !ok {
		return ""
	}

	for j := len(ir.Actions) - 1; j >= 0; j-- {
		outMap, ok := ir.Actions[j].Output.(map[string]any)
		if !ok {
			continue
		}

		stdout, _ := outMap["stdout"].(string)
		stdout = strings.TrimSpace(stdout)

		if stdout == "" {
			continue
		}

		lines := strings.Split(stdout, "\n")

		return strings.TrimSpace(lines[len(lines)-1])
	}

	return ""
}

func emitStepStart(ctx *ActionContext, idx, total, si, pipelineLen int, action, label string) {
	if ctx.EmitLog == nil {
		return
	}

	if label != "" {
		ctx.EmitLog(fmt.Sprintf("[%d/%d] %s", idx+1, total, label))
		return
	}

	ctx.EmitLog(fmt.Sprintf("[%d/%d] step %d/%d %s", idx+1, total, si+1, pipelineLen, action))
}

func emitStepSuccess(ctx *ActionContext, idx, total, si, pipelineLen int, action, label string, durationMs int64) {
	if ctx.EmitLog == nil {
		return
	}

	if label != "" {
		ctx.EmitLog(fmt.Sprintf("[%d/%d] %s OK (%dms)", idx+1, total, label, durationMs))
		return
	}

	ctx.EmitLog(fmt.Sprintf("[%d/%d] step %d/%d %s OK (%dms)", idx+1, total, si+1, pipelineLen, action, durationMs))
}

func firstErrIdx(errs []error) int {
	for i, e := range errs {
		if e != nil {
			return i
		}
	}

	return 0
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	case int64:
		return int(n), true
	default:
		return 0, false
	}
}

func deepCopyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}

	cp := make(map[string]any, len(m))

	for k, v := range m {
		switch val := v.(type) {
		case map[string]any:
			cp[k] = deepCopyMap(val)
		case []any:
			sl := make([]any, len(val))
			copy(sl, val)
			cp[k] = sl
		default:
			cp[k] = v
		}
	}

	return cp
}
