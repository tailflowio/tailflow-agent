package server

import (
	"context"
	"errors"

	"github.com/tailflow/tailflow/internal/export"
)

// stubClaimer scripts ClaimExecution responses for idempotency tests without
// reaching for an httptest mock or the deleted SaaS HTTP client.
type stubClaimer struct {
	result *export.ClaimResult
	err    error
}

func (c *stubClaimer) ClaimExecution(_ context.Context, _, _, _ string) (*export.ClaimResult, error) {
	return c.result, c.err
}

// stubRecoverer scripts the recovered executions returned to the server's
// recovery loop on startup.
type stubRecoverer struct {
	executions []export.RecoveredExecution
	err        error
}

func (r *stubRecoverer) RecoverExecutions(_ context.Context, _ string) ([]export.RecoveredExecution, error) {
	return r.executions, r.err
}

// recordingClaimer captures the executionID passed to ClaimExecution so a test
// can assert it matches the persisted execution row id.
type recordingClaimer struct {
	result        *export.ClaimResult
	capturedID    string
	capturedCalls int
}

func (c *recordingClaimer) ClaimExecution(_ context.Context, executionID, _, _ string) (*export.ClaimResult, error) {
	c.capturedID = executionID
	c.capturedCalls++

	return c.result, nil
}

var errStubClaim = errors.New("stub claim failure")
