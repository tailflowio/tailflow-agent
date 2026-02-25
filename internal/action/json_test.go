package action

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type JSONActionTestSuite struct {
	suite.Suite
}

func TestJSONAction(t *testing.T) {
	suite.Run(t, new(JSONActionTestSuite))
}

func (s *JSONActionTestSuite) SetupTest() {}

func (s *JSONActionTestSuite) TestDecodeExecute() {
	a := NewJSONDecodeAction()
	ctx := newTestContext(map[string]any{
		"input": `{"name":"Alice","age":30}`,
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	m := out.(map[string]any)
	s.Equal("Alice", m["name"])
	s.Equal(float64(30), m["age"])
}

func (s *JSONActionTestSuite) TestDecodeInvalidJSON() {
	a := NewJSONDecodeAction()
	ctx := newTestContext(map[string]any{
		"input": "not json",
	})

	_, err := a.Execute(ctx)
	s.Error(err)
}

func (s *JSONActionTestSuite) TestDecodeExtractFromCodeBlock() {
	a := NewJSONDecodeAction()
	ctx := newTestContext(map[string]any{
		"input": "```json\n{\"title\":\"Hello\",\"content\":\"World\"}\n```",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	m := out.(map[string]any)
	s.Equal("Hello", m["title"])
	s.Equal("World", m["content"])
}

func (s *JSONActionTestSuite) TestDecodeExtractFromText() {
	a := NewJSONDecodeAction()
	ctx := newTestContext(map[string]any{
		"input": "Here is the result:\n{\"key\": \"value\"}\nDone.",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	m := out.(map[string]any)
	s.Equal("value", m["key"])
}

func (s *JSONActionTestSuite) TestDecodeNestedJSON() {
	a := NewJSONDecodeAction()
	ctx := newTestContext(map[string]any{
		"input": "```json\n{\"a\":{\"b\":1},\"c\":[1,2]}\n```",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	m := out.(map[string]any)
	s.Equal(float64(1), m["a"].(map[string]any)["b"])
}

func (s *JSONActionTestSuite) TestDecodeLLMNewlinesInStrings() {
	a := NewJSONDecodeAction()
	// LLM returns code block with literal newlines inside string values
	ctx := newTestContext(map[string]any{
		"input": "```json\n{\n  \"title\": \"Mon titre\",\n  \"content\": \"Premier paragraphe.\n\nDeuxième paragraphe.\nTroisième ligne.\"\n}\n```",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	m := out.(map[string]any)
	s.Equal("Mon titre", m["title"])
	s.Contains(m["content"], "Premier paragraphe.")
	s.Contains(m["content"], "Deuxième paragraphe.")
}

func (s *JSONActionTestSuite) TestEncodeExecute() {
	a := NewJSONEncodeAction()
	ctx := newTestContext(map[string]any{
		"input": map[string]any{"name": "Alice"},
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.Equal(`{"name":"Alice"}`, out)
}

func (s *JSONActionTestSuite) TestEncodePretty() {
	a := NewJSONEncodeAction()
	ctx := newTestContext(map[string]any{
		"input":  map[string]any{"name": "Alice"},
		"pretty": true,
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.Contains(out.(string), "\n")
}
