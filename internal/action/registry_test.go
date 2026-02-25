package action_test

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/action"
	"github.com/tailflow/tailflow/internal/fake/fakeaction"
)

type RegistryTestSuite struct {
	suite.Suite
}

func TestRegistry(t *testing.T) {
	suite.Run(t, new(RegistryTestSuite))
}

func (s *RegistryTestSuite) SetupTest() {}

func (s *RegistryTestSuite) TestRegisterAndCreate() {
	reg := action.NewRegistry()
	fa := fakeaction.NewAction(s.T())
	fa.EXPECT().Execute(mock.Anything).Return("ok", nil)

	reg.Register("mock", func() action.Action { return fa })

	a, err := reg.Create("mock")
	s.Require().NoError(err)
	s.NotNil(a)

	out, err := a.Execute(nil)
	s.Require().NoError(err)
	s.Equal("ok", out)
}

func (s *RegistryTestSuite) TestCreateUnknown() {
	reg := action.NewRegistry()
	_, err := reg.Create("nonexistent")
	s.ErrorContains(err, "unknown action")
}

func (s *RegistryTestSuite) TestHas() {
	reg := action.NewRegistry()
	s.False(reg.Has("mock"))

	fa := fakeaction.NewAction(s.T())
	reg.Register("mock", func() action.Action { return fa })
	s.True(reg.Has("mock"))
}

func (s *RegistryTestSuite) TestNames() {
	reg := action.NewRegistry()
	fa := fakeaction.NewAction(s.T())
	reg.Register("http", func() action.Action { return fa })
	reg.Register("exec", func() action.Action { return fa })
	reg.Register("log", func() action.Action { return fa })

	names := reg.Names()
	s.Equal([]string{"exec", "http", "log"}, names)
}

func (s *RegistryTestSuite) TestAllowlistBlocksAction() {
	reg := action.NewRegistry()
	fa := fakeaction.NewAction(s.T())
	reg.Register("http", func() action.Action { return fa })
	reg.Register("exec", func() action.Action { return fa })

	reg.SetAllowlist([]string{"http"})

	_, err := reg.Create("exec")
	s.ErrorContains(err, "not allowed")
}

func (s *RegistryTestSuite) TestAllowlistAllowsAction() {
	reg := action.NewRegistry()
	fa := fakeaction.NewAction(s.T())
	reg.Register("http", func() action.Action { return fa })
	reg.Register("exec", func() action.Action { return fa })

	reg.SetAllowlist([]string{"http", "exec"})

	a, err := reg.Create("http")
	s.Require().NoError(err)
	s.NotNil(a)

	a, err = reg.Create("exec")
	s.Require().NoError(err)
	s.NotNil(a)
}

func (s *RegistryTestSuite) TestAllowlistNilAllowsAll() {
	reg := action.NewRegistry()
	fa := fakeaction.NewAction(s.T())
	reg.Register("http", func() action.Action { return fa })
	reg.Register("exec", func() action.Action { return fa })

	// Default: no allowlist set
	a, err := reg.Create("http")
	s.Require().NoError(err)
	s.NotNil(a)

	a, err = reg.Create("exec")
	s.Require().NoError(err)
	s.NotNil(a)
}

func (s *RegistryTestSuite) TestSetAllowlistNilRemovesRestriction() {
	reg := action.NewRegistry()
	fa := fakeaction.NewAction(s.T())
	reg.Register("http", func() action.Action { return fa })

	reg.SetAllowlist([]string{"exec"})
	_, err := reg.Create("http")
	s.Error(err)

	reg.SetAllowlist(nil)
	a, err := reg.Create("http")
	s.Require().NoError(err)
	s.NotNil(a)
}
