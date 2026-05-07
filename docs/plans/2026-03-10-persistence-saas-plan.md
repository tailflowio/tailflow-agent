# Phase 1: Persistence, Recovery & Idempotency — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Enable workflow persistence via SaaS, crash recovery, step-level idempotency, and generic execution grouping — while keeping the agent stateless.

**Architecture:** The agent pushes enriched execution state to the SaaS (TiDB) via the existing exporter. On startup, it pulls incomplete executions and resumes them. Idempotency operates at two levels: workflow-level (dedup via key) and step-level (on_recovery strategy). `group_by` on params enables generic execution grouping.

**Tech Stack:** Go 1.26, testify/suite, mockery, existing exporter HTTP client, TiDB (SaaS-side)

**Design doc:** `docs/plans/2026-03-10-persistence-saas-design.md`

**Project rules:** `.claude/rules.md`, `.claude/conventions.md`

**Key conventions:**
- testify/suite mandatory, `SetupTest()` always present
- Mockery fakes in `internal/fake/fake{pkg}/`, `.EXPECT()` pattern
- Logger: `slog.New(slog.NewTextHandler(io.Discard, nil))`
- Zero comments unless non-obvious WHY
- Early returns, no `if err := ...; err != nil`
- `make lint && make test` before every commit
- 100% coverage via `make test-coverage`
- `synctest` for async test assertions (Go 1.26 native)
- Test naming: `Test<Feature>_<Scenario>` on suite methods

---

### Task 1: Add `group_by` field to Param struct

**Files:**
- Modify: `internal/parser/schema.go:22-28`
- Test: `internal/parser/parser_test.go`

**Step 1: Write the failing test**

Add to `internal/parser/parser_test.go` on the `ParserTestSuite`:

```go
func (s *ParserTestSuite) TestParseBytes_GroupByParam() {
	yaml := `
version: "2.0"
name: test-group-by
params:
  - name: customer_id
    type: string
    required: true
    group_by: true
  - name: note
    type: string
steps:
  - id: step1
    action: log
    config:
      message: hello
`
	wf, err := ParseBytes([]byte(yaml))
	s.Require().NoError(err)

	s.True(wf.Params[0].GroupBy)
	s.False(wf.Params[1].GroupBy)
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && go test ./internal/parser/ -run "TestParser/TestParseBytes_GroupByParam" -v`
Expected: FAIL — `GroupBy` field does not exist on Param

**Step 3: Write minimal implementation**

In `internal/parser/schema.go`, add `GroupBy` to the Param struct:

```go
type Param struct {
	Name     string `json:"name"               yaml:"name"`
	Type     string `json:"type"               yaml:"type"`
	Required bool   `json:"required,omitempty" yaml:"required,omitempty"`
	Default  any    `json:"default,omitempty"  yaml:"default,omitempty"`
	Pattern  string `json:"pattern,omitempty"  yaml:"pattern,omitempty"`
	GroupBy  bool   `json:"group_by,omitempty" yaml:"group_by,omitempty"`
}
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && go test ./internal/parser/ -run "TestParser/TestParseBytes_GroupByParam" -v`
Expected: PASS

**Step 5: Validate**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && make lint && make test`
Expected: All PASS, zero warnings

**Step 6: Commit**

```bash
git add internal/parser/schema.go internal/parser/parser_test.go
git commit -m "feat(parser): add group_by field to Param struct"
```

---

### Task 2: Add `on_recovery` field to Step struct

**Files:**
- Modify: `internal/parser/schema.go:38-51`
- Modify: `internal/parser/parser.go` (validateSteps)
- Test: `internal/parser/parser_test.go`

**Step 1: Write the failing tests**

Add to `internal/parser/parser_test.go`:

```go
func (s *ParserTestSuite) TestParseBytes_OnRecoveryValues() {
	yaml := `
version: "2.0"
name: test-on-recovery
steps:
  - id: step-retry
    action: http
    on_recovery: retry
    config:
      url: http://example.com
  - id: step-skip
    action: http
    on_recovery: skip
    depends_on: [step-retry]
    config:
      url: http://example.com
  - id: step-fail
    action: http
    on_recovery: fail
    depends_on: [step-skip]
    config:
      url: http://example.com
  - id: step-default
    action: log
    depends_on: [step-fail]
    config:
      message: hello
`
	wf, err := ParseBytes([]byte(yaml))
	s.Require().NoError(err)

	s.Equal("retry", wf.Steps[0].OnRecovery)
	s.Equal("skip", wf.Steps[1].OnRecovery)
	s.Equal("fail", wf.Steps[2].OnRecovery)
	s.Empty(wf.Steps[3].OnRecovery)
}

