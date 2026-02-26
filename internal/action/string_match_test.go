package action

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type StringMatchAllActionTestSuite struct {
	suite.Suite
}

func TestStringMatchAllAction(t *testing.T) {
	suite.Run(t, new(StringMatchAllActionTestSuite))
}

func (s *StringMatchAllActionTestSuite) SetupTest() {}

func (s *StringMatchAllActionTestSuite) TestValidate_MissingFields() {
	a := NewStringMatchAllAction()

	s.Error(a.Validate(newTestContext(map[string]any{})))
	s.Error(a.Validate(newTestContext(map[string]any{"input": "x"})))
	s.Error(a.Validate(newTestContext(map[string]any{"input": "x", "pattern": "[invalid"})))
	s.NoError(a.Validate(newTestContext(map[string]any{"input": "x", "pattern": "x"})))
}

func (s *StringMatchAllActionTestSuite) TestSimplePattern() {
	a := NewStringMatchAllAction()
	ctx := newTestContext(map[string]any{
		"input":   "foo 123 bar 456 baz 123",
		"pattern": `\d+`,
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	m := out.(map[string]any)
	matches := m["matches"].([]any)
	s.Equal([]any{"123", "456"}, matches) // deduplicated
}

func (s *StringMatchAllActionTestSuite) TestCaptureGroup() {
	a := NewStringMatchAllAction()
	ctx := newTestContext(map[string]any{
		"input":   "[Article One](https://example.com/a) and [Article Two](https://example.com/b)",
		"pattern": `\[.*?\]\((https://example\.com/[^)]+)\)`,
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	m := out.(map[string]any)
	matches := m["matches"].([]any)
	s.Equal([]any{"https://example.com/a", "https://example.com/b"}, matches)
}

func (s *StringMatchAllActionTestSuite) TestCaptureGroupDeduplicated() {
	a := NewStringMatchAllAction()
	ctx := newTestContext(map[string]any{
		"input":   "[A](https://example.com/a) [B](https://example.com/a) [C](https://example.com/b)",
		"pattern": `\[.*?\]\((https://example\.com/[^)]+)\)`,
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	m := out.(map[string]any)
	matches := m["matches"].([]any)
	s.Equal([]any{"https://example.com/a", "https://example.com/b"}, matches) // deduplicated
}

func (s *StringMatchAllActionTestSuite) TestNoMatch() {
	a := NewStringMatchAllAction()
	ctx := newTestContext(map[string]any{
		"input":   "no numbers here",
		"pattern": `\d+`,
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	m := out.(map[string]any)
	matches := m["matches"].([]any)
	s.Equal([]any{}, matches)
}
