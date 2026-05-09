// Package clickhouse implements a ClickHouse backend for the
// store.ExecutionStore contract.
//
// ClickHouse is a columnar OLAP store: it has no transactions and no
// row-level locks. Updates are modelled as INSERTs of new versions; the
// background MergeTree merge collapses duplicates by primary key, so reads
// must use FINAL (or argMax aggregation) to see the latest snapshot.
//
// The agent has a single writer per execution in practice, so the simpler
// "insert new version, read with FINAL" pattern is safe for our concurrency.
package clickhouse

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
)

// DefaultTablePrefix is applied when the workflow YAML omits one.
const DefaultTablePrefix = "tailflow_"

var validPrefixRe = regexp.MustCompile(`^[A-Za-z0-9_]*$`)

func validatePrefix(prefix string) error {
	if !validPrefixRe.MatchString(prefix) {
		return fmt.Errorf("clickhouse: invalid table_prefix %q (allowed: A-Z a-z 0-9 _)", prefix)
	}

	return nil
}

// migrate creates every table the connector needs. The DDL is idempotent.
func migrate(ctx context.Context, db *sql.DB, prefix string) error {
	for _, stmt := range schemaStatements(prefix) {
		_, err := db.ExecContext(ctx, stmt)
		if err != nil {
			return fmt.Errorf("clickhouse migrate: %w", err)
		}
	}

	return nil
}

// schemaStatements returns the DDL needed to provision the tables.
//
// executions uses ReplacingMergeTree(version) so that subsequent
// UpdateExecution / UpdateStep calls insert a new row with a bumped
// version; the background merge keeps only the latest per id.
//
// events uses MergeTree (append-only) ordered by (execution_id, seq) so
// chronological replay is a single-partition scan.
//
// step_exec_counts uses SummingMergeTree(cnt) so IncrStepExecCount is a
// dirt-cheap INSERT VALUES(step_id, 1) — the engine sums on merge.
func schemaStatements(prefix string) []string {
	return []string{
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %sexecutions (
    id              String,
    workflow_name   String,
    status          String,
    params          String,
    steps           String,
    started_at      DateTime64(6),
    finished_at     Nullable(DateTime64(6)),
    error_msg       String,
    version         DateTime64(6) DEFAULT now64(6)
) ENGINE = ReplacingMergeTree(version)
ORDER BY id`, prefix),

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %sevents (
    seq           UInt64,
    execution_id  String,
    event_type    String,
    step_id       String,
    message       String,
    data          String,
    timestamp     DateTime64(6)
) ENGINE = MergeTree
ORDER BY (execution_id, seq)`, prefix),

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %sstep_exec_counts (
    step_id  String,
    cnt      UInt64
) ENGINE = SummingMergeTree(cnt)
ORDER BY step_id`, prefix),
	}
}