func (s *ParserTestSuite) TestParseBytes_OnRecoveryInvalidValue() {
	yaml := `
version: "2.0"
name: test-invalid-recovery
steps:
  - id: step1
    action: http
    on_recovery: explode
    config:
      url: http://example.com
`
	_, err := ParseBytes([]byte(yaml))
	s.Error(err)
	s.Contains(err.Error(), "on_recovery")
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && go test ./internal/parser/ -run "TestParser/TestParseBytes_OnRecovery" -v`
Expected: FAIL

**Step 3: Write minimal implementation**

In `internal/parser/schema.go`, add `OnRecovery` to the Step struct:

```go
OnRecovery string `json:"on_recovery,omitempty" yaml:"on_recovery,omitempty"`
```

In `internal/parser/parser.go`, add validation in `validateSteps()` after the existing error_policy check:

```go
if s.OnRecovery != "" && s.OnRecovery != "retry" && s.OnRecovery != "skip" && s.OnRecovery != "fail" {
	return fmt.Errorf("step %q: on_recovery must be retry, skip, or fail", s.ID)
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && go test ./internal/parser/ -run "TestParser/TestParseBytes_OnRecovery" -v`
Expected: Both PASS

**Step 5: Validate**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && make lint && make test`
Expected: All PASS

**Step 6: Commit**

```bash
git add internal/parser/schema.go internal/parser/parser.go internal/parser/parser_test.go
git commit -m "feat(parser): add on_recovery field to Step (retry/skip/fail)"
```

---

### Task 3: Add `idempotency_key` field to HTTPTrigger

**Files:**
- Modify: `internal/parser/schema.go:99-103`
- Test: `internal/parser/parser_test.go`

**Step 1: Write the failing tests**

Add to `internal/parser/parser_test.go`:

```go
func (s *ParserTestSuite) TestParseBytes_IdempotencyKey() {
	yaml := `
version: "2.0"
name: test-idempotency
trigger:
  http:
    method: POST
    path: /api/test
    idempotency_key: "{{ trigger.body.customer_id }}"
steps:
  - id: step1
    action: log
    config:
      message: hello
`
	wf, err := ParseBytes([]byte(yaml))
	s.Require().NoError(err)
	s.Equal("{{ trigger.body.customer_id }}", wf.Trigger.HTTP.IdempotencyKey)
}

func (s *ParserTestSuite) TestParseBytes_IdempotencyKeyOptional() {
	yaml := `
version: "2.0"
name: test-no-idempotency
trigger:
  http:
    method: POST
    path: /api/test
steps:
  - id: step1
    action: log
    config:
      message: hello
`
	wf, err := ParseBytes([]byte(yaml))
	s.Require().NoError(err)
	s.Empty(wf.Trigger.HTTP.IdempotencyKey)
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && go test ./internal/parser/ -run "TestParser/TestParseBytes_IdempotencyKey" -v`
Expected: FAIL

**Step 3: Write minimal implementation**

In `internal/parser/schema.go`, add to HTTPTrigger:

```go
type HTTPTrigger struct {
	Method         string `json:"method"                    yaml:"method"`
	Path           string `json:"path"                      yaml:"path"`
	Async          bool   `json:"async,omitempty"           yaml:"async,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty" yaml:"idempotency_key,omitempty"`
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && go test ./internal/parser/ -run "TestParser/TestParseBytes_IdempotencyKey" -v`
Expected: PASS

**Step 5: Validate**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && make lint && make test`
Expected: All PASS

**Step 6: Commit**

```bash
git add internal/parser/schema.go internal/parser/parser_test.go
git commit -m "feat(parser): add idempotency_key to HTTPTrigger"
```

---

### Task 4: Add recovery fields to ExecutionContext and ExecuteOptions

**Files:**
- Modify: `internal/runtime/context.go:27-38`
- Modify: `internal/engine/engine.go:63-68` (ExecuteOptions)
- Test: `internal/runtime/context_test.go` (add to existing suite or create)

**Step 1: Check existing context test file**

Read `internal/runtime/context_test.go` to find the existing suite name and pattern.

**Step 2: Write the failing test**

Add to the existing runtime test suite (or create `ExecutionContextTestSuite`):

```go
func (s *ExecutionContextTestSuite) TestNewExecutionContext_DefaultRecoveryFields() {
	ctx := NewExecutionContext("exec-1", "test-wf", nil, nil)

	s.False(ctx.Resumed)
	s.Empty(ctx.IdempotencyKey)
}

func (s *ExecutionContextTestSuite) TestExecutionContext_SetRecoveryFields() {
	ctx := NewExecutionContext("exec-1", "test-wf", nil, nil)

	ctx.Resumed = true
	ctx.IdempotencyKey = "user-42"

	s.True(ctx.Resumed)
	s.Equal("user-42", ctx.IdempotencyKey)
}
```

**Step 3: Run tests to verify they fail**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && go test ./internal/runtime/ -run "TestExecutionContext" -v`
Expected: FAIL

**Step 4: Write minimal implementation**

In `internal/runtime/context.go`, add fields to ExecutionContext:

```go
Resumed        bool
IdempotencyKey string
```

In `internal/engine/engine.go`, add fields to ExecuteOptions:

```go
type ExecuteOptions struct {
	ExecutionID    string
	TriggerData    map[string]any
	Services       *runtime.ActionServices
	TestCaseName   string
	Resumed        bool
	RecoveredSteps map[string]*runtime.StepResult
}
```

**Step 5: Run tests to verify they pass**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && go test ./internal/runtime/ -run "TestExecutionContext" -v`
Expected: PASS

**Step 6: Validate**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && make lint && make test`
Expected: All PASS

**Step 7: Commit**

```bash
git add internal/runtime/context.go internal/runtime/context_test.go internal/engine/engine.go
git commit -m "feat: add Resumed and IdempotencyKey to ExecutionContext and ExecuteOptions"
```

---

### Task 5: Implement recovery logic in engine (skip completed, apply on_recovery)

**Files:**
- Modify: `internal/engine/engine.go` (Execute and executeNode functions)
- Test: `internal/engine/engine_test.go`

**Step 1: Write the failing test — skip completed steps**

Add to `EngineTestSuite` in `internal/engine/engine_test.go`:

```go
func (s *EngineTestSuite) TestExecute_RecoverySkipsCompletedSteps() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	var executedSteps []string
	bus.Subscribe(func(ev event.Event) {
		if ev.Type == "step.started" {
			executedSteps = append(executedSteps, ev.StepID)
		}
	})

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-recovery",
		Steps: []parser.Step{
			{ID: "step1", Action: "log", Config: map[string]any{"message": "hello"}},
			{ID: "step2", Action: "log", DependsOn: []string{"step1"}, Config: map[string]any{"message": "world"}},
			{ID: "step3", Action: "log", DependsOn: []string{"step2"}, Config: map[string]any{"message": "!"}},
		},
	}

	now := time.Now()
	recoveredSteps := map[string]*runtime.StepResult{
		"step1": {
			Status:     runtime.StatusSuccess,
			StartedAt:  &now,
			FinishedAt: &now,
			Output:     map[string]any{"message": "hello"},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{
		Resumed:        true,
		RecoveredSteps: recoveredSteps,
	})
	s.Require().NoError(err)
	s.Equal(runtime.StatusSuccess, result.Status)

	for _, stepID := range executedSteps {
		s.NotEqual("step1", stepID, "step1 should have been skipped")
	}

	s.Contains(executedSteps, "step2")
	s.Contains(executedSteps, "step3")
}
```

**Step 2: Write the failing test — on_recovery skip**

```go
func (s *EngineTestSuite) TestExecute_RecoveryOnRecoverySkip() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	var executedSteps []string
	bus.Subscribe(func(ev event.Event) {
		if ev.Type == "step.started" {
			executedSteps = append(executedSteps, ev.StepID)
		}
	})

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-recovery-skip",
		Steps: []parser.Step{
			{ID: "send-email", Action: "log", OnRecovery: "skip", Config: map[string]any{"message": "email"}},
			{ID: "next-step", Action: "log", DependsOn: []string{"send-email"}, Config: map[string]any{"message": "next"}},
		},
	}

	now := time.Now()
	recoveredSteps := map[string]*runtime.StepResult{
		"send-email": {
			Status:    runtime.StatusRunning,
			StartedAt: &now,
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{
		Resumed:        true,
		RecoveredSteps: recoveredSteps,
	})
	s.Require().NoError(err)
	s.Equal(runtime.StatusSuccess, result.Status)

	for _, stepID := range executedSteps {
		s.NotEqual("send-email", stepID, "send-email should have been skipped (on_recovery=skip)")
	}

	s.Contains(executedSteps, "next-step")
}
```

**Step 3: Write the failing test — on_recovery fail**

```go
func (s *EngineTestSuite) TestExecute_RecoveryOnRecoveryFail() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-recovery-fail",
		Steps: []parser.Step{
			{ID: "payment", Action: "log", OnRecovery: "fail", Config: map[string]any{"message": "pay"}},
			{ID: "next", Action: "log", DependsOn: []string{"payment"}, Config: map[string]any{"message": "next"}},
		},
	}

	now := time.Now()
	recoveredSteps := map[string]*runtime.StepResult{
		"payment": {
			Status:    runtime.StatusRunning,
			StartedAt: &now,
		},
	}

	result, _ := exec.Execute(context.Background(), wf, nil, ExecuteOptions{
		Resumed:        true,
		RecoveredSteps: recoveredSteps,
	})
	s.NotEqual(runtime.StatusSuccess, result.Status)
}
```

**Step 4: Write the failing test — on_recovery retry (default)**

```go
func (s *EngineTestSuite) TestExecute_RecoveryOnRecoveryRetryDefault() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	var executedSteps []string
	bus.Subscribe(func(ev event.Event) {
		if ev.Type == "step.started" {
			executedSteps = append(executedSteps, ev.StepID)
		}
	})

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-recovery-retry",
		Steps: []parser.Step{
			{ID: "create-account", Action: "log", Config: map[string]any{"message": "create"}},
			{ID: "next", Action: "log", DependsOn: []string{"create-account"}, Config: map[string]any{"message": "next"}},
		},
	}

	now := time.Now()
	recoveredSteps := map[string]*runtime.StepResult{
		"create-account": {
			Status:    runtime.StatusRunning,
			StartedAt: &now,
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{
		Resumed:        true,
		RecoveredSteps: recoveredSteps,
	})
	s.Require().NoError(err)
	s.Equal(runtime.StatusSuccess, result.Status)
	s.Contains(executedSteps, "create-account", "should be re-executed with default retry strategy")
}
```

**Step 5: Run tests to verify they fail**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && go test ./internal/engine/ -run "TestEngine/TestExecute_Recovery" -v`
Expected: FAIL

**Step 6: Write minimal implementation**

In `Execute()` function, after `buildExecutionContext()`, inject recovered steps:

```go
if len(opts) > 0 && opts[0].Resumed {
	execCtx.Resumed = true
	for stepID, sr := range opts[0].RecoveredSteps {
		execCtx.SetStepResult(stepID, sr)
	}
}
```

In `executeNode()`, at the start before existing logic, add recovery check:

```go
func (e *Executor) handleRecovery(ectx *runtime.ExecutionContext, step *parser.Step) (skip bool, err error) {
	if !ectx.Resumed {
		return false, nil
	}

	sr := ectx.GetStepResult(step.ID)
	if sr == nil {
		return false, nil
	}

	if sr.Status == runtime.StatusSuccess || sr.Status == runtime.StatusSkipped {
		return true, nil
	}

	if sr.Status != runtime.StatusRunning {
		return false, nil
	}

	strategy := step.OnRecovery
	if strategy == "" {
		strategy = "retry"
	}

	now := time.Now()

	switch strategy {
	case "skip":
		ectx.SetStepResult(step.ID, &runtime.StepResult{
			Status:     runtime.StatusSuccess,
			StartedAt:  sr.StartedAt,
			FinishedAt: &now,
		})
		return true, nil
	case "fail":
		ectx.SetStepResult(step.ID, &runtime.StepResult{
			Status:     runtime.StatusFailed,
			StartedAt:  sr.StartedAt,
			FinishedAt: &now,
			Error: &runtime.StepError{
				Message: "step was running at crash time and on_recovery=fail",
				Code:    "recovery_failed",
				StepID:  step.ID,
			},
		})
		return true, fmt.Errorf("step %q: recovery failed, manual intervention required", step.ID)
	default:
		ectx.SetStepResult(step.ID, nil)
		return false, nil
	}
}
```

Call `handleRecovery()` at the start of `executeNode()`:

```go
skip, recoveryErr := e.handleRecovery(ectx, &node.Step)
if skip {
	return recoveryErr
}
```

**Step 7: Run tests to verify they pass**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && go test ./internal/engine/ -run "TestEngine/TestExecute_Recovery" -v`
Expected: All 4 PASS

**Step 8: Validate**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && make lint && make test`
Expected: All PASS

**Step 9: Commit**

```bash
git add internal/engine/engine.go internal/engine/engine_test.go
git commit -m "feat(engine): implement recovery logic with on_recovery strategies"
```

---

### Task 6: Emit execution.state events with group_params

**Files:**
- Modify: `internal/engine/engine.go` (Execute function, after DAG execution)
- Test: `internal/engine/engine_test.go`

**Step 1: Write the failing test**

Add to `EngineTestSuite`:

```go
func (s *EngineTestSuite) TestExecute_EmitsExecutionStateWithGroupParams() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	var stateEvents []event.Event
	bus.Subscribe(func(ev event.Event) {
		if ev.Type == "execution.state" {
			stateEvents = append(stateEvents, ev)
		}
	})

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-state-events",
		Params: []parser.Param{
			{Name: "customer_id", Type: "string", GroupBy: true},
			{Name: "note", Type: "string"},
		},
		Steps: []parser.Step{
			{ID: "step1", Action: "log", Config: map[string]any{"message": "hello"}},
		},
	}

	result, err := exec.Execute(context.Background(), wf, map[string]any{
		"customer_id": "user-42",
		"note":        "test",
	})
	s.Require().NoError(err)
	s.Equal(runtime.StatusSuccess, result.Status)
	s.NotEmpty(stateEvents)

	lastEvt := stateEvents[len(stateEvents)-1]
	data, ok := lastEvt.Data.(map[string]any)
	s.Require().True(ok)

	groupParams, ok := data["group_params"].(map[string]any)
	s.Require().True(ok)
	s.Equal("user-42", groupParams["customer_id"])
	_, hasNote := groupParams["note"]
	s.False(hasNote, "non-group_by param should not be in group_params")
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && go test ./internal/engine/ -run "TestEngine/TestExecute_EmitsExecutionState" -v`
Expected: FAIL

**Step 3: Write minimal implementation**

In `Execute()`, after `executeDAG()` returns and before building the result, emit the state event:

```go
groupParams := make(map[string]any)
for _, p := range wf.Params {
	if !p.GroupBy {
		continue
	}
	v, ok := execCtx.Params[p.Name]
	if !ok {
		continue
	}
	groupParams[p.Name] = v
}

stepsSnapshot := make(map[string]any, len(execCtx.Steps))
for stepID := range execCtx.Steps {
	sr := execCtx.GetStepResult(stepID)
	if sr == nil {
		continue
	}
	stepsSnapshot[stepID] = map[string]any{
		"status":      sr.Status,
		"started_at":  sr.StartedAt,
		"finished_at": sr.FinishedAt,
	}
}

e.bus.Publish(event.Event{
	Type:        "execution.state",
	ExecutionID: execCtx.ExecutionID,
	Data: map[string]any{
		"workflow_name": wf.Name,
		"status":        result.Status,
		"steps":         stepsSnapshot,
		"group_params":  groupParams,
	},
})
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && go test ./internal/engine/ -run "TestEngine/TestExecute_EmitsExecutionState" -v`
Expected: PASS

**Step 5: Validate**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && make lint && make test`
Expected: All PASS

**Step 6: Commit**

```bash
git add internal/engine/engine.go internal/engine/engine_test.go
git commit -m "feat(engine): emit execution.state events with group_params"
```

---

### Task 7: Resolve and emit idempotency_key

**Files:**
- Modify: `internal/engine/engine.go` (Execute function)
- Test: `internal/engine/engine_test.go`

**Step 1: Write the failing test**

Add to `EngineTestSuite`:

```go
func (s *EngineTestSuite) TestExecute_ResolvesIdempotencyKey() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	var capturedKey string
	bus.Subscribe(func(ev event.Event) {
		if ev.Type != "execution.state" {
			return
		}
		data, ok := ev.Data.(map[string]any)
		if !ok {
			return
		}
		key, _ := data["idempotency_key"].(string)
		capturedKey = key
	})

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-idemp",
		Trigger: &parser.Trigger{
			HTTP: &parser.HTTPTrigger{
				Method:         "POST",
				Path:           "/test",
				IdempotencyKey: "{{ params.customer_id }}",
			},
		},
		Params: []parser.Param{
			{Name: "customer_id", Type: "string"},
		},
		Steps: []parser.Step{
			{ID: "step1", Action: "log", Config: map[string]any{"message": "hello"}},
		},
	}

	_, err := exec.Execute(context.Background(), wf, map[string]any{
		"customer_id": "user-42",
	})
	s.Require().NoError(err)
	s.Equal("user-42", capturedKey)
}

func (s *EngineTestSuite) TestExecute_NoIdempotencyKeyWhenNotConfigured() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	var stateEvents []event.Event
	bus.Subscribe(func(ev event.Event) {
		if ev.Type == "execution.state" {
			stateEvents = append(stateEvents, ev)
		}
	})

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-no-idemp",
		Steps: []parser.Step{
			{ID: "step1", Action: "log", Config: map[string]any{"message": "hello"}},
		},
	}

	_, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.NotEmpty(stateEvents)

	data := stateEvents[0].Data.(map[string]any)
	s.Empty(data["idempotency_key"])
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && go test ./internal/engine/ -run "TestEngine/TestExecute_.*Idempotency" -v`
Expected: FAIL

**Step 3: Write minimal implementation**

In `Execute()`, after resolving params, resolve the idempotency key:

```go
var idempotencyKey string
if wf.Trigger != nil && wf.Trigger.HTTP != nil && wf.Trigger.HTTP.IdempotencyKey != "" {
	resolved, evalErr := e.eval.EvalString(wf.Trigger.HTTP.IdempotencyKey, execCtx.ToMap())
	if evalErr == nil {
		idempotencyKey = resolved
		execCtx.IdempotencyKey = resolved
	}
}
```

Then include `"idempotency_key": idempotencyKey` in the `execution.state` event data from Task 6.

**Step 4: Run tests to verify they pass**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && go test ./internal/engine/ -run "TestEngine/TestExecute_.*Idempotency" -v`
Expected: PASS

**Step 5: Validate**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && make lint && make test`
Expected: All PASS

**Step 6: Commit**

```bash
git add internal/engine/engine.go internal/engine/engine_test.go
git commit -m "feat(engine): resolve and emit idempotency_key in execution state"
```

---

### Task 8: Add RecoveryClient for pulling state from SaaS

**Files:**
- Create: `internal/export/recovery.go`
- Create: `internal/export/recovery_test.go`

**Step 1: Write the failing tests**

Create `internal/export/recovery_test.go`:

```go
package export

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/runtime"
)

type RecoveryClientTestSuite struct {
	suite.Suite
}

func TestRecoveryClient(t *testing.T) {
	suite.Run(t, new(RecoveryClientTestSuite))
}

func (s *RecoveryClientTestSuite) SetupTest() {}

func (s *RecoveryClientTestSuite) TestRecoverExecutions_ReturnsExecutions() {
	now := time.Now()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("/api/recovery", r.URL.Path)
		s.Equal("agent-1", r.URL.Query().Get("agent_id"))
		s.Equal("Bearer test-key", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]RecoveredExecution{
			{
				ExecutionID:  "exec-001",
				WorkflowName: "onboarding",
				Status:       runtime.StatusWaiting,
				Params:       map[string]any{"customer_id": "user-42"},
				Steps: map[string]*runtime.StepResult{
					"step1": {Status: runtime.StatusSuccess, StartedAt: &now, FinishedAt: &now},
					"step2": {Status: runtime.StatusWaiting, StartedAt: &now},
				},
			},
		})
	}))
	defer srv.Close()

	client := NewRecoveryClient(srv.URL, "test-key")
	execs, err := client.RecoverExecutions(context.Background(), "agent-1")
	s.Require().NoError(err)
	s.Len(execs, 1)
	s.Equal("exec-001", execs[0].ExecutionID)
	s.Equal("onboarding", execs[0].WorkflowName)
	s.Len(execs[0].Steps, 2)
}

func (s *RecoveryClientTestSuite) TestRecoverExecutions_SaaSUnreachable() {
	client := NewRecoveryClient("http://127.0.0.1:1", "key")

	execs, err := client.RecoverExecutions(context.Background(), "agent-1")
	s.NoError(err)
	s.Empty(execs)
}

func (s *RecoveryClientTestSuite) TestRecoverExecutions_SaaSReturnsError() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewRecoveryClient(srv.URL, "key")
	execs, err := client.RecoverExecutions(context.Background(), "agent-1")
	s.NoError(err)
	s.Empty(execs)
}

func (s *RecoveryClientTestSuite) TestRecoverExecutions_EmptyResponse() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]RecoveredExecution{})
	}))
	defer srv.Close()

	client := NewRecoveryClient(srv.URL, "key")
	execs, err := client.RecoverExecutions(context.Background(), "agent-1")
	s.NoError(err)
	s.Empty(execs)
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && go test ./internal/export/ -run "TestRecoveryClient" -v`
Expected: FAIL — types don't exist

**Step 3: Write minimal implementation**

Create `internal/export/recovery.go`:

```go
package export

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/tailflow/tailflow/internal/runtime"
)

type RecoveredExecution struct {
	ExecutionID  string                         `json:"execution_id"`
	WorkflowName string                         `json:"workflow_name"`
	Status       string                         `json:"status"`
	Params       map[string]any                 `json:"params,omitempty"`
	Steps        map[string]*runtime.StepResult `json:"steps,omitempty"`
}

type RecoveryClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewRecoveryClient(baseURL, apiKey string) *RecoveryClient {
	return &RecoveryClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (rc *RecoveryClient) RecoverExecutions(ctx context.Context, agentID string) ([]RecoveredExecution, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rc.baseURL+"/api/recovery?agent_id="+agentID, nil)
	if err != nil {
		return nil, nil
	}

	req.Header.Set("Authorization", "Bearer "+rc.apiKey)

	resp, err := rc.client.Do(req)
	if err != nil {
		return nil, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	var execs []RecoveredExecution
	err = json.NewDecoder(resp.Body).Decode(&execs)
	if err != nil {
		return nil, nil
	}

	return execs, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && go test ./internal/export/ -run "TestRecoveryClient" -v`
Expected: All PASS

**Step 5: Validate**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && make lint && make test`
Expected: All PASS

**Step 6: Commit**

```bash
git add internal/export/recovery.go internal/export/recovery_test.go
git commit -m "feat(export): add RecoveryClient for SaaS crash recovery"
```

---

### Task 9: Add ClaimClient for idempotency dedup via SaaS

**Files:**
- Create: `internal/export/claim.go`
- Create: `internal/export/claim_test.go`

**Step 1: Write the failing tests**

Create `internal/export/claim_test.go`:

```go
package export

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/suite"
)

