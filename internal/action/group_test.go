package action

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type GroupActionTestSuite struct {
	suite.Suite
}

func TestGroupAction(t *testing.T) {
	suite.Run(t, new(GroupActionTestSuite))
}

func (s *GroupActionTestSuite) TestValidate_MissingKey() {
	a := NewGroupAction()
	err := a.Validate(newTestContext(map[string]any{}))
	s.Error(err)
	s.Contains(err.Error(), "key")
}

func (s *GroupActionTestSuite) TestValidate_OK() {
	a := NewGroupAction()
	err := a.Validate(newTestContext(map[string]any{"key": "customer-123"}))
	s.NoError(err)
}

func (s *GroupActionTestSuite) TestExecute_EmitsGroup() {
	a := NewGroupAction()
	ctx := newTestContext(map[string]any{"key": "order-456"})

	var emitted string
	ctx.EmitGroup = func(key string) { emitted = key }

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.Equal("order-456", emitted)
	s.Equal("order-456", out.(map[string]any)["key"])
}

func (s *GroupActionTestSuite) TestExecute_NonStringKey() {
	a := NewGroupAction()
	ctx := newTestContext(map[string]any{"key": 42})

	var emitted string
	ctx.EmitGroup = func(key string) { emitted = key }

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.Equal("42", emitted)
	s.Equal("42", out.(map[string]any)["key"])
}
