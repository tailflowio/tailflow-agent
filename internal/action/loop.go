package action

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// LoopAction iterates over a list.
type LoopAction struct{}

func NewLoopAction() Action { return &LoopAction{} }

func (a *LoopAction) Validate(ctx *ActionContext) error {
	if _, ok := ctx.Config["items"]; !ok {
		return errors.New("loop action requires 'items' in config")
	}

	return nil
}

func (a *LoopAction) Execute(ctx *ActionContext) (any, error) {
	items, ok := ctx.Config["items"].([]any)
	if !ok {
		return nil, errors.New("loop action: 'items' must be an array")
	}

	varName := "item"
	if as, ok := ctx.Config["as"]; ok {
		varName = fmt.Sprintf("%v", as)
	}

	indexName := "index"
	if idx, ok := ctx.Config["index"]; ok {
		indexName = fmt.Sprintf("%v", idx)
	}

	concurrency := 1
	if c, ok := toInt(ctx.Config["concurrency"]); ok && c > 0 {
		concurrency = c
	}

	// Check for pipeline actions array first
	if rawActions, ok := ctx.Config["actions"]; ok {
		pipeline, err := parsePipelineActions(rawActions)
		if err != nil {
			return nil, fmt.Errorf("loop action: %w", err)
		}

		if ctx.RunAction == nil {
			return nil, errors.New("loop action: RunAction callback not available")
		}

		return a.executePipeline(ctx, items, varName, indexName, concurrency, pipeline)
	}

	// Fallback: single action
	actionName, _ := ctx.Config["action"].(string)
	if actionName == "" {
		return a.executeLegacy(ctx, items, varName, indexName)
	}

	// action_config is kept raw (unresolved) by the engine so that {{ loop.* }}
	// templates survive until RunAction resolves them per iteration.
	actionConfig, _ := ctx.Config["action_config"].(map[string]any)

	if ctx.RunAction == nil {
		return nil, errors.New("loop action: RunAction callback not available")
	}

	sem := make(chan struct{}, concurrency)
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
				varName:   it,
				indexName: idx,
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

	errorPolicy, _ := ctx.Config["error_policy"].(string)

	var loopErrors []map[string]any
	for i, err := range errs {
		if err != nil {
			loopErrors = append(loopErrors, map[string]any{
				"index":   i,
				"item":    items[i],
				"message": err.Error(),
			})
		}
	}

	if len(loopErrors) > 0 && errorPolicy != "continue" {
		return nil, fmt.Errorf("loop iteration failed: %w", errs[firstErrIdx(errs)])
	}

	// Set last item vars for backward compat
	if len(items) > 0 {
		ctx.ExecCtx.SetVariable(varName, items[len(items)-1])
		ctx.ExecCtx.SetVariable(indexName, len(items)-1)
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

// pipelineStep represents a single action in a pipeline.
type pipelineStep struct {
	Action string
	Config map[string]any
}

// parsePipelineActions validates and parses the "actions" config value.
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

// executePipeline runs a pipeline of actions for each iteration with concurrency control.
func (a *LoopAction) executePipeline(
	ctx *ActionContext,
	items []any,
	varName, indexName string,
	concurrency int,
	pipeline []pipelineStep,
) (any, error) {
	type actionResult struct {
		Action     string `json:"action"`
		Output     any    `json:"output"`
		Error      string `json:"error,omitempty"`
		DurationMs int64  `json:"duration_ms"`
	}

	type iterationResult struct {
		Actions []actionResult `json:"actions"`
	}

	sem := make(chan struct{}, concurrency)
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
				varName:   it,
				indexName: idx,
			}

			var actionResults []actionResult

			for si, ps := range pipeline {
				if ctx.EmitLog != nil {
					ctx.EmitLog(fmt.Sprintf("[%d/%d] step %d/%d %s", idx+1, len(items), si+1, len(pipeline), ps.Action))
				}

				start := time.Now()
				output, err := ctx.RunAction(ps.Action, deepCopyMap(ps.Config), loopVars)
				durationMs := time.Since(start).Milliseconds()

				if err != nil {
					if ctx.EmitLog != nil {
						ctx.EmitLog(fmt.Sprintf("[%d/%d] step %d/%d %s FAILED (%dms): %s", idx+1, len(items), si+1, len(pipeline), ps.Action, durationMs, err.Error()))
					}

					errs[idx] = fmt.Errorf("action %q (step %d/%d): %w", ps.Action, si+1, len(pipeline), err)

					actionResults = append(actionResults, actionResult{
						Action:     ps.Action,
						Output:     output,
						Error:      err.Error(),
						DurationMs: durationMs,
					})

					break
				}

				if ctx.EmitLog != nil {
					ctx.EmitLog(fmt.Sprintf("[%d/%d] step %d/%d %s OK (%dms)", idx+1, len(items), si+1, len(pipeline), ps.Action, durationMs))
				}

				loopVars["prev"] = output

				actionResults = append(actionResults, actionResult{
					Action:     ps.Action,
					Output:     output,
					DurationMs: durationMs,
				})
			}

			results[idx] = iterationResult{Actions: actionResults}
		}(i, item)
	}

	wg.Wait()

	errorPolicy, _ := ctx.Config["error_policy"].(string)

	var loopErrors []map[string]any
	for i, err := range errs {
		if err != nil {
			loopErrors = append(loopErrors, map[string]any{
				"index":   i,
				"item":    items[i],
				"message": err.Error(),
			})
		}
	}

	if len(loopErrors) > 0 && errorPolicy != "continue" {
		return nil, fmt.Errorf("loop iteration failed: %w", errs[firstErrIdx(errs)])
	}

	// Set last item vars for backward compat
	if len(items) > 0 {
		ctx.ExecCtx.SetVariable(varName, items[len(items)-1])
		ctx.ExecCtx.SetVariable(indexName, len(items)-1)
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

// executeLegacy preserves the original loop behaviour: set variables for each item.
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

// firstErrIdx returns the index of the first non-nil error.
func firstErrIdx(errs []error) int {
	for i, e := range errs {
		if e != nil {
			return i
		}
	}
	return 0
}

// toInt converts a config value to int.
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

// deepCopyMap creates a shallow copy of a map so template resolution doesn't mutate the original.
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
