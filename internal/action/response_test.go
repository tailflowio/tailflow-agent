package action

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ResponseActionTestSuite struct {
	suite.Suite
}

func TestResponseAction(t *testing.T) {
	suite.Run(t, new(ResponseActionTestSuite))
}

func (s *ResponseActionTestSuite) SetupTest() {}

func (s *ResponseActionTestSuite) TestExecute() {
	a := NewResponseAction()
	ctx := newTestContext(map[string]any{
		"status": 201,
		"body":   map[string]any{"id": "123"},
		"headers": map[string]any{
			"X-Custom": "value",
		},
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	m := out.(map[string]any)
	s.Equal(201, m["status"])
	s.Equal(map[string]any{"id": "123"}, m["body"])
	s.Equal("value", m["headers"].(map[string]string)["X-Custom"])
}

func (s *ResponseActionTestSuite) TestDefaults() {
	a := NewResponseAction()
	ctx := newTestContext(map[string]any{})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	m := out.(map[string]any)
	s.Equal(200, m["status"])
}
