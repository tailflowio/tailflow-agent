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

func (s *ResponseActionTestSuite) TestExecute_CustomStatusAndHeaders() {
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

func (s *ResponseActionTestSuite) TestDefaults_Status200() {
	a := NewResponseAction()
	ctx := newTestContext(map[string]any{})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	m := out.(map[string]any)
	s.Equal(200, m["status"])
}

func (s *ResponseActionTestSuite) TestValidateAlwaysNil() {
	a := NewResponseAction()
	err := a.Validate(newTestContext(map[string]any{}))
	s.Nil(err)

	err = a.Validate(newTestContext(map[string]any{"status": 500, "body": "error"}))
	s.Nil(err)
}

func (s *ResponseActionTestSuite) TestExecuteFloat64Status() {
	a := NewResponseAction()
	ctx := newTestContext(map[string]any{
		"status": float64(404),
		"body":   "not found",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	m := out.(map[string]any)
	s.Equal(404, m["status"])
	s.Equal("not found", m["body"])
}

func (s *ResponseActionTestSuite) TestExecuteNoHeaders() {
	a := NewResponseAction()
	ctx := newTestContext(map[string]any{
		"status": 200,
		"body":   "ok",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	m := out.(map[string]any)
	s.Equal(200, m["status"])
	s.Equal("ok", m["body"])
	headers := m["headers"].(map[string]string)
	s.Empty(headers)
}
