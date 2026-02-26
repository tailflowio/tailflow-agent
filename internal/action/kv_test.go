package action

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/runtime"
)

// errorKVStore wraps MemoryKVStore but always returns an error on Delete.
type errorKVStore struct {
	runtime.MemoryKVStore
}

func (e *errorKVStore) Delete(_ context.Context, _ string) (bool, error) {
	return false, fmt.Errorf("delete failed")
}

func newTestContextWithKVStore(config map[string]any) (*ActionContext, *runtime.MemoryKVStore) {
	kvStore := runtime.NewMemoryKVStore()
	services := &runtime.ActionServices{
		KVStore: kvStore,
	}
	return &ActionContext{
		Context:  context.Background(),
		Config:   config,
		ExecCtx:  runtime.NewExecutionContext("test-exec", "test-wf", nil, nil),
		StepID:   "test-step",
		Logger:   slog.Default(),
		Services: services,
	}, kvStore
}

type KVActionTestSuite struct {
	suite.Suite
}

func TestKVAction(t *testing.T) {
	suite.Run(t, new(KVActionTestSuite))
}

func (s *KVActionTestSuite) SetupTest() {}

func (s *KVActionTestSuite) TestGetMissingKey() {
	a := NewKVGetAction()
	ctx, _ := newTestContextWithKVStore(map[string]any{})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "key")
}

func (s *KVActionTestSuite) TestGetNoServices() {
	a := NewKVGetAction()
	ctx := newTestContext(map[string]any{"key": "test"})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "serve")
}

func (s *KVActionTestSuite) TestGetNotFound() {
	a := NewKVGetAction()
	ctx, _ := newTestContextWithKVStore(map[string]any{"key": "missing"})

	err := a.Validate(ctx)
	s.Require().NoError(err)

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	result := out.(map[string]any)
	s.Equal("missing", result["key"])
	s.False(result["found"].(bool))
	s.Nil(result["value"])
}

func (s *KVActionTestSuite) TestGetFound() {
	a := NewKVGetAction()
	ctx, kvStore := newTestContextWithKVStore(map[string]any{"key": "mykey"})
	kvStore.Set(context.Background(), "mykey", "myvalue", 0)

	err := a.Validate(ctx)
	s.Require().NoError(err)

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	result := out.(map[string]any)
	s.Equal("mykey", result["key"])
	s.True(result["found"].(bool))
	s.Equal("myvalue", result["value"])
}

func (s *KVActionTestSuite) TestSetMissingKey() {
	a := NewKVSetAction()
	ctx, _ := newTestContextWithKVStore(map[string]any{"value": "x"})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "key")
}

func (s *KVActionTestSuite) TestSetMissingValue() {
	a := NewKVSetAction()
	ctx, _ := newTestContextWithKVStore(map[string]any{"key": "x"})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "value")
}

func (s *KVActionTestSuite) TestSetNoServices() {
	a := NewKVSetAction()
	ctx := newTestContext(map[string]any{"key": "test", "value": "val"})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "serve")
}

func (s *KVActionTestSuite) TestSetExecute() {
	a := NewKVSetAction()
	ctx, kvStore := newTestContextWithKVStore(map[string]any{"key": "mykey", "value": "myvalue"})

	err := a.Validate(ctx)
	s.Require().NoError(err)

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	result := out.(map[string]any)
	s.Equal("mykey", result["key"])
	s.True(result["stored"].(bool))
	s.Nil(result["ttl"]) // no TTL set

	// Verify the value was actually stored
	val, found := kvStore.Get(context.Background(), "mykey")
	s.True(found)
	s.Equal("myvalue", val)
}

func (s *KVActionTestSuite) TestSetWithTTL() {
	a := NewKVSetAction()
	ctx, _ := newTestContextWithKVStore(map[string]any{
		"key":   "mykey",
		"value": "myvalue",
		"ttl":   "5m",
	})

	err := a.Validate(ctx)
	s.Require().NoError(err)

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	result := out.(map[string]any)
	s.Equal("mykey", result["key"])
	s.True(result["stored"].(bool))
	s.Equal("5m0s", result["ttl"])
}

func (s *KVActionTestSuite) TestDeleteMissingKey() {
	a := NewKVDeleteAction()
	ctx, _ := newTestContextWithKVStore(map[string]any{})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "key")
}

func (s *KVActionTestSuite) TestDeleteNoServices() {
	a := NewKVDeleteAction()
	ctx := newTestContext(map[string]any{"key": "test"})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "serve")
}

func (s *KVActionTestSuite) TestDeleteExisting() {
	a := NewKVDeleteAction()
	ctx, kvStore := newTestContextWithKVStore(map[string]any{"key": "mykey"})
	kvStore.Set(context.Background(), "mykey", "myvalue", 0)

	err := a.Validate(ctx)
	s.Require().NoError(err)

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	result := out.(map[string]any)
	s.Equal("mykey", result["key"])
	s.True(result["deleted"].(bool))

	// Verify deletion
	_, found := kvStore.Get(context.Background(), "mykey")
	s.False(found)
}

func (s *KVActionTestSuite) TestDeleteNonExistent() {
	a := NewKVDeleteAction()
	ctx, _ := newTestContextWithKVStore(map[string]any{"key": "missing"})

	err := a.Validate(ctx)
	s.Require().NoError(err)

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	result := out.(map[string]any)
	s.Equal("missing", result["key"])
	s.False(result["deleted"].(bool))
}

func (s *KVActionTestSuite) TestDeleteError() {
	a := NewKVDeleteAction()
	errStore := &errorKVStore{}
	services := &runtime.ActionServices{
		KVStore: errStore,
	}
	ctx := &ActionContext{
		Context:  context.Background(),
		Config:   map[string]any{"key": "mykey"},
		ExecCtx:  runtime.NewExecutionContext("test-exec", "test-wf", nil, nil),
		StepID:   "test-step",
		Logger:   slog.Default(),
		Services: services,
	}

	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "delete failed")
}