type ClaimClientTestSuite struct {
	suite.Suite
}

func TestClaimClient(t *testing.T) {
	suite.Run(t, new(ClaimClientTestSuite))
}

func (s *ClaimClientTestSuite) SetupTest() {}

func (s *ClaimClientTestSuite) TestClaimExecution_NewExecution() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("/api/executions/claim", r.URL.Path)
		s.Equal("Bearer test-key", r.Header.Get("Authorization"))

		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		s.Equal("user-42", body["idempotency_key"])

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"claimed": true})
	}))
	defer srv.Close()

	client := NewClaimClient(srv.URL, "test-key")
	result, err := client.ClaimExecution(context.Background(), "exec-1", "onboarding", "user-42")
	s.Require().NoError(err)
	s.True(result.Claimed)
}

func (s *ClaimClientTestSuite) TestClaimExecution_Duplicate() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"claimed":      false,
			"execution_id": "exec-existing",
			"status":       "running",
		})
	}))
	defer srv.Close()

	client := NewClaimClient(srv.URL, "test-key")
	result, err := client.ClaimExecution(context.Background(), "exec-new", "onboarding", "user-42")
	s.Require().NoError(err)
	s.False(result.Claimed)
	s.Equal("exec-existing", result.ExistingExecutionID)
	s.Equal("running", result.ExistingStatus)
}

