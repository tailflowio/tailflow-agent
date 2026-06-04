package engine

import (
	"context"
	"time"

	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/runtime"
)

func (s *EngineTestSuite) TestExecute_RecoverySkipsCompletedSteps() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	var executedSteps eventCollector[string]

	ch := bus.Subscribe(100)
	go func() {
		for ev := range ch {
			if ev.Type == event.StepStarted {
				executedSteps.add(ev.StepID)
			}
		}
	}()

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

	s.Eventually(func() bool {
		return executedSteps.load() >= 2
	}, 2*time.Second, 10*time.Millisecond)

	steps := executedSteps.snapshot()
	for _, stepID := range steps {
		s.NotEqual("step1", stepID, "step1 should have been skipped")
	}

	s.Contains(steps, "step2")
	s.Contains(steps, "step3")
}

func (s *EngineTestSuite) TestExecute_RecoveryOnRecoverySkip() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	var executedSteps eventCollector[string]

	ch := bus.Subscribe(100)
	go func() {
		for ev := range ch {
			if ev.Type == event.StepStarted {
				executedSteps.add(ev.StepID)
			}
		}
	}()

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

	s.Eventually(func() bool {
		return executedSteps.load() >= 1
	}, 2*time.Second, 10*time.Millisecond)

	steps := executedSteps.snapshot()
	for _, stepID := range steps {
		s.NotEqual("send-email", stepID, "send-email should have been skipped (on_recovery=skip)")
	}

	s.Contains(steps, "next-step")
}

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

func (s *EngineTestSuite) TestExecute_RecoveryOnRecoveryRetryDefault() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	var executedSteps eventCollector[string]

	ch := bus.Subscribe(100)
	go func() {
		for ev := range ch {
			if ev.Type == event.StepStarted {
				executedSteps.add(ev.StepID)
			}
		}
	}()

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

	s.Eventually(func() bool {
		return executedSteps.load() >= 2
	}, 2*time.Second, 10*time.Millisecond)

	s.Contains(executedSteps.snapshot(), "create-account")
}

func (s *EngineTestSuite) TestExecute_RecoveryWaitingStepFallsThrough() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	var executedSteps eventCollector[string]

	ch := bus.Subscribe(100)
	go func() {
		for ev := range ch {
			if ev.Type == event.StepStarted {
				executedSteps.add(ev.StepID)
			}
		}
	}()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-recovery-waiting",
		Steps: []parser.Step{
			{ID: "await", Action: "log", OnRecovery: "skip", Config: map[string]any{"message": "await"}},
			{ID: "next", Action: "log", DependsOn: []string{"await"}, Config: map[string]any{"message": "next"}},
		},
	}

	now := time.Now()
	recoveredSteps := map[string]*runtime.StepResult{
		"await": {
			Status:    runtime.StatusWaiting,
			StartedAt: &now,
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{
		Resumed:        true,
		RecoveredSteps: recoveredSteps,
	})
	s.Require().NoError(err)
	s.Equal(runtime.StatusSuccess, result.Status)

	s.Eventually(func() bool {
		return executedSteps.load() >= 2
	}, 2*time.Second, 10*time.Millisecond)

	steps := executedSteps.snapshot()
	s.Contains(steps, "await")
	s.Contains(steps, "next")
}

func (s *EngineTestSuite) TestExecute_GroupActionEmitsGroupEvent() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	var groupEvents eventCollector[event.Event]

	ch := bus.Subscribe(100)
	go func() {
		for ev := range ch {
			if ev.Type == event.ExecutionGroup {
				groupEvents.add(ev)
			}
		}
	}()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-group-action",
		Params: []parser.Param{
			{Name: "customer_id", Type: "string"},
		},
		Steps: []parser.Step{
			{ID: "tag-customer", Action: "group", Config: map[string]any{"key": "customer-{{ params.customer_id }}"}},
			{ID: "step1", Action: "log", DependsOn: []string{"tag-customer"}, Config: map[string]any{"message": "hello"}},
		},
	}

	result, err := exec.Execute(context.Background(), wf, map[string]any{
		"customer_id": "user-42",
	})
	s.Require().NoError(err)
	s.Equal(runtime.StatusSuccess, result.Status)

	s.Eventually(func() bool {
		return groupEvents.load() >= 1
	}, 2*time.Second, 10*time.Millisecond)

	events := groupEvents.snapshot()
	s.Require().NotEmpty(events)
	s.Equal("customer-user-42", events[0].Data["group_key"])
	s.Equal("tag-customer", events[0].StepID)
}

func (s *EngineTestSuite) TestExecute_ResolvesIdempotencyKey() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	var capturedKeys eventCollector[string]

	ch := bus.Subscribe(100)
	go func() {
		for ev := range ch {
			if ev.Type != event.ExecutionState {
				continue
			}

			key, _ := ev.Data["idempotency_key"].(string)
			capturedKeys.add(key)
		}
	}()

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

	s.Eventually(func() bool {
		return capturedKeys.load() >= 1
	}, 2*time.Second, 10*time.Millisecond)

	keys := capturedKeys.snapshot()
	s.Equal("user-42", keys[len(keys)-1])
}

func (s *EngineTestSuite) TestExecute_NoIdempotencyKeyWhenNotConfigured() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	var stateEvents eventCollector[event.Event]

	ch := bus.Subscribe(100)
	go func() {
		for ev := range ch {
			if ev.Type == event.ExecutionState {
				stateEvents.add(ev)
			}
		}
	}()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-no-idemp",
		Steps: []parser.Step{
			{ID: "step1", Action: "log", Config: map[string]any{"message": "hello"}},
		},
	}

	_, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)

	s.Eventually(func() bool {
		return stateEvents.load() >= 1
	}, 2*time.Second, 10*time.Millisecond)

	events := stateEvents.snapshot()
	s.NotEmpty(events)
	s.Empty(events[0].Data["idempotency_key"])
}

func (s *EngineTestSuite) TestExecute_RecoveryNonRunningStepIgnored() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	var executedSteps eventCollector[string]

	ch := bus.Subscribe(100)
	go func() {
		for ev := range ch {
			if ev.Type == event.StepStarted {
				executedSteps.add(ev.StepID)
			}
		}
	}()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-recovery-failed-step",
		Steps: []parser.Step{
			{ID: "step1", Action: "log", Config: map[string]any{"message": "hello"}},
		},
	}

	now := time.Now()
	recoveredSteps := map[string]*runtime.StepResult{
		"step1": {
			Status:    runtime.StatusFailed,
			StartedAt: &now,
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{
		Resumed:        true,
		RecoveredSteps: recoveredSteps,
	})
	s.Require().NoError(err)
	s.Equal(runtime.StatusSuccess, result.Status)

	s.Eventually(func() bool {
		return executedSteps.load() >= 1
	}, 2*time.Second, 10*time.Millisecond)

	s.Contains(executedSteps.snapshot(), "step1")
}
