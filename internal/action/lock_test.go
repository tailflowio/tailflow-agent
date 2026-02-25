package action

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/runtime"
	fakeruntime "github.com/tailflow/tailflow/internal/fake/fakeruntime"
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

func (s *LockActionTestSuite) TestExecute() {
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

func (s *LockActionTestSuite) TestUnlockMissingKey() {
	a := NewUnlockAction()
	locker := fakeruntime.NewLocker(s.T())
	ctx := newTestContextWithServices(map[string]any{}, locker)
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "key")
}

func (s *LockActionTestSuite) TestUnlockNoServices() {
	a := NewUnlockAction()
	ctx := newTestContext(map[string]any{"key": "test"})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "serve")
}

func (s *LockActionTestSuite) TestUnlockExecute() {
	locker := fakeruntime.NewLocker(s.T())
	locker.EXPECT().Lock(mock.Anything, "my-lock", 1*time.Second).Return(nil)
	locker.EXPECT().Unlock(mock.Anything, "my-lock").Return(nil)

	ctx := newTestContextWithServices(map[string]any{"key": "my-lock", "timeout": "1s"}, locker)

	// Lock first
	a := NewLockAction()
	_, err := a.Execute(ctx)
	s.Require().NoError(err)

	// Then unlock
	u := NewUnlockAction()
	out, err := u.Execute(ctx)
	s.Require().NoError(err)
	result := out.(map[string]any)
	s.True(result["released"].(bool))
	s.Equal("my-lock", result["key"])
}