func (s *ClaimClientTestSuite) TestClaimExecution_SaaSUnreachable() {
	client := NewClaimClient("http://127.0.0.1:1", "key")
	result, err := client.ClaimExecution(context.Background(), "exec-1", "wf", "key-1")
	s.Require().NoError(err)
	s.True(result.Claimed, "should proceed without dedup when SaaS is down")
}

func (s *ClaimClientTestSuite) TestClaimExecution_SaaSReturnsError() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewClaimClient(srv.URL, "key")
	result, err := client.ClaimExecution(context.Background(), "exec-1", "wf", "key-1")
	s.Require().NoError(err)
	s.True(result.Claimed, "should proceed without dedup on server error")
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && go test ./internal/export/ -run "TestClaimClient" -v`
Expected: FAIL

**Step 3: Write minimal implementation**

Create `internal/export/claim.go`:

```go
package export

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type ClaimResult struct {
	Claimed             bool   `json:"claimed"`
	ExistingExecutionID string `json:"execution_id,omitempty"`
	ExistingStatus      string `json:"status,omitempty"`
}

type ClaimClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewClaimClient(baseURL, apiKey string) *ClaimClient {
	return &ClaimClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *ClaimClient) ClaimExecution(ctx context.Context, executionID, workflowName, idempotencyKey string) (*ClaimResult, error) {
	body, err := json.Marshal(map[string]any{
		"execution_id":    executionID,
		"workflow_name":   workflowName,
		"idempotency_key": idempotencyKey,
	})
	if err != nil {
		return &ClaimResult{Claimed: true}, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/executions/claim", bytes.NewReader(body))
	if err != nil {
		return &ClaimResult{Claimed: true}, nil
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return &ClaimResult{Claimed: true}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &ClaimResult{Claimed: true}, nil
	}

	var result ClaimResult
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return &ClaimResult{Claimed: true}, nil
	}

	return &result, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && go test ./internal/export/ -run "TestClaimClient" -v`
Expected: All PASS

**Step 5: Validate**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && make lint && make test`
Expected: All PASS

**Step 6: Commit**

```bash
git add internal/export/claim.go internal/export/claim_test.go
git commit -m "feat(export): add ClaimClient for idempotency dedup via SaaS"
```

---

### Task 10: Wire recovery into server startup

**Files:**
- Modify: `internal/server/server.go` (Run function)
- Test: `internal/server/server_test.go`

**Step 1: Read current server code**

Read `internal/server/server.go` lines 65-143 to understand the full Server struct and Run() function. Identify where recovery should be wired — after exporter starts, before cron/rabbitmq.

**Step 2: Write the failing test**

Add to server test suite. The test should verify that when ExportURL is configured, the recovery endpoint is called on startup. Use httptest.NewServer as a mock SaaS.

Follow the existing server test patterns (check `internal/server/server_test.go` for the suite structure).

**Step 3: Implement**

In `Run()`, after `s.exporter.Start(ctx)` and before cron scheduler:

```go
if s.config.ExportURL != "" {
	recoveryClient := export.NewRecoveryClient(s.config.ExportURL, s.config.APIKey)
	recovered, _ := recoveryClient.RecoverExecutions(ctx, s.config.AgentName)

	for _, exec := range recovered {
		wf := s.findWorkflowByName(exec.WorkflowName)
		if wf == nil {
			s.logger.Warn("recovery: workflow not found",
				"workflow", exec.WorkflowName,
				"execution", exec.ExecutionID,
			)
			continue
		}

		s.logger.Info("recovery: resuming execution",
			"execution", exec.ExecutionID,
			"workflow", exec.WorkflowName,
		)

		go s.runWorkflowAsync(ctx, wf, exec.Params, engine.ExecuteOptions{
			ExecutionID:    exec.ExecutionID,
			Resumed:        true,
			RecoveredSteps: exec.Steps,
		})
	}
}
```

Note: `findWorkflowByName` needs to be implemented — look up from the server's loaded workflows. Check how workflows are currently stored in the server struct.

**Step 4: Validate**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && make lint && make test`
Expected: All PASS

**Step 5: Commit**

```bash
git add internal/server/server.go internal/server/server_test.go
git commit -m "feat(server): wire SaaS recovery into server startup"
```

---

### Task 11: Wire idempotency claim into HTTP trigger handler

**Files:**
- Modify: `internal/server/handlers.go`
- Test: `internal/server/handlers_test.go` (or server_test.go)

**Step 1: Read current handler code**

Read `internal/server/handlers.go` to find the HTTP trigger handler. Understand where the execution is initiated — before `runWorkflowAsync()`.

**Step 2: Write the failing test**

Add to server test suite. Test that:
- First request with idempotency_key → claimed, workflow executes
- Second request with same key → not claimed, returns existing execution info
- Request with no idempotency_key → executes normally (no claim)

**Step 3: Implement**

In the trigger handler, before calling `runWorkflowAsync()`:

```go
if wf.Trigger != nil && wf.Trigger.HTTP != nil && wf.Trigger.HTTP.IdempotencyKey != "" && s.config.ExportURL != "" {
	keyTemplate := wf.Trigger.HTTP.IdempotencyKey
	resolvedKey, evalErr := s.resolveIdempotencyKey(keyTemplate, triggerData, resolvedParams)

	if evalErr == nil && resolvedKey != "" {
		claimClient := export.NewClaimClient(s.config.ExportURL, s.config.APIKey)
		claimResult, _ := claimClient.ClaimExecution(r.Context(), executionID, wf.Name, resolvedKey)

		if !claimResult.Claimed {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"execution_id": claimResult.ExistingExecutionID,
				"status":       claimResult.ExistingStatus,
				"deduplicated": true,
			})
			return
		}
	}
}
```

The `resolveIdempotencyKey` helper resolves the template using the expression evaluator with trigger data and params as context.

**Step 4: Validate**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && make lint && make test`
Expected: All PASS

