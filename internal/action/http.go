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

	tfotel "github.com/tailflow/tailflow/internal/otel"
)

type HTTPAction struct{}

func NewHTTPAction() Action { return &HTTPAction{} }

func (a *HTTPAction) Validate(ctx *ActionContext) error {
	_, ok := ctx.Config["url"]
	if !ok {
		return errors.New("http action requires 'url' in config")
	}

	return nil
}

func (a *HTTPAction) Execute(ctx *ActionContext) (any, error) {
	method := "GET"

	m, ok := ctx.Config["method"]
	if ok {
		method = strings.ToUpper(fmt.Sprintf("%v", m))
	}

	url := fmt.Sprintf("%v", ctx.Config["url"])

	bodyReader, err := buildHTTPBody(ctx)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("http: create request: %w", err)
	}

	applyHTTPHeaders(req, ctx, bodyReader)
	tfotel.InjectTraceContext(req)

	client := &http.Client{Timeout: resolveHTTPTimeout(ctx)}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: request failed: %w", err)
	}
	defer resp.Body.Close()

	output, err := parseHTTPResponse(resp)
	if err != nil {
		return nil, err
	}

	err = checkExpectedStatus(ctx, output, resp.StatusCode)
	if err != nil {
		return output, err
	}

	return output, nil
}

func buildHTTPBody(ctx *ActionContext) (io.Reader, error) {
	body, ok := ctx.Config["body"]
	if !ok {
		return nil, nil
	}

	b, ok := body.(string)
	if ok {
		return strings.NewReader(b), nil
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("http: marshal body: %w", err)
	}

	return bytes.NewReader(data), nil
}

func applyHTTPHeaders(req *http.Request, ctx *ActionContext, bodyReader io.Reader) {
	headers, ok := ctx.Config["headers"]
	if ok {
		hMap, ok := headers.(map[string]any)
		if ok {
			for k, v := range hMap {
				req.Header.Set(k, fmt.Sprintf("%v", v))
			}
		}
	}

	if bodyReader != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
}

func resolveHTTPTimeout(ctx *ActionContext) time.Duration {
	d, ok := parseConfigDuration(ctx)
	if ok {
		return d
	}

	_, hasDeadline := ctx.Deadline()
	if hasDeadline {
		return 0
	}

	return 30 * time.Second
}

func parseConfigDuration(ctx *ActionContext) (time.Duration, bool) {
	t, ok := ctx.Config["timeout"]
	if !ok {
		return 0, false
	}

	ts, ok := t.(string)
	if !ok {
		return 0, false
	}

	d, err := time.ParseDuration(ts)
	if err != nil {
		return 0, false
	}

	return d, true
}

func parseHTTPResponse(resp *http.Response) (map[string]any, error) {
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("http: read response: %w", err)
	}

	output := map[string]any{
		"status":      resp.StatusCode,
		"status_text": resp.Status,
		"headers":     headerToMap(resp.Header),
	}

	var jsonBody any

	output["body"] = string(respBody)
	if json.Unmarshal(respBody, &jsonBody) == nil {
		output["body"] = jsonBody
	}

	return output, nil
}

func checkExpectedStatus(ctx *ActionContext, output map[string]any, statusCode int) error {
	expect, ok := ctx.Config["expect"]
	if !ok {
		return nil
	}

	expectMap, ok := expect.(map[string]any)
	if !ok {
		return nil
	}

	expectedStatus, ok := expectMap["status"]
	if !ok {
		return nil
	}

	var expected int

	switch es := expectedStatus.(type) {
	case int:
		expected = es
	case float64:
		expected = int(es)
	default:
		return nil
	}

	if expected == statusCode {
		return nil
	}

	body := truncateBody(output["body"], 512)

	return fmt.Errorf("http: expected status %d, got %d — %s", expected, statusCode, body)
}

func truncateBody(body any, maxLen int) string {
	var s string

	switch b := body.(type) {
	case string:
		s = b
	case nil:
		return "(empty body)"
	default:
		data, err := json.Marshal(b)
		if err != nil {
			return fmt.Sprintf("%v", b)
		}

		s = string(data)
	}

	if len(s) <= maxLen {
		return s
	}

	return s[:maxLen] + "…"
}

func headerToMap(h http.Header) map[string]string {
	m := make(map[string]string, len(h))
	for k := range h {
		m[k] = h.Get(k)
	}

	return m
}
