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

func (s *JSONActionTestSuite) TestDecodeValidateMissingInput() {
	a := NewJSONDecodeAction()
	err := a.Validate(newTestContext(map[string]any{}))
	s.Error(err)
	s.Contains(err.Error(), "input")
}

func (s *JSONActionTestSuite) TestEncodeValidateMissingInput() {
	a := NewJSONEncodeAction()
	err := a.Validate(newTestContext(map[string]any{}))
	s.Error(err)
	s.Contains(err.Error(), "input")
}

func (s *JSONActionTestSuite) TestDecodeValidateOK() {
	a := NewJSONDecodeAction()
	err := a.Validate(newTestContext(map[string]any{"input": "{}"}))
	s.NoError(err)
}

func (s *JSONActionTestSuite) TestEncodeValidateOK() {
	a := NewJSONEncodeAction()
	err := a.Validate(newTestContext(map[string]any{"input": "value"}))
	s.NoError(err)
}

func (s *JSONActionTestSuite) TestDecodeExtractArray() {
	a := NewJSONDecodeAction()
	ctx := newTestContext(map[string]any{
		"input": "Result: [1, 2, 3] done",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	arr := out.([]any)
	s.Len(arr, 3)
}

func (s *JSONActionTestSuite) TestDecodeNoJSON() {
	a := NewJSONDecodeAction()
	ctx := newTestContext(map[string]any{
		"input": "no json here at all, just plain text without braces or brackets",
	})

	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "json.decode")
}

func (s *JSONActionTestSuite) TestDecodeLongInputPreview() {
	a := NewJSONDecodeAction()
	longInput := "not json " + string(make([]byte, 250))
	ctx := newTestContext(map[string]any{
		"input": longInput,
	})

	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "...")
}

func (s *JSONActionTestSuite) TestDecodeSanitizeWithTabs() {
	a := NewJSONDecodeAction()
	// JSON with literal tab characters inside string values
	ctx := newTestContext(map[string]any{
		"input": "```json\n{\"key\": \"value\twith\ttabs\"}\n```",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	m := out.(map[string]any)
	s.Contains(m["key"], "value")
}

func (s *JSONActionTestSuite) TestDecodeSanitizeWithCarriageReturn() {
	a := NewJSONDecodeAction()
	// JSON with literal \r inside string values
	ctx := newTestContext(map[string]any{
		"input": "```json\n{\"key\": \"line1\r\nline2\"}\n```",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	m := out.(map[string]any)
	s.Contains(m["key"], "line1")
}

func (s *JSONActionTestSuite) TestDecodeSanitizeEscapedBackslash() {
	a := NewJSONDecodeAction()
	// Properly escaped backslash followed by quotes
	ctx := newTestContext(map[string]any{
		"input": `{"path": "C:\\Users\\test"}`,
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	m := out.(map[string]any)
	s.Equal("C:\\Users\\test", m["path"])
}

func (s *JSONActionTestSuite) TestEncodeNotPretty() {
	a := NewJSONEncodeAction()
	ctx := newTestContext(map[string]any{
		"input":  map[string]any{"a": 1},
		"pretty": false,
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.NotContains(out.(string), "\n")
}

func (s *JSONActionTestSuite) TestExtractJSONUnmatched() {
	// Test extractJSON with unmatched braces
	result := extractJSON("{unmatched")
	s.Equal("", result)
}

func (s *JSONActionTestSuite) TestExtractJSONWithEscapedQuotes() {
	result := extractJSON(`prefix {"key": "value with \"quotes\""} suffix`)
	s.Equal(`{"key": "value with \"quotes\""}`, result)
}

func (s *JSONActionTestSuite) TestSanitizeJSONStringsOutsideString() {
	// Characters outside string values should pass through unchanged
	result := sanitizeJSONStrings(`{"key": "value"}`)
	s.Equal(`{"key": "value"}`, result)
}

func (s *JSONActionTestSuite) TestSanitizeJSONStringsEscapedChar() {
	// Escaped characters inside strings should be preserved
	result := sanitizeJSONStrings(`{"key": "val\\nue"}`)
	s.Equal(`{"key": "val\\nue"}`, result)
}

func (s *JSONActionTestSuite) TestSanitizeJSONStringsAllControlChars() {
	// Test all control characters: \n, \r, \t
	input := "{\"key\": \"line1\nline2\rline3\tline4\"}"
	result := sanitizeJSONStrings(input)
	s.Contains(result, "\\n")
	s.Contains(result, "\\r")
	s.Contains(result, "\\t")
}

func (s *JSONActionTestSuite) TestExtractJSONArrayBracket() {
	result := extractJSON("prefix [1, 2, 3] suffix")
	s.Equal("[1, 2, 3]", result)
}

func (s *JSONActionTestSuite) TestExtractJSONNoJSON() {
	result := extractJSON("no json here")
	s.Equal("", result)
}

func (s *JSONActionTestSuite) TestExtractJSONNestedWithStrings() {
	result := extractJSON(`text {"a": "b{c}d"} more`)
	s.Equal(`{"a": "b{c}d"}`, result)
}

func (s *JSONActionTestSuite) TestEncodeMarshalError() {
	a := NewJSONEncodeAction()
	ctx := newTestContext(map[string]any{
		"input": make(chan int), // channels can't be marshaled
	})

	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "json.encode")
}

func (s *JSONActionTestSuite) TestEncodePrettyMarshalError() {
	a := NewJSONEncodeAction()
	ctx := newTestContext(map[string]any{
		"input":  make(chan int),
		"pretty": true,
	})

	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "json.encode")
}

func (s *JSONActionTestSuite) TestEncodeNonPrettyBoolFalse() {
	a := NewJSONEncodeAction()
	ctx := newTestContext(map[string]any{
		"input":  map[string]any{"a": 1},
		"pretty": "not-a-bool",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.NotContains(out.(string), "\n")
}