**Step 5: Commit**

```bash
git add internal/server/handlers.go internal/server/handlers_test.go
git commit -m "feat(server): wire idempotency claim into HTTP trigger handler"
```

---

### Task 12: Add example workflow

**Files:**
- Create: `examples/recovery-demo.yaml`

**Step 1: Create example**

```yaml
version: "2.0"
name: customer-onboarding
description: "Onboarding workflow with persistence, recovery, and grouping"

params:
  - name: customer_id
    type: string
    required: true
    group_by: true
  - name: email
    type: string
    required: true
  - name: plan
    type: string
    default: "free"
    group_by: true

trigger:
  http:
    method: POST
    path: /api/onboard
    idempotency_key: "{{ trigger.body.customer_id }}"

steps:
  - id: validate
    action: js
    config:
      code: |
        if (!params.email.includes('@')) throw new Error('invalid email');
        return { valid: true };

  - id: send-welcome-email
    action: http
    depends_on: [validate]
    on_recovery: skip
    config:
      url: "https://api.email.com/send"
      method: POST
      body:
        to: "{{ params.email }}"
        template: welcome

  - id: create-account
    action: http
    depends_on: [validate]
    on_recovery: retry
    config:
      url: "https://api.accounts.com/users"
      method: PUT
      body:
        id: "{{ params.customer_id }}"
        email: "{{ params.email }}"
        plan: "{{ params.plan }}"

  - id: setup-billing
    action: http
    depends_on: [create-account]
    on_recovery: fail
    config:
      url: "https://api.billing.com/setup"
      method: POST
      body:
        customer_id: "{{ params.customer_id }}"
        plan: "{{ params.plan }}"

  - id: activate
    action: http
    depends_on: [send-welcome-email, setup-billing]
    config:
      url: "https://api.accounts.com/users/{{ params.customer_id }}/activate"
      method: POST
```

**Step 2: Validate**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && go run ./cmd/tailflow validate examples/recovery-demo.yaml`
Expected: Valid

**Step 3: Commit**

```bash
git add examples/recovery-demo.yaml
git commit -m "docs: add recovery-demo example showcasing persistence features"
```

---

### Task 13: Full validation

**Step 1: Run full test suite**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && make lint && make test`
Expected: All PASS, zero warnings

**Step 2: Run coverage check**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && make test-coverage`
Expected: 100% coverage

**Step 3: Fix any coverage gaps**

Add missing test cases for uncovered branches. Every `if`, every error path, every switch case must be tested.

**Step 4: Final commit if needed**

```bash
git add -A
git commit -m "test: achieve 100% coverage for persistence features"
```

---

---

## SHARED REPO TASKS (tailflow-shared)

### Task 14: Add ExecutionState event type and data structs (Go)

**Repo:** `tailflow-shared`

**Files:**
- Modify: `/Users/jackgianesini/workspace/tailflow/tailflow-shared/pkg/event/types.go`
- Modify: `/Users/jackgianesini/workspace/tailflow/tailflow-shared/pkg/event/data.go`

**Step 1: Add event type**

In `pkg/event/types.go`, add to the const block:

```go
ExecutionState EventType = "execution.state"
```

Add to `AllTypes` slice:

```go
var AllTypes = []EventType{
	WorkflowStarted, WorkflowCompleted,
	StepStarted, StepCompleted, StepFailed, StepSkipped,
	StepLog, StepWaiting, StepInput, StepOutput, StepGoto,
	Metrics, ExecutionState,
}
```

**Step 2: Add data structs**

In `pkg/event/data.go`, add:

```go
type ExecutionStateData struct {
	WorkflowName   string                    `json:"workflow_name"`
	Status         string                    `json:"status"`
	Steps          map[string]*StepStateData `json:"steps,omitempty"`
	GroupParams    map[string]any            `json:"group_params,omitempty"`
	IdempotencyKey string                    `json:"idempotency_key,omitempty"`
	Params         map[string]any            `json:"params,omitempty"`
	ErrorMessage   string                    `json:"error_message,omitempty"`
}

type StepStateData struct {
	Status     string `json:"status"`
	OnRecovery string `json:"on_recovery,omitempty"`
	StartedAt  string `json:"started_at,omitempty"`
	FinishedAt string `json:"finished_at,omitempty"`
	ErrorMsg   string `json:"error_message,omitempty"`
	ErrorCode  string `json:"error_code,omitempty"`
}
```

**Step 3: Validate**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-shared && go build ./...`
Expected: Build success

**Step 4: Commit**

```bash
cd /Users/jackgianesini/workspace/tailflow/tailflow-shared
git add pkg/event/types.go pkg/event/data.go
git commit -m "feat(event): add ExecutionState event type and data structs"
```

---

