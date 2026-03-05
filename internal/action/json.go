package action

import (
	"encoding/json"
	"errors"
	"fmt"
)

type JSONDecodeAction struct{}

func NewJSONDecodeAction() Action { return &JSONDecodeAction{} }

func (a *JSONDecodeAction) Validate(ctx *ActionContext) error {
	_, ok := ctx.Config["input"]
	if !ok {
		return errors.New("json.decode action requires 'input' in config")
	}

	return nil
}

func (a *JSONDecodeAction) Execute(ctx *ActionContext) (any, error) {
	input := fmt.Sprintf("%v", ctx.Config["input"])
	var result any

	err := json.Unmarshal([]byte(input), &result)
	if err == nil {
		return result, nil
	}

	return tryRecoverJSON(input, err)
}

func tryRecoverJSON(input string, originalErr error) (any, error) {
	extracted := extractJSON(input)
	if extracted == "" {
		extracted = input
	}

	sanitized := sanitizeJSONStrings(extracted)

	var result any

	err := json.Unmarshal([]byte(sanitized), &result)
	if err == nil {
		return result, nil
	}

	preview := input
	if len(preview) > 200 {
		preview = preview[:200] + "..."
	}

	return nil, fmt.Errorf("json.decode: %w (input: %s)", originalErr, preview)
}

func extractJSON(s string) string {
	start, opener := findJSONStart(s)
	if start == -1 {
		return ""
	}

	closer := byte('}')
	if opener == '[' {
		closer = ']'
	}

	return findMatchingClose(s, start, opener, closer)
}

func findJSONStart(s string) (int, byte) {
	for i := 0; i < len(s); i++ {
		if s[i] == '{' || s[i] == '[' {
			return i, s[i]
		}
	}

	return -1, 0
}

func findMatchingClose(s string, start int, opener, closer byte) string {
	depth := 0
	inString := false
	escaped := false

	for i := start; i < len(s); i++ {
		if escaped {
			escaped = false
			continue
		}

		ch := s[i]

		if ch == '\\' && inString {
			escaped = true
			continue
		}

		if ch == '"' {
			inString = !inString
			continue
		}

		if inString {
			continue
		}

		switch ch {
		case opener:
			depth++
		case closer:
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}

	return ""
}

func sanitizeJSONStrings(s string) string {
	buf := make([]byte, 0, len(s))
	inString := false
	escaped := false

	for i := 0; i < len(s); i++ {
		ch := s[i]
		appendCh := true

		switch {
		case escaped:
			escaped = false
		case ch == '\\' && inString:
			escaped = true
		case ch == '"':
			inString = !inString
		case inString && ch == '\n':
			buf = append(buf, '\\', 'n')
			appendCh = false
		case inString && ch == '\r':
			buf = append(buf, '\\', 'r')
			appendCh = false
		case inString && ch == '\t':
			buf = append(buf, '\\', 't')
			appendCh = false
		}

		if appendCh {
			buf = append(buf, ch)
		}
	}

	return string(buf)
}

type JSONEncodeAction struct{}

func NewJSONEncodeAction() Action { return &JSONEncodeAction{} }

func (a *JSONEncodeAction) Validate(ctx *ActionContext) error {
	_, ok := ctx.Config["input"]
	if !ok {
		return errors.New("json.encode action requires 'input' in config")
	}

	return nil
}

func (a *JSONEncodeAction) Execute(ctx *ActionContext) (any, error) {
	input := ctx.Config["input"]

	pretty, _ := ctx.Config["pretty"].(bool)

	if pretty {
		data, err := json.MarshalIndent(input, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("json.encode: %w", err)
		}

		return string(data), nil
	}

	data, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("json.encode: %w", err)
	}

	return string(data), nil
}
