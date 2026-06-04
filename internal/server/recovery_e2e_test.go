package server

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/tailflow/tailflow/internal/action"
	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/runtime"
	"github.com/tailflow/tailflow/internal/store"
)

type RecoveryE2ETestSuite struct {
	suite.Suite

	context context.Context
}

func TestRecoveryE2E(t *testing.T) {
	suite.Run(t, new(RecoveryE2ETestSuite))
}

func (s *RecoveryE2ETestSuite) SetupTest() {
	s.context = context.Background()
}

func (s *RecoveryE2ETestSuite) newRecoveryServer(memStore *store.MemoryExecutionStore) *Server {
	yaml := `version: "2.0"
name: "recovery-workflow"
description: "recovery e2e workflow"
stages:
  - name: default
steps:
  - id: send-email
    action: log
    stage: default
    on_recovery: skip
    config:
      message: "email"
  - id: downstream
    action: log
    stage: default
    depends_on: ["send-email"]
    config:
      message: "downstream"
`
	wf, err := parser.ParseBytes([]byte(yaml))
	s.Require().NoError(err)

	bus := event.NewBus()
	s.T().Cleanup(bus.Close)

	reg := action.NewRegistry()
	action.RegisterBuiltins(reg)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	exec := engine.NewExecutor(reg, bus, logger, nil, nil, nil)

	srv := New(Config{
		Port:           0,
		Executor:       exec,
		Workflow:       wf,
		ExecutionStore: memStore,
		EventBus:       bus,
		Logger:         logger,
		Recoverer:      store.NewMemoryRecoverer(memStore),
	})
	srv.config.Workflow.Recovery = true
	srv.ctx = context.Background()

	return srv
}

type recoveryRecorder struct {
	mu      sync.Mutex
	started map[string]int
	steps   map[string]int
}

func newRecoveryRecorder(bus *event.Bus) *recoveryRecorder {
	rec := &recoveryRecorder{
		started: map[string]int{},
		steps:   map[string]int{},
	}

	ch := bus.Subscribe(200)

	go func() {
		for ev := range ch {
			rec.mu.Lock()

			switch ev.Type {
			case event.WorkflowStarted:
				rec.started[ev.ExecutionID]++
			case event.StepStarted:
				rec.steps[ev.StepID]++
			default:
			}

			rec.mu.Unlock()
		}
	}()

	return rec
}

func (r *recoveryRecorder) startedCount(executionID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.started[executionID]
}

func (r *recoveryRecorder) stepCount(stepID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.steps[stepID]
}

func (s *RecoveryE2ETestSuite) TestRecovery_E2E_MemoryBackend_ResumesWithOnRecoverySkip() {
	now := time.Now()
	memStore := store.NewExecutionStore(10)

	s.Require().NoError(memStore.Add(s.context, &store.Execution{
		ID:           "exec-recovered",
		WorkflowName: "recovery-workflow",
		Status:       runtime.StatusRunning,
		Params:       map[string]any{},
		Steps: map[string]*runtime.StepResult{
			"send-email": {Status: runtime.StatusRunning, StartedAt: &now},
		},
		StartedAt: now,
	}))

	srv := s.newRecoveryServer(memStore)
	recorder := newRecoveryRecorder(srv.config.EventBus)

	srv.recoverExecutions(s.context)

	s.Eventually(func() bool {
		return recorder.startedCount("exec-recovered") >= 1
	}, 2*time.Second, 10*time.Millisecond)

	s.Eventually(func() bool {
		return recorder.stepCount("downstream") >= 1
	}, 2*time.Second, 10*time.Millisecond)

	s.assertTerminalSuccess(srv, "exec-recovered")

	s.Equal(0, recorder.stepCount("send-email"), "on_recovery=skip step must not re-execute")
}

func (s *RecoveryE2ETestSuite) TestRecovery_E2E_OnlyNonTerminalResumed() {
	now := time.Now()
	memStore := store.NewExecutionStore(10)

	s.Require().NoError(memStore.Add(s.context, &store.Execution{
		ID:           "exec-success",
		WorkflowName: "recovery-workflow",
		Status:       runtime.StatusSuccess,
		StartedAt:    now,
	}))
	s.Require().NoError(memStore.Add(s.context, &store.Execution{
		ID:           "exec-failed",
		WorkflowName: "recovery-workflow",
		Status:       runtime.StatusFailed,
		StartedAt:    now,
	}))
	s.Require().NoError(memStore.Add(s.context, &store.Execution{
		ID:           "exec-running",
		WorkflowName: "recovery-workflow",
		Status:       runtime.StatusRunning,
		Params:       map[string]any{},
		Steps: map[string]*runtime.StepResult{
			"send-email": {Status: runtime.StatusRunning, StartedAt: &now},
		},
		StartedAt: now,
	}))

	srv := s.newRecoveryServer(memStore)
	recorder := newRecoveryRecorder(srv.config.EventBus)

	srv.recoverExecutions(s.context)

	s.Eventually(func() bool {
		return recorder.startedCount("exec-running") >= 1
	}, 2*time.Second, 10*time.Millisecond)

	s.assertTerminalSuccess(srv, "exec-running")

	s.Equal(0, recorder.startedCount("exec-success"), "terminal success must not resume")
	s.Equal(0, recorder.startedCount("exec-failed"), "terminal failed must not resume")
}

func (s *RecoveryE2ETestSuite) assertTerminalSuccess(srv *Server, executionID string) {
	s.T().Helper()

	s.Eventually(func() bool {
		events, err := srv.config.ExecutionStore.GetEvents(s.context, executionID)
		if err != nil {
			return false
		}

		for _, ev := range events {
			if ev.Type != event.WorkflowCompleted {
				continue
			}

			status, _ := ev.Data["status"].(string)
			if status == runtime.StatusSuccess {
				return true
			}
		}

		return false
	}, 3*time.Second, 10*time.Millisecond)
}