### Task 15: Add TypeScript types for ExecutionState and Groups

**Repo:** `tailflow-shared`

**Files:**
- Modify: `/Users/jackgianesini/workspace/tailflow/tailflow-shared/web/src/types/events.ts`
- Create: `/Users/jackgianesini/workspace/tailflow/tailflow-shared/web/src/types/groups.ts`
- Modify: `/Users/jackgianesini/workspace/tailflow/tailflow-shared/web/src/index.ts`

**Step 1: Add ExecutionState types to events.ts**

```typescript
export interface ExecutionStateData {
  workflow_name: string
  status: string
  steps?: Record<string, StepStateData>
  group_params?: Record<string, unknown>
  idempotency_key?: string
  params?: Record<string, unknown>
  error_message?: string
}

export interface StepStateData {
  status: string
  on_recovery?: string
  started_at?: string
  finished_at?: string
  error_message?: string
  error_code?: string
}
```

**Step 2: Create groups.ts**

```typescript
export interface GroupSummary {
  param_name: string
  param_value: string
  execution_count: number
  status_counts: Record<string, number>
  last_execution_at: string
}

export interface GroupExecution {
  execution_id: string
  workflow_name: string
  status: string
  started_at: string
  finished_at?: string
  current_step?: string
}

export interface GroupDetail {
  param_name: string
  param_value: string
  executions: GroupExecution[]
  total: number
}

export interface GroupMatrixCell {
  status: string
  execution_id: string
}

export interface GroupMatrixRow {
  param_value: string
  workflows: Record<string, GroupMatrixCell | null>
}
```

**Step 3: Export from index.ts**

Add exports for the new types in `web/src/index.ts`.

**Step 4: Commit**

```bash
cd /Users/jackgianesini/workspace/tailflow/tailflow-shared
git add web/src/types/events.ts web/src/types/groups.ts web/src/index.ts
git commit -m "feat(web): add TypeScript types for ExecutionState and Groups"
```

---

### Task 16: Update tailflow-shared version and publish

**Repo:** `tailflow-shared`

**Step 1: Tag new version**

```bash
cd /Users/jackgianesini/workspace/tailflow/tailflow-shared
git tag v0.2.0
git push origin main --tags
```

**Step 2: Update go.mod in agent and SaaS to use new version**

Both repos will need `go get github.com/tailflowio/tailflow-shared@v0.2.0` (done in their respective tasks).

---

## SAAS BACKEND TASKS (tailflow-saas)

### Task 17: Migrate database from MariaDB + ClickHouse to TiDB

**Repo:** `tailflow-saas`

**Files:**
- Create: `migrations/tidb/001_initial.sql`
- Modify: `internal/database/` (replace dual DB with single TiDB connection)
- Modify: `internal/module/` (update FX providers)
- Modify: `go.mod` (remove `clickhouse-go/v2`)

**Step 1: Create consolidated TiDB migration**

Create `migrations/tidb/001_initial.sql` combining all existing MariaDB tables + converted ClickHouse tables:

- Copy all MariaDB tables as-is (organizations, users, org_members, api_keys, plans, subscriptions, agent_sessions, workflows, export_jobs)
- Convert ClickHouse `events` table: remove `LowCardinality`, `MergeTree`, `PARTITION BY`, `TTL` → standard InnoDB + `ALTER TABLE events SET TIFLASH REPLICA 1`
- Convert ClickHouse `heartbeats` table: same treatment
- Add `seq` column (from ClickHouse migration 003)
- Add `agent_name` column (from MariaDB migration 003)

**Step 2: Replace database connection layer**

In `internal/database/`, replace the dual connection setup:
- Remove ClickHouse connection struct and constructor
- Keep a single `*sql.DB` connection to TiDB (uses same `go-sql-driver/mysql`)
- Update FX module to provide single DB

**Step 3: Update repositories**

Refactor all repository files:
- `EventRepository`: change from ClickHouse-specific queries to standard MySQL queries
  - Replace `count()` → `COUNT(*)`
  - Replace `ILIKE` → `LIKE` (MySQL is case-insensitive by default with utf8mb4)
  - Replace ClickHouse batch insert with standard `INSERT INTO ... VALUES`
  - Remove ClickHouse transaction handling
- `HeartbeatRepository`: same treatment
- All MariaDB repositories: change `*sql.DB` injection if the variable name changes

**Step 4: Remove ClickHouse dependency**

```bash
cd /Users/jackgianesini/workspace/tailflow/tailflow-saas
go get -u github.com/tailflowio/tailflow-shared@v0.2.0
# Remove clickhouse-go
go mod tidy
```

**Step 5: Validate**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-saas && go build ./...`
Expected: Build success

**Step 6: Commit**

```bash
git add migrations/ internal/database/ internal/repository/ internal/module/ go.mod go.sum
git commit -m "feat(db): migrate from MariaDB + ClickHouse to TiDB"
```

---

### Task 18: Add execution tables migration

**Repo:** `tailflow-saas`

**Files:**
- Create: `migrations/tidb/002_executions.sql`

**Step 1: Create migration**

```sql
SET NAMES utf8mb4;

