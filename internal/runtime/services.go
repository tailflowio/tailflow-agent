package runtime

import (
	"context"
	"database/sql"
	"time"
)

type WaitRequest struct {
	Method  string              `json:"method"`
	Path    string              `json:"path"`
	Headers map[string]string   `json:"headers"`
	Query   map[string][]string `json:"query"`
	Body    any                 `json:"body"`
}

type KVStore interface {
	Get(ctx context.Context, key string) (any, bool)
	Set(ctx context.Context, key string, value any, ttl time.Duration)
	Delete(ctx context.Context, key string) (bool, error)
	Close() error
}

type Locker interface {
	Lock(ctx context.Context, key string, timeout time.Duration) error
	Unlock(ctx context.Context, key string) error
}

type DBPool interface {
	Get(ctx context.Context, dsn string) (*sql.DB, error)
	Close() error
}

type TxRegistry interface {
	Begin(ctx context.Context, db *sql.DB, name string) (*sql.Tx, error)
	Get(name string) (*sql.Tx, error)
	Commit(name string) error
	Rollback(name string) error
	RollbackAll() // called on execution cleanup
}

type ActionServices struct {
	WaitWebhookRegister  func(executionID, stepID, path string, ctx context.Context) (<-chan WaitRequest, func())
	WaitRabbitMQRegister func(url, queue, matchField, matchValue string, ctx context.Context) (<-chan map[string]any, func())

	EmitWaiting func(executionID, stepID string, waitType string, details map[string]any)

	ScheduleExecution func(delay time.Duration, params map[string]any) (string, error)

	Locker     Locker
	DBPool     DBPool
	TxRegistry TxRegistry
	KVStore    KVStore
}
