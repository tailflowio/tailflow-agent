package action

import (
	"encoding/json"
	"errors"
	"fmt"
)

// JSONDecodeAction parses JSON strings.
type JSONDecodeAction struct{}

func NewJSONDecodeAction() Action { return &JSONDecodeAction{} }

func (a *JSONDecodeAction) Validate(ctx *ActionContext) error {
	if _, ok := ctx.Config["input"]; !ok {
		return errors.New("json.decode action requires 'input' in config")
	}

	return nil
}

func (a *JSONDecodeAction) Execute(ctx *ActionContext) (any, error) {
	input := fmt.Sprintf("%v", ctx.Config["input"])
	var result any

	err := json.Unmarshal([]byte(input), &result)
	if err != nil {
		extracted := extractJSON(input)
		if extracted == "" {
			extracted = input
		}
		sanitized := sanitizeJSONStrings(extracted)
		err2 := json.Unmarshal([]byte(sanitized), &result)
		if err2 == nil {
			return result, nil
		}
		preview := input
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		return nil, fmt.Errorf("json.decode: %w (input: %s)", err, preview)
	}

	return result, nil
}

// extractJSON tries to find the first JSON object or array in a string.
func extractJSON(s string) string {
	// Find first { or [
	start := -1
	opener := byte(0)
	for i := 0; i < len(s); i++ {
		if s[i] == '{' || s[i] == '[' {
			start = i
			opener = s[i]
			break
		}
	}
	if start == -1 {
		return ""
	}

	closer := byte('}')
	if opener == '[' {
		closer = ']'
	}

	// Find matching closer, accounting for nesting and strings
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
		if ch == opener {
			depth++
		} else if ch == closer {
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

// sanitizeJSONStrings escapes literal control characters (newlines, tabs)
// inside JSON string values, which LLMs often produce.
func sanitizeJSONStrings(s string) string {
	var buf []byte
	inString := false
	escaped := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if escaped {
			escaped = false
			buf = append(buf, ch)
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			buf = append(buf, ch)
			continue
		}
		if ch == '"' {
			inString = !inString
			buf = append(buf, ch)
			continue
		}
		if inString {
			switch ch {
			case '\n':
				buf = append(buf, '\\', 'n')
			case '\r':
				buf = append(buf, '\\', 'r')
			case '\t':
				buf = append(buf, '\\', 't')
			default:
				buf = append(buf, ch)
			}
		} else {
			buf = append(buf, ch)
		}
	}
	return string(buf)
}

// JSONEncodeAction serializes values to JSON.
type JSONEncodeAction struct{}

func NewJSONEncodeAction() Action { return &JSONEncodeAction{} }

func (a *JSONEncodeAction) Validate(ctx *ActionContext) error {
	if _, ok := ctx.Config["input"]; !ok {
		return errors.New("json.encode action requires 'input' in config")
	}

	return nil
}

func (a *JSONEncodeAction) Execute(ctx *ActionContext) (any, error) {
	input := ctx.Config["input"]
	pretty := false

	if p, ok := ctx.Config["pretty"]; ok {
		if pb, ok := p.(bool); ok {
			pretty = pb
		}
	}

	var data []byte

	var err error
	if pretty {
		data, err = json.MarshalIndent(input, "", "  ")
	} else {
		data, err = json.Marshal(input)
	}

	if err != nil {
		return nil, fmt.Errorf("json.encode: %w", err)
	}

	return string(data), nil
}