CREATE TABLE IF NOT EXISTS executions (
    id                CHAR(36) NOT NULL PRIMARY KEY,
    organization_id   CHAR(36) NOT NULL,
    agent_session_id  CHAR(36) NOT NULL,
    workflow_name     VARCHAR(255) NOT NULL,
    status            VARCHAR(50) NOT NULL,
    params            JSON,
    error_message     TEXT,
    idempotency_key   VARCHAR(255),
    started_at        DATETIME(3) NOT NULL,
    finished_at       DATETIME(3),
    UNIQUE KEY uq_idempotency (organization_id, workflow_name, idempotency_key),
    INDEX idx_org_status (organization_id, status),
    INDEX idx_org_workflow (organization_id, workflow_name),
    INDEX idx_agent_status (agent_session_id, status),
    FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
    FOREIGN KEY (agent_session_id) REFERENCES agent_sessions(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS execution_steps (
    execution_id  CHAR(36) NOT NULL,
    step_id       VARCHAR(255) NOT NULL,
    status        VARCHAR(50) NOT NULL,
    on_recovery   VARCHAR(10) NOT NULL DEFAULT 'retry',
    input_data    JSON,
    output_data   JSON,
    error_message TEXT,
    error_code    VARCHAR(50),
    started_at    DATETIME(3),
    finished_at   DATETIME(3),
    PRIMARY KEY (execution_id, step_id),
    FOREIGN KEY (execution_id) REFERENCES executions(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS execution_group_params (
    execution_id  CHAR(36) NOT NULL,
    param_name    VARCHAR(255) NOT NULL,
    param_value   VARCHAR(255) NOT NULL,
    INDEX idx_lookup (param_name, param_value),
    INDEX idx_execution (execution_id),
    FOREIGN KEY (execution_id) REFERENCES executions(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

**Step 2: Commit**

```bash
git add migrations/tidb/002_executions.sql
git commit -m "feat(db): add executions, execution_steps, execution_group_params tables"
```

---

### Task 19: Add Execution models

**Repo:** `tailflow-saas`

**Files:**
- Create: `internal/model/execution.go`

**Step 1: Create models**

```go
package model

import "time"

type Execution struct {
	ID              string     `json:"id"`
	OrganizationID  string     `json:"organization_id"`
	AgentSessionID  string     `json:"agent_session_id"`
	WorkflowName    string     `json:"workflow_name"`
	Status          string     `json:"status"`
	Params          string     `json:"params,omitempty"`
	ErrorMessage    string     `json:"error_message,omitempty"`
	IdempotencyKey  string     `json:"idempotency_key,omitempty"`
	StartedAt       time.Time  `json:"started_at"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
}

type ExecutionStep struct {
	ExecutionID  string     `json:"execution_id"`
	StepID       string     `json:"step_id"`
	Status       string     `json:"status"`
	OnRecovery   string     `json:"on_recovery"`
	InputData    string     `json:"input_data,omitempty"`
	OutputData   string     `json:"output_data,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
	ErrorCode    string     `json:"error_code,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
}

type ExecutionGroupParam struct {
	ExecutionID string `json:"execution_id"`
	ParamName   string `json:"param_name"`
	ParamValue  string `json:"param_value"`
}
```

**Step 2: Commit**

```bash
git add internal/model/execution.go
git commit -m "feat(model): add Execution, ExecutionStep, ExecutionGroupParam models"
```

---

### Task 20: Add ExecutionRepository

**Repo:** `tailflow-saas`

**Files:**
- Create: `internal/repository/execution.go`

**Step 1: Implement repository**

Follow existing repository pattern (`*sql.DB` injection, `fmt.Errorf` wrapping, early returns):

Methods needed:
- `Create(exec *model.Execution) error`
- `UpdateStatus(id, status string, finishedAt *time.Time, errorMsg string) error`
- `UpsertStep(step *model.ExecutionStep) error`
- `InsertGroupParams(executionID string, params map[string]string) error`
- `FindByID(orgID, id string) (*model.Execution, error)`
- `FindSteps(executionID string) ([]model.ExecutionStep, error)`
- `FindRecoverable(agentSessionID string) ([]model.Execution, error)` — `WHERE status IN ('running', 'waiting')`
- `ClaimExecution(orgID, workflowName, idempotencyKey, executionID string) (claimed bool, existingID string, err error)` — `INSERT ... ON DUPLICATE KEY`
- `ListByGroupParam(orgID, paramName, paramValue string, limit, offset int) ([]model.Execution, int64, error)`
- `ListGroupSummaries(orgID string) ([]GroupSummary, error)` — aggregation query

All queries use standard MySQL syntax (TiDB compatible).

**Step 2: Validate**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-saas && go build ./...`

**Step 3: Commit**

```bash
git add internal/repository/execution.go
git commit -m "feat(repository): add ExecutionRepository with recovery, claim, and group queries"
```

---

### Task 21: Add TTL cleanup service

**Repo:** `tailflow-saas`

**Files:**
- Create: `internal/service/ttl.go`

**Step 1: Implement TTL service**

```go
package service

import (
	"context"
	"database/sql"
	"log/slog"
	"time"
)

type TTLService struct {
	db     *sql.DB
	plans  *PlanService
	logger *slog.Logger
}

func NewTTLService(db *sql.DB, plans *PlanService, logger *slog.Logger) *TTLService {
	return &TTLService{db: db, plans: plans, logger: logger}
}

func (s *TTLService) Start(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	s.cleanup(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.cleanup(ctx)
		}
	}
}

func (s *TTLService) cleanup(ctx context.Context) {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM events WHERE event_timestamp < NOW() - INTERVAL 90 DAY`)
	if err != nil {
		s.logger.Error("ttl cleanup events failed", "error", err)
	}

	_, err = s.db.ExecContext(ctx,
		`DELETE FROM heartbeats WHERE recorded_at < NOW() - INTERVAL 30 DAY`)
	if err != nil {
		s.logger.Error("ttl cleanup heartbeats failed", "error", err)
	}

	s.logger.Info("ttl cleanup completed")
}
```

**Step 2: Wire into server startup** (in FX module or server.go)

**Step 3: Commit**

```bash
git add internal/service/ttl.go
git commit -m "feat(service): add TTL cleanup service for events and heartbeats"
```

---

### Task 22: Handle execution.state events in TelemetryService.Ingest()

**Repo:** `tailflow-saas`

**Files:**
- Modify: `internal/service/telemetry.go`

**Step 1: Add execution state handling**

In `Ingest()`, after the existing event processing loop, detect `execution.state` events and persist to TiDB:

```go
// Inside the event loop, when ev.Type == "execution.state":
// 1. Parse ExecutionStateData from ev.Data
// 2. Upsert execution in executions table
// 3. Upsert each step in execution_steps table
// 4. Insert group params in execution_group_params table
```

Add `executionRepo *repository.ExecutionRepository` to `TelemetryService` struct and constructor.

**Step 2: Add Recovery method**

```go
func (s *TelemetryService) Recovery(agentSessionID string) ([]RecoveryExecution, error) {
	execs, err := s.executionRepo.FindRecoverable(agentSessionID)
	if err != nil {
		return nil, fmt.Errorf("find recoverable executions: %w", err)
	}

	var result []RecoveryExecution
	for _, exec := range execs {
		steps, err := s.executionRepo.FindSteps(exec.ID)
		if err != nil {
			s.logger.Warn("failed to find steps for recovery",
				"execution_id", exec.ID,
				"error", err,
			)
			continue
		}
		result = append(result, RecoveryExecution{
			ExecutionID:  exec.ID,
			WorkflowName: exec.WorkflowName,
			Status:       exec.Status,
			Params:       exec.Params,
			Steps:        steps,
		})
	}

	return result, nil
}
```

**Step 3: Add ClaimExecution method**

```go
func (s *TelemetryService) ClaimExecution(orgID, workflowName, idempotencyKey, executionID string) (*ClaimResult, error) {
	claimed, existingID, err := s.executionRepo.ClaimExecution(orgID, workflowName, idempotencyKey, executionID)
	if err != nil {
		return nil, fmt.Errorf("claim execution: %w", err)
	}

	if !claimed {
		existing, _ := s.executionRepo.FindByID(orgID, existingID)
		return &ClaimResult{
			Claimed:             false,
			ExistingExecutionID: existingID,
			ExistingStatus:      existing.Status,
		}, nil
	}

	return &ClaimResult{Claimed: true}, nil
}
```

**Step 4: Validate**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-saas && go build ./...`

**Step 5: Commit**

```bash
git add internal/service/telemetry.go
git commit -m "feat(service): handle execution.state events, add Recovery and ClaimExecution"
```

---

### Task 23: Add recovery and claim HTTP handlers

**Repo:** `tailflow-saas`

**Files:**
- Modify: `internal/handler/telemetry.go`
- Modify: `internal/server/router.go`

**Step 1: Add Recovery handler**

Follow existing Fiber handler pattern:

```go
func (h *TelemetryHandler) Recovery(c *fiber.Ctx) error {
	orgID, _ := c.Locals(LocalsOrgID).(string)
	agentID := c.Query("agent_id")

	if agentID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "agent_id required"})
	}

	execs, err := h.telemetry.Recovery(agentID)
	if err != nil {
		h.logger.Error("recovery failed", "error", err, "agent_id", agentID)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(execs)
}
```

**Step 2: Add Claim handler**

```go
func (h *TelemetryHandler) ClaimExecution(c *fiber.Ctx) error {
	orgID, _ := c.Locals(LocalsOrgID).(string)

	var req struct {
		ExecutionID    string `json:"execution_id"`
		WorkflowName   string `json:"workflow_name"`
		IdempotencyKey string `json:"idempotency_key"`
	}

	err := c.BodyParser(&req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	result, err := h.telemetry.ClaimExecution(orgID, req.WorkflowName, req.IdempotencyKey, req.ExecutionID)
	if err != nil {
		h.logger.Error("claim failed", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(result)
}
```

**Step 3: Register routes**

In `internal/server/router.go`, add to agent API routes:

```go
app.Get("/api/v1/agent/recovery", agentAuth, p.TelemetryH.Recovery)
app.Post("/api/v1/agent/executions/claim", agentAuth, p.TelemetryH.ClaimExecution)
```

**Step 4: Validate**

Run: `cd /Users/jackgianesini/workspace/tailflow/tailflow-saas && go build ./...`

**Step 5: Commit**

```bash
git add internal/handler/telemetry.go internal/server/router.go
git commit -m "feat(handler): add recovery and claim HTTP endpoints"
```

---

### Task 24: Add Groups dashboard handler and routes

**Repo:** `tailflow-saas`

**Files:**
- Modify: `internal/handler/dashboard.go`
- Modify: `internal/server/router.go`

**Step 1: Add Groups handlers**

```go
func (h *DashboardHandler) ListGroups(c *fiber.Ctx) error {
	orgID, _ := c.Locals(LocalsOrgID).(string)

	summaries, err := h.executionRepo.ListGroupSummaries(orgID)
	if err != nil {
		h.logger.Error("list groups failed", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(summaries)
}

func (h *DashboardHandler) GetGroupExecutions(c *fiber.Ctx) error {
	orgID, _ := c.Locals(LocalsOrgID).(string)
	paramName := c.Params("param_name")
	paramValue := c.Params("param_value")
	limit := c.QueryInt("limit", 50)
	offset := c.QueryInt("offset", 0)

	execs, total, err := h.executionRepo.ListByGroupParam(orgID, paramName, paramValue, limit, offset)
	if err != nil {
		h.logger.Error("get group executions failed", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(fiber.Map{
		"executions": execs,
		"total":      total,
	})
}
```

**Step 2: Register routes**

```go
api.Get("/groups", p.DashboardH.ListGroups)
api.Get("/groups/:param_name/:param_value/executions", p.DashboardH.GetGroupExecutions)
```

**Step 3: Validate and commit**

```bash
git add internal/handler/dashboard.go internal/server/router.go
git commit -m "feat(handler): add groups dashboard endpoints"
```

---

## SAAS FRONTEND TASKS (tailflow-saas/web)

### Task 25: Add groups Pinia store

**Repo:** `tailflow-saas`

**Files:**
- Create: `web/src/stores/groups.ts`
- Modify: `web/src/types/index.ts` (import shared group types)

**Step 1: Create store**

Follow existing store pattern (composition API, `useApi` composable):

```typescript
import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { GroupSummary, GroupDetail } from '@tailflow/shared'
import { useApi } from '../composables/useApi'

export const useGroupsStore = defineStore('groups', () => {
  const summaries = ref<GroupSummary[]>([])
  const currentGroup = ref<GroupDetail | null>(null)

  const { get } = useApi()

  async function fetchSummaries() {
    summaries.value = await get<GroupSummary[]>('/api/v1/groups') ?? []
  }

  async function fetchGroupDetail(paramName: string, paramValue: string, limit = 50, offset = 0) {
    const qs = new URLSearchParams({ limit: String(limit), offset: String(offset) })
    const data = await get<GroupDetail>(`/api/v1/groups/${paramName}/${paramValue}/executions?${qs}`)
    currentGroup.value = data
  }

  return { summaries, currentGroup, fetchSummaries, fetchGroupDetail }
})
```

**Step 2: Commit**

```bash
git add web/src/stores/groups.ts web/src/types/
git commit -m "feat(web): add groups Pinia store"
```

---

### Task 26: Add GroupsView page

**Repo:** `tailflow-saas`

**Files:**
- Create: `web/src/views/GroupsView.vue`
- Create: `web/src/views/GroupDetailView.vue`
- Modify: `web/src/router/index.ts`

**Step 1: Create GroupsView**

List view showing all group_by params with their values and execution counts. Follow existing view patterns (TailwindCSS, composition API).

Key elements:
- Fetch summaries on mount
- Table with columns: param_name, param_value, execution_count, status breakdown, last_execution_at
- Click row → navigate to detail view
- Search/filter

**Step 2: Create GroupDetailView**

Detail view showing all executions for a specific group value.

Key elements:
- Header with param_name=param_value
- List of executions with status, workflow_name, started_at, current_step
- Click execution → navigate to existing ExecutionDetailView
- Pagination

**Step 3: Add routes**

```typescript
{
  path: '/groups',
  name: 'groups',
  component: () => import('../views/GroupsView.vue'),
  meta: { auth: true },
},
{
  path: '/groups/:paramName/:paramValue',
  name: 'group-detail',
  component: () => import('../views/GroupDetailView.vue'),
  meta: { auth: true },
},
```

**Step 4: Add navigation link** in the sidebar/nav (wherever existing nav links are)

**Step 5: Commit**

```bash
git add web/src/views/GroupsView.vue web/src/views/GroupDetailView.vue web/src/router/index.ts
git commit -m "feat(web): add Groups list and detail views"
```

---

### Task 27: Add GroupMatrix component

**Repo:** `tailflow-saas`

**Files:**
- Create: `web/src/components/GroupMatrix.vue`
- Modify: `web/src/views/GroupsView.vue` (add matrix tab)

**Step 1: Create GroupMatrix component**

Matrix view: rows = group values, columns = workflows, cells = status icon.

Key elements:
- Fetches data from a new API endpoint or aggregates client-side from summaries
- Color-coded status cells (success=green, running=blue, waiting=amber, failed=red)
- Click cell → navigate to execution detail
- Responsive table with horizontal scroll

**Step 2: Add as tab in GroupsView**

Toggle between list view and matrix view.

**Step 3: Commit**

```bash
git add web/src/components/GroupMatrix.vue web/src/views/GroupsView.vue
git commit -m "feat(web): add GroupMatrix component with status overview"
```

---

### Task 28: Add on_recovery and resumed badges to agent frontend

**Repo:** `tailflow-agent`

**Files:**
- Modify: `web/frontend/` (find the step detail component and execution detail component)

**Step 1: Identify the relevant Vue components**

Read the agent's `web/frontend/src/` directory to find:
- Step detail/DAG visualization component
- Execution detail component

**Step 2: Add on_recovery badge**

On each step in the DAG visualization, if `on_recovery` is set and not "retry" (default), show a small badge:
- `skip` → amber badge "skip on recovery"
- `fail` → red badge "fail on recovery"

**Step 3: Add "resumed" badge**

On the execution detail view, if the execution was resumed from recovery, show a badge "Resumed from crash recovery".

**Step 4: Commit**

```bash
git add web/frontend/
git commit -m "feat(web): add on_recovery and resumed badges to agent UI"
```

---

### Task 29: Final validation across all repos

**Step 1: Validate shared**

```bash
cd /Users/jackgianesini/workspace/tailflow/tailflow-shared && go build ./...
cd /Users/jackgianesini/workspace/tailflow/tailflow-shared/web && npm run build
```

**Step 2: Validate agent**

```bash
cd /Users/jackgianesini/workspace/tailflow/tailflow-agent && make lint && make test && make test-coverage
```

**Step 3: Validate SaaS**

```bash
cd /Users/jackgianesini/workspace/tailflow/tailflow-saas && go build ./...
cd /Users/jackgianesini/workspace/tailflow/tailflow-saas/web && npm run build
```

**Step 4: Final commits if needed**

---

## Task Dependency Graph

```
SHARED:
  Task 14 (Go event types) ──┬──▶ Task 16 (version + publish)
  Task 15 (TS types) ────────┘

AGENT (depends on Task 16):
  Task 1 (group_by) ─────────┐
  Task 2 (on_recovery) ──────┤
  Task 3 (idempotency_key) ──┼──▶ Task 5 (recovery engine) ──▶ Task 6 (state events) ──▶ Task 7 (idemp resolve)
  Task 4 (context fields) ───┘                                                                │
                                                            Task 8 (recovery client) ──▶ Task 10 (server recovery)
                                                            Task 9 (claim client) ────▶ Task 11 (server idemp)
                                                                                               │
                                                                                Task 12 (example) ──▶ Task 13 (validation)
                                                                                Task 28 (agent UI badges)

SAAS (depends on Task 16):
  Task 17 (TiDB migration) ──▶ Task 18 (exec tables) ──▶ Task 19 (models) ──▶ Task 20 (repository)
                                                                                      │
                                Task 21 (TTL service)                                  ▼
                                                                              Task 22 (telemetry service)
                                                                                      │
                                                                              Task 23 (handlers) ──▶ Task 24 (groups handlers)
                                                                                                           │
                                                                              Task 25 (groups store) ──────┤
                                                                              Task 26 (groups views) ──────┤
                                                                              Task 27 (matrix component) ──┘
                                                                                                           │
                                                                                                    Task 29 (final validation)
```

Parallelizable:
- Tasks 14-15 (shared Go + TS)
- Tasks 1-4 (agent parser + context)
- Tasks 8-9 (agent clients)
- Tasks 17 + 21 (TiDB migration + TTL)
- Tasks 25-27 (SaaS frontend)
- Agent tasks + SaaS tasks (independent repos, can run in parallel after shared is published)
