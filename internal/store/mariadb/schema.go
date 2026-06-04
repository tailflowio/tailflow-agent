// Package mariadb implements a MariaDB / MySQL backend for the
// store.ExecutionStore contract.
package mariadb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"

	mysqldriver "github.com/go-sql-driver/mysql"
)

// mysqlErrDuplicateColumn and mysqlErrDuplicateKeyName are the MySQL/MariaDB
// error codes returned when an ADD COLUMN / ADD UNIQUE KEY targets a column or
// key that already exists. MySQL<8.0 and MariaDB<10.0.2 lack "IF NOT EXISTS"
// on these clauses, so existing installs are migrated by tolerating these.
const (
	mysqlErrDuplicateColumn  = 1060
	mysqlErrDuplicateKeyName = 1061
)

// DefaultTablePrefix is applied when the workflow YAML omits one. All
// connector tables are named "<prefix>executions", "<prefix>events", etc.
const DefaultTablePrefix = "tailflow_"

// validPrefixRe matches table-prefix strings that are safe to interpolate
// into raw DDL. We restrict to identifier-like characters so the prefix
// can never be used as a SQL injection vector.
var validPrefixRe = regexp.MustCompile(`^[A-Za-z0-9_]*$`)

// validatePrefix returns an error if the prefix contains anything other
// than ASCII letters, digits or underscores.
func validatePrefix(prefix string) error {
	if !validPrefixRe.MatchString(prefix) {
		return fmt.Errorf("mariadb: invalid table_prefix %q (allowed: A-Z a-z 0-9 _)", prefix)
	}

	return nil
}

// migrate creates every table the connector needs. The DDL is idempotent
// (CREATE TABLE IF NOT EXISTS) so callers can run it on every boot. It then
// runs guarded ALTER statements so installs created before the idempotency
// column existed pick it up too.
func migrate(ctx context.Context, db *sql.DB, prefix string) error {
	for _, stmt := range schemaStatements(prefix) {
		_, err := db.ExecContext(ctx, stmt)
		if err != nil {
			return fmt.Errorf("mariadb migrate: %w", err)
		}
	}

	err := migrateIdempotencyColumn(ctx, db, prefix)
	if err != nil {
		return err
	}

	return nil
}

// migrateIdempotencyColumn adds the idempotency_key column and its UNIQUE key
// to a pre-existing executions table. "duplicate column" / "duplicate key
// name" errors mean the install is already migrated and are tolerated.
func migrateIdempotencyColumn(ctx context.Context, db *sql.DB, prefix string) error {
	for _, stmt := range idempotencyAlterStatements(prefix) {
		_, err := db.ExecContext(ctx, stmt)
		if isDuplicateSchemaErr(err) {
			continue
		}

		if err != nil {
			return fmt.Errorf("mariadb migrate: %w", err)
		}
	}

	return nil
}

// isDuplicateSchemaErr reports whether err is a MySQL/MariaDB duplicate column
// or duplicate key name error, signalling the schema change already exists.
func isDuplicateSchemaErr(err error) bool {
	if err == nil {
		return false
	}

	var myErr *mysqldriver.MySQLError

	ok := errors.As(err, &myErr)
	if !ok {
		return false
	}

	return myErr.Number == mysqlErrDuplicateColumn || myErr.Number == mysqlErrDuplicateKeyName
}

// idempotencyAlterStatements returns the guarded ALTERs that migrate an
// existing executions table to the idempotency schema.
func idempotencyAlterStatements(prefix string) []string {
	return []string{
		fmt.Sprintf(
			`ALTER TABLE %sexecutions ADD COLUMN idempotency_key VARCHAR(255) NULL`,
			prefix,
		),
		fmt.Sprintf(
			`ALTER TABLE %sexecutions ADD UNIQUE KEY uk_%sexec_idem (workflow_name, idempotency_key)`,
			prefix,
			prefix,
		),
	}
}

// schemaStatements returns the ordered DDL needed to provision the
// connector tables for the given prefix.
func schemaStatements(prefix string) []string {
	return []string{
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %sexecutions (
    id            VARCHAR(64)   NOT NULL,
    workflow_name VARCHAR(255)  NOT NULL,
    status        VARCHAR(32)   NOT NULL,
    params        JSON          NULL,
    steps         JSON          NULL,
    started_at    DATETIME(6)   NOT NULL,
    finished_at   DATETIME(6)   NULL,
    error_msg     TEXT          NULL,
    idempotency_key VARCHAR(255) NULL,
    created_seq   BIGINT        NOT NULL AUTO_INCREMENT UNIQUE,
    PRIMARY KEY (id),
    UNIQUE KEY uk_%sexec_idem (workflow_name, idempotency_key),
    INDEX idx_%sexec_status (status),
    INDEX idx_%sexec_started_at (started_at),
    INDEX idx_%sexec_created_seq (created_seq)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`, prefix, prefix, prefix, prefix, prefix),

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %sevents (
    seq           BIGINT        NOT NULL AUTO_INCREMENT,
    execution_id  VARCHAR(64)   NOT NULL,
    event_type    VARCHAR(64)   NOT NULL,
    step_id       VARCHAR(255)  NULL,
    message       TEXT          NULL,
    data          JSON          NULL,
    timestamp     DATETIME(6)   NOT NULL,
    PRIMARY KEY (seq),
    INDEX idx_%sevents_exec_seq (execution_id, seq)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`, prefix, prefix),

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %sstep_exec_counts (
    step_id  VARCHAR(255) NOT NULL,
    cnt      BIGINT       NOT NULL DEFAULT 0,
    PRIMARY KEY (step_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`, prefix),
	}
}
