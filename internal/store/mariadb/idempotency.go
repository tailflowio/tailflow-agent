package mariadb

import (
	"context"
	"errors"
	"fmt"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"

	"github.com/tailflow/tailflow/internal/export"
	"github.com/tailflow/tailflow/internal/runtime"
)

const mysqlErrDuplicateEntry = 1062

var _ export.IdempotencyClaimer = (*Store)(nil)

// ClaimExecution attempts to insert a running execution row for the
// (workflowName, idempotencyKey) pair. The UNIQUE constraint on those columns
// is the sole arbiter of deduplication: a successful INSERT grants the claim,
// while a duplicate-entry error (1062) means another execution already won it,
// in which case the existing row is returned with Claimed=false. No
// transaction or GET_LOCK is used; the agent is mono-instance.
func (s *Store) ClaimExecution(
	ctx context.Context,
	executionID, workflowName, idempotencyKey string,
) (*export.ClaimResult, error) {
	q := fmt.Sprintf(`INSERT INTO %sexecutions
        (id, workflow_name, status, started_at, idempotency_key)
        VALUES (?, ?, ?, ?, ?)`, s.prefix)

	_, err := s.db.ExecContext(ctx, q,
		executionID,
		workflowName,
		runtime.StatusRunning,
		time.Now().UTC(),
		idempotencyKey,
	)
	if err == nil {
		return &export.ClaimResult{Claimed: true}, nil
	}

	var myErr *mysqldriver.MySQLError

	isDuplicate := errors.As(err, &myErr) && myErr.Number == mysqlErrDuplicateEntry
	if !isDuplicate {
		return nil, fmt.Errorf("mariadb claim: %w", err)
	}

	return s.lookupClaim(ctx, workflowName, idempotencyKey)
}

// lookupClaim resolves the execution that already won the claim for the given
// (workflowName, idempotencyKey) pair.
func (s *Store) lookupClaim(
	ctx context.Context,
	workflowName, idempotencyKey string,
) (*export.ClaimResult, error) {
	q := fmt.Sprintf(`SELECT id, status
        FROM %sexecutions
        WHERE workflow_name = ? AND idempotency_key = ?`, s.prefix)

	var (
		existingID     string
		existingStatus string
	)

	err := s.db.QueryRowContext(ctx, q, workflowName, idempotencyKey).Scan(
		&existingID,
		&existingStatus,
	)
	if err != nil {
		return nil, fmt.Errorf("mariadb claim: lookup: %w", err)
	}

	return &export.ClaimResult{
		Claimed:             false,
		ExistingExecutionID: existingID,
		ExistingStatus:      existingStatus,
	}, nil
}
