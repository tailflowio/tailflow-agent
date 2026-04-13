package export

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type ClaimResult struct {
	Claimed             bool   `json:"claimed"`
	ExistingExecutionID string `json:"execution_id,omitempty"`
	ExistingStatus      string `json:"status,omitempty"`
}

type claimBody struct {
	ExecutionID    string `json:"execution_id"`
	WorkflowName   string `json:"workflow_name"`
	IdempotencyKey string `json:"idempotency_key"`
}

type ClaimClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewClaimClient(baseURL, apiKey string) *ClaimClient {
	return &ClaimClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *ClaimClient) ClaimExecution(ctx context.Context, executionID, workflowName, idempotencyKey string) (*ClaimResult, error) {
	body, err := json.Marshal(claimBody{
		ExecutionID:    executionID,
		WorkflowName:   workflowName,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return &ClaimResult{Claimed: true}, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/executions/claim", bytes.NewReader(body))
	if err != nil {
		return &ClaimResult{Claimed: true}, nil
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return &ClaimResult{Claimed: true}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &ClaimResult{Claimed: true}, nil
	}

	var result ClaimResult

	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return &ClaimResult{Claimed: true}, nil
	}

	return &result, nil
}
