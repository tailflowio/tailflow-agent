package action

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	fakeruntime "github.com/tailflow/tailflow/internal/fake/fakeruntime"
	"github.com/tailflow/tailflow/internal/runtime"
)

func newTestContextWithServices(config map[string]any, locker runtime.Locker) *ActionContext {
	services := &runtime.ActionServices{
		Locker: locker,
	}
	return &ActionContext{
		Context:  context.Background(),
		Config:   config,
		ExecCtx:  runtime.NewExecutionContext("test-exec", "test-wf", nil, nil),
		StepID:   "test-step",
		Logger:   slog.Default(),
		Services: services,
	}
}

type LockActionTestSuite struct {
	suite.Suite
}

func TestLockAction(t *testing.T) {
	suite.Run(t, new(LockActionTestSuite))
}

func (s *LockActionTestSuite) SetupTest() {}

func (s *LockActionTestSuite) TestMissingKey() {
	a := NewLockAction()
	locker := fakeruntime.NewLocker(s.T())
	ctx := newTestContextWithServices(map[string]any{}, locker)
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "key")
}

func (s *LockActionTestSuite) TestNoServices() {
	a := NewLockAction()
	ctx := newTestContext(map[string]any{"key": "test"})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "serve")
}

func (s *LockActionTestSuite) TestExecute_AcquiresLock() {
	a := NewLockAction()
	locker := fakeruntime.NewLocker(s.T())
	locker.EXPECT().Lock(mock.Anything, "my-lock", 1*time.Second).Return(nil)

	ctx := newTestContextWithServices(map[string]any{"key": "my-lock", "timeout": "1s"}, locker)

	err := a.Validate(ctx)
	s.Require().NoError(err)

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	result := out.(map[string]any)
	s.True(result["acquired"].(bool))
	s.Equal("my-lock", result["key"])
}

func (s *LockActionTestSuite) TestLockValidateOK() {
	a := NewLockAction()
	locker := fakeruntime.NewLocker(s.T())
	ctx := newTestContextWithServices(map[string]any{"key": "my-lock"}, locker)
	err := a.Validate(ctx)
	s.NoError(err)
}

func (s *LockActionTestSuite) TestLockExecuteError() {
	locker := fakeruntime.NewLocker(s.T())
	locker.EXPECT().Lock(mock.Anything, "fail-lock", mock.Anything).Return(fmt.Errorf("lock timeout"))

	ctx := newTestContextWithServices(map[string]any{"key": "fail-lock"}, locker)

	a := NewLockAction()
	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "lock timeout")
}

func (s *LockActionTestSuite) TestLockExecuteDefaultTimeout() {
	locker := fakeruntime.NewLocker(s.T())
	locker.EXPECT().Lock(mock.Anything, "my-lock", 30*time.Second).Return(nil)

	// No timeout in config - should use default 30s
	ctx := newTestContextWithServices(map[string]any{"key": "my-lock"}, locker)

	a := NewLockAction()
	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.True(out.(map[string]any)["acquired"].(bool))
}
