package api

// RunRequest is the payload for starting a workflow execution.
type RunRequest struct {
	Params map[string]any `json:"params,omitempty"`
}

// RunResponse is the response after starting a workflow execution.
type RunResponse struct {
	ExecutionID string `json:"execution_id"`
	Status      string `json:"status"`
}

// ValidateResponse is the response for workflow validation.
type ValidateResponse struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors,omitempty"`
}

// ErrorResponse is a generic error response.
type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}
