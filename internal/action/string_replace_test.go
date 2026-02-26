package action

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type StringReplaceActionTestSuite struct {
	suite.Suite
}

func TestStringReplaceAction(t *testing.T) {
	suite.Run(t, new(StringReplaceActionTestSuite))
}

func (s *StringReplaceActionTestSuite) SetupTest() {}

func (s *StringReplaceActionTestSuite) TestMissingInput() {
	a := NewStringReplaceAction()
	ctx := newTestContext(map[string]any{"pattern": "foo"})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "input")
}

func (s *StringReplaceActionTestSuite) TestMissingPattern() {
	a := NewStringReplaceAction()
	ctx := newTestContext(map[string]any{"input": "foo"})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "pattern")
}

func (s *StringReplaceActionTestSuite) TestInvalidRegex() {
	a := NewStringReplaceAction()
	ctx := newTestContext(map[string]any{"input": "foo", "pattern": "[invalid"})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "invalid pattern")
}

func (s *StringReplaceActionTestSuite) TestSimpleReplace() {
	a := NewStringReplaceAction()
	ctx := newTestContext(map[string]any{
		"input":   "hello world",
		"pattern": "world",
	})

	s.Require().NoError(a.Validate(ctx))

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.Equal("hello ", out.(map[string]any)["result"])
}

func (s *StringReplaceActionTestSuite) TestRegexTimestamp() {
	a := NewStringReplaceAction()
	ctx := newTestContext(map[string]any{
		"input":   "## En direct à 10:50 ### Trafic normal sur le RER A",
		"pattern": `En direct à \d{1,2}:\d{2}`,
	})

	s.Require().NoError(a.Validate(ctx))

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.Equal("##  ### Trafic normal sur le RER A", out.(map[string]any)["result"])
}

func (s *StringReplaceActionTestSuite) TestWithReplacement() {
	a := NewStringReplaceAction()
	ctx := newTestContext(map[string]any{
		"input":       "prix: 42€",
		"pattern":     `\d+`,
		"replacement": "XX",
	})

	s.Require().NoError(a.Validate(ctx))

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.Equal("prix: XX€", out.(map[string]any)["result"])
}

func (s *StringReplaceActionTestSuite) TestMultiplePatterns() {
	a := NewStringReplaceAction()
	ctx := newTestContext(map[string]any{
		"input": "## En direct à 10:50 ### Mis à jour le 14/02/2026 Trafic OK",
		"pattern": []any{
			`En direct à \d{1,2}:\d{2}`,
			`Mis à jour le \d{2}/\d{2}/\d{4}`,
		},
	})

	s.Require().NoError(a.Validate(ctx))

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.Equal("##  ###  Trafic OK", out.(map[string]any)["result"])
}

func (s *StringReplaceActionTestSuite) TestValidateOK() {
	a := NewStringReplaceAction()
	err := a.Validate(newTestContext(map[string]any{
		"input":   "hello",
		"pattern": "world",
	}))
	s.NoError(err)
}

func (s *StringReplaceActionTestSuite) TestValidateInvalidRegexInArray() {
	a := NewStringReplaceAction()
	err := a.Validate(newTestContext(map[string]any{
		"input":   "hello",
		"pattern": []any{"[invalid"},
	}))
	s.Error(err)
	s.Contains(err.Error(), "invalid pattern")
}

func (s *StringReplaceActionTestSuite) TestValidateValidArrayPattern() {
	a := NewStringReplaceAction()
	err := a.Validate(newTestContext(map[string]any{
		"input":   "hello",
		"pattern": []any{`\d+`, `[a-z]+`},
	}))
	s.NoError(err)
}
