package action

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPAction performs HTTP requests.
type HTTPAction struct{}

func NewHTTPAction() Action { return &HTTPAction{} }

func (a *HTTPAction) Validate(ctx *ActionContext) error {
	if _, ok := ctx.Config["url"]; !ok {
		return errors.New("http action requires 'url' in config")
	}

	return nil
}

func (a *HTTPAction) Execute(ctx *ActionContext) (any, error) {
	method := "GET"
	if m, ok := ctx.Config["method"]; ok {
		method = strings.ToUpper(fmt.Sprintf("%v", m))
	}

	url := fmt.Sprintf("%v", ctx.Config["url"])

	var bodyReader io.Reader

	if body, ok := ctx.Config["body"]; ok {
		switch b := body.(type) {
		case string:
			bodyReader = strings.NewReader(b)
		default:
			data, err := json.Marshal(b)
			if err != nil {
				return nil, fmt.Errorf("http: marshal body: %w", err)
			}

			bodyReader = bytes.NewReader(data)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("http: create request: %w", err)
	}

	// Set headers
	if headers, ok := ctx.Config["headers"]; ok {
		if hMap, ok := headers.(map[string]any); ok {
			for k, v := range hMap {
				req.Header.Set(k, fmt.Sprintf("%v", v))
			}
		}
	}

	// Default content-type for body
	if bodyReader != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// Use config timeout if provided, otherwise check if the step context
	// already has a deadline (from step-level timeout). Only fall back to 30s
	// default when neither is set.
	var clientTimeout time.Duration

	if t, ok := ctx.Config["timeout"]; ok {
		if ts, ok := t.(string); ok {
			if d, parseErr := time.ParseDuration(ts); parseErr == nil {
				clientTimeout = d
			}
		}
	}

	if clientTimeout == 0 {
		if _, hasDeadline := ctx.Deadline(); hasDeadline {
			clientTimeout = 0 // context deadline will handle it
		} else {
			clientTimeout = 30 * time.Second
		}
	}

	client := &http.Client{Timeout: clientTimeout}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: request failed: %w", err)
	}

	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("http: read response: %w", err)
	}

	output := map[string]any{
		"status":      resp.StatusCode,
		"status_text": resp.Status,
		"headers":     headerToMap(resp.Header),
	}

	// Try to parse as JSON
	var jsonBody any
	err = json.Unmarshal(respBody, &jsonBody)
	if err == nil {
		output["body"] = jsonBody
	} else {
		output["body"] = string(respBody)
	}

	// Check expected status
	if expect, ok := ctx.Config["expect"]; ok {
		if expectMap, ok := expect.(map[string]any); ok {
			if expectedStatus, ok := expectMap["status"]; ok {
				switch es := expectedStatus.(type) {
				case int:
					if es != resp.StatusCode {
						return output, fmt.Errorf("http: expected status %d, got %d", es, resp.StatusCode)
					}
				case float64:
					if int(es) != resp.StatusCode {
						return output, fmt.Errorf("http: expected status %d, got %d", int(es), resp.StatusCode)
					}
				}
			}
		}
	}

	return output, nil
}

func headerToMap(h http.Header) map[string]string {
	m := make(map[string]string, len(h))
	for k := range h {
		m[k] = h.Get(k)
	}

	return m
}
