package saas

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/tailflow/tailflow/internal/export"
	"github.com/tailflow/tailflow/internal/runtime"
)

type rawStep struct {
	StepID       string  `json:"step_id"`
	Status       string  `json:"status"`
	OutputData   string  `json:"output_data,omitempty"`
	ErrorMessage string  `json:"error_message,omitempty"`
	ErrorCode    string  `json:"error_code,omitempty"`
	StartedAt    *string `json:"started_at,omitempty"`
	FinishedAt   *string `json:"finished_at,omitempty"`
}

type RecoveryClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewRecoveryClient(baseURL, apiKey string) *RecoveryClient {
	return &RecoveryClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (rc *RecoveryClient) RecoverExecutions(ctx context.Context, agentID string) ([]export.RecoveredExecution, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rc.baseURL+"/api/v1/agent/recovery?agent_id="+url.QueryEscape(agentID), nil)
	if err != nil {
		return nil, fmt.Errorf("recovery: build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+rc.apiKey)

	resp, err := rc.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("recovery: http call: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("recovery: unexpected status %d", resp.StatusCode)
	}

	var execs []export.RecoveredExecution

	err = json.NewDecoder(resp.Body).Decode(&execs)
	if err != nil {
		return nil, fmt.Errorf("recovery: decode response: %w", err)
	}

	for i := range execs {
		if execs[i].RawParams != "" {
			unmarshalErr := json.Unmarshal([]byte(execs[i].RawParams), &execs[i].Params)
			if unmarshalErr != nil {
				return nil, fmt.Errorf("recovery: decode params for %s: %w", execs[i].ExecutionID, unmarshalErr)
			}
		}

		execs[i].Steps = parseSteps(execs[i].RawSteps)
	}

	return execs, nil
}

func parseSteps(raw json.RawMessage) map[string]*runtime.StepResult {
	if len(raw) == 0 {
		return nil
	}

	var arr []rawStep

	err := json.Unmarshal(raw, &arr)
	if err == nil && len(arr) > 0 {
		return convertRawSteps(arr)
	}

	var stepMap map[string]*runtime.StepResult

	mapErr := json.Unmarshal(raw, &stepMap)
	if mapErr == nil {
		return stepMap
	}

	return nil
}

func convertRawSteps(arr []rawStep) map[string]*runtime.StepResult {
	result := make(map[string]*runtime.StepResult, len(arr))

	for _, s := range arr {
		sr := &runtime.StepResult{
			Status: s.Status,
		}

		if s.OutputData != "" {
			var out any

			outErr := json.Unmarshal([]byte(s.OutputData), &out)
			if outErr == nil {
				sr.Output = out
			}
		}

		if s.ErrorMessage != "" {
			sr.Error = &runtime.StepError{
				Message: s.ErrorMessage,
				Code:    s.ErrorCode,
				StepID:  s.StepID,
			}
		}

		if s.StartedAt != nil {
			t, parseErr := time.Parse(time.RFC3339, *s.StartedAt)
			if parseErr == nil {
				sr.StartedAt = &t
			}
		}

		if s.FinishedAt != nil {
			t, parseErr := time.Parse(time.RFC3339, *s.FinishedAt)
			if parseErr == nil {
				sr.FinishedAt = &t
			}
		}

		result[s.StepID] = sr
	}

	return result
}
