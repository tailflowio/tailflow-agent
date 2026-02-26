package action

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	fakeruntime "github.com/tailflow/tailflow/internal/fake/fakeruntime"
)

type UnlockActionTestSuite struct {
	suite.Suite
}

func TestUnlockAction(t *testing.T) {
	suite.Run(t, new(UnlockActionTestSuite))
}

func (s *UnlockActionTestSuite) SetupTest() {}

func (s *UnlockActionTestSuite) TestUnlockMissingKey() {
	a := NewUnlockAction()
	locker := fakeruntime.NewLocker(s.T())
	ctx := newTestContextWithServices(map[string]any{}, locker)
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "key")
}

func (s *UnlockActionTestSuite) TestUnlockNoServices() {
	a := NewUnlockAction()
	ctx := newTestContext(map[string]any{"key": "test"})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "serve")
}

func (s *UnlockActionTestSuite) TestUnlockExecute() {
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

func (s *UnlockActionTestSuite) TestUnlockExecuteError() {
	locker := fakeruntime.NewLocker(s.T())
	locker.EXPECT().Unlock(mock.Anything, "fail-lock").Return(fmt.Errorf("unlock failed"))

	ctx := newTestContextWithServices(map[string]any{"key": "fail-lock"}, locker)

	u := NewUnlockAction()
	_, err := u.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "unlock failed")
}

func (s *UnlockActionTestSuite) TestUnlockValidateOK() {
	a := NewUnlockAction()
	locker := fakeruntime.NewLocker(s.T())
	ctx := newTestContextWithServices(map[string]any{"key": "my-lock"}, locker)
	err := a.Validate(ctx)
	s.NoError(err)
}
