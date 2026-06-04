package export

import (
	"context"
	"sync"

	"github.com/tailflow/tailflow/internal/runtime"
)

const claimKeySeparator = "\x00"

var _ IdempotencyClaimer = (*MemoryClaimer)(nil)

// MemoryClaimer is an in-memory IdempotencyClaimer for the memory backend. It
// deduplicates inbound triggers via a mutex-guarded check-and-set on a map
// keyed by (workflowName, idempotencyKey).
//
// The map is unbounded: acceptable in v1 for a mono-instance agent. TTL/cap is
// post-v1.
type MemoryClaimer struct {
	mu     sync.Mutex
	claims map[string]memoryClaim
}

type memoryClaim struct {
	executionID string
	status      string
}

// NewMemoryClaimer returns a MemoryClaimer with an empty claim map.
func NewMemoryClaimer() *MemoryClaimer {
	return &MemoryClaimer{claims: make(map[string]memoryClaim)}
}

// ClaimExecution grants the claim for a fresh (workflowName, idempotencyKey)
// pair and deduplicates any subsequent claim for the same pair against the
// first execution that won it.
func (c *MemoryClaimer) ClaimExecution(
	_ context.Context,
	executionID, workflowName, idempotencyKey string,
) (*ClaimResult, error) {
	key := workflowName + claimKeySeparator + idempotencyKey

	c.mu.Lock()
	defer c.mu.Unlock()

	existing, found := c.claims[key]
	if found {
		return &ClaimResult{
			Claimed:             false,
			ExistingExecutionID: existing.executionID,
			ExistingStatus:      existing.status,
		}, nil
	}

	c.claims[key] = memoryClaim{
		executionID: executionID,
		status:      runtime.StatusRunning,
	}

	return &ClaimResult{Claimed: true}, nil
}
