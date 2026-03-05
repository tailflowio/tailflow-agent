package runtime

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"
)

type RedisKVStoreTestSuite struct {
	suite.Suite
}

func TestRedisKVStore(t *testing.T) {
	suite.Run(t, new(RedisKVStoreTestSuite))
}

func (s *RedisKVStoreTestSuite) SetupTest() {
	// required by convention
}

func (s *RedisKVStoreTestSuite) TestNewRedisKVStore_InvalidURL() {
	_, err := NewRedisKVStore(context.Background(), "not-a-valid-url")
	s.Error(err)
	s.Contains(err.Error(), "invalid URL")
}

func (s *RedisKVStoreTestSuite) TestNewRedisKVStore_ConnectionFailed() {
	_, err := NewRedisKVStore(context.Background(), "redis://localhost:59999")
	s.Error(err)
	s.Contains(err.Error(), "ping failed")
}

func (s *RedisKVStoreTestSuite) TestGetSetDelete() {
	url := os.Getenv("REDIS_TEST_URL")
	if url == "" {
		s.T().Skip("REDIS_TEST_URL not set, skipping Redis tests")
	}

	store, err := NewRedisKVStore(context.Background(), url)
	s.Require().NoError(err)
	defer store.Close()

	ctx := context.Background()

	// Get non-existent key
	val, found := store.Get(ctx, "redis-test-missing")
	s.False(found)
	s.Nil(val)

	// Set and get
	store.Set(ctx, "redis-test-key", "hello", 0)
	val, found = store.Get(ctx, "redis-test-key")
	s.True(found)
	s.Equal("hello", val)

	// Delete
	deleted, err := store.Delete(ctx, "redis-test-key")
	s.NoError(err)
	s.True(deleted)

	// Delete non-existent
	deleted, err = store.Delete(ctx, "redis-test-key")
	s.NoError(err)
	s.False(deleted)
}

func (s *RedisKVStoreTestSuite) TestTTL() {
	url := os.Getenv("REDIS_TEST_URL")
	if url == "" {
		s.T().Skip("REDIS_TEST_URL not set, skipping Redis tests")
	}

	store, err := NewRedisKVStore(context.Background(), url)
	s.Require().NoError(err)
	defer store.Close()

	ctx := context.Background()

	store.Set(ctx, "redis-test-ttl", "expires", 100*time.Millisecond)

	val, found := store.Get(ctx, "redis-test-ttl")
	s.True(found)
	s.Equal("expires", val)

	s.Eventually(func() bool {
		_, found := store.Get(ctx, "redis-test-ttl")
		return !found
	}, 2*time.Second, 10*time.Millisecond)
}

func (s *RedisKVStoreTestSuite) TestJSONRoundtrip() {
	url := os.Getenv("REDIS_TEST_URL")
	if url == "" {
		s.T().Skip("REDIS_TEST_URL not set, skipping Redis tests")
	}

	store, err := NewRedisKVStore(context.Background(), url)
	s.Require().NoError(err)
	defer store.Close()

	ctx := context.Background()

	// Store a complex value
	input := map[string]any{
		"name":   "test",
		"count":  float64(42),
		"nested": map[string]any{"ok": true},
	}
	store.Set(ctx, "redis-test-json", input, 0)

	val, found := store.Get(ctx, "redis-test-json")
	s.True(found)
	s.Equal(input, val)

	// Cleanup
	store.Delete(ctx, "redis-test-json")
}

// mockRedisClient implements redisClient for testing.
type mockRedisClient struct {
	getFunc   func(ctx context.Context, key string) *redis.StringCmd
	setFunc   func(ctx context.Context, key string, value any, exp time.Duration) *redis.StatusCmd
	delFunc   func(ctx context.Context, keys ...string) *redis.IntCmd
	pingFunc  func(ctx context.Context) *redis.StatusCmd
	closeFunc func() error
}

func (m *mockRedisClient) Get(ctx context.Context, key string) *redis.StringCmd {
	if m.getFunc != nil {
		return m.getFunc(ctx, key)
	}
	return redis.NewStringCmd(ctx)
}

func (m *mockRedisClient) Set(ctx context.Context, key string, value any, exp time.Duration) *redis.StatusCmd {
	if m.setFunc != nil {
		return m.setFunc(ctx, key, value, exp)
	}
	return redis.NewStatusCmd(ctx)
}

func (m *mockRedisClient) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	if m.delFunc != nil {
		return m.delFunc(ctx, keys...)
	}
	return redis.NewIntCmd(ctx)
}

func (m *mockRedisClient) Ping(ctx context.Context) *redis.StatusCmd {
	if m.pingFunc != nil {
		return m.pingFunc(ctx)
	}
	return redis.NewStatusCmd(ctx)
}

func (m *mockRedisClient) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func newMockStore(mc *mockRedisClient) *RedisKVStore {
	return &RedisKVStore{client: mc}
}

func (s *RedisKVStoreTestSuite) TestGet_Found() {
	mc := &mockRedisClient{
		getFunc: func(ctx context.Context, key string) *redis.StringCmd {
			cmd := redis.NewStringCmd(ctx)
			cmd.SetVal(`{"name":"Alice"}`)
			return cmd
		},
	}
	store := newMockStore(mc)

	val, ok := store.Get(context.Background(), "user")
	s.True(ok)
	m := val.(map[string]any)
	s.Equal("Alice", m["name"])
}

func (s *RedisKVStoreTestSuite) TestGet_NotFound() {
	mc := &mockRedisClient{
		getFunc: func(ctx context.Context, key string) *redis.StringCmd {
			cmd := redis.NewStringCmd(ctx)
			cmd.SetErr(redis.Nil)
			return cmd
		},
	}
	store := newMockStore(mc)

	val, ok := store.Get(context.Background(), "missing")
	s.False(ok)
	s.Nil(val)
}

func (s *RedisKVStoreTestSuite) TestGet_NetworkError() {
	mc := &mockRedisClient{
		getFunc: func(ctx context.Context, key string) *redis.StringCmd {
			cmd := redis.NewStringCmd(ctx)
			cmd.SetErr(fmt.Errorf("connection refused"))
			return cmd
		},
	}
	store := newMockStore(mc)

	val, ok := store.Get(context.Background(), "key")
	s.False(ok)
	s.Nil(val)
}

func (s *RedisKVStoreTestSuite) TestGet_UnmarshalError() {
	mc := &mockRedisClient{
		getFunc: func(ctx context.Context, key string) *redis.StringCmd {
			cmd := redis.NewStringCmd(ctx)
			cmd.SetVal(`{"valid":"json"}`)
			return cmd
		},
	}
	store := newMockStore(mc)

	orig := jsonUnmarshalKV
	jsonUnmarshalKV = func(data []byte, v any) error { return fmt.Errorf("unmarshal boom") }
	defer func() { jsonUnmarshalKV = orig }()

	val, ok := store.Get(context.Background(), "key")
	s.False(ok)
	s.Nil(val)
}

func (s *RedisKVStoreTestSuite) TestSet_Success() {
	var gotKey string
	var gotValue any
	mc := &mockRedisClient{
		setFunc: func(ctx context.Context, key string, value any, exp time.Duration) *redis.StatusCmd {
			gotKey = key
			gotValue = value
			cmd := redis.NewStatusCmd(ctx)
			cmd.SetVal("OK")
			return cmd
		},
	}
	store := newMockStore(mc)

	store.Set(context.Background(), "greeting", "hello", 5*time.Minute)
	s.Equal(kvRedisPrefix+"greeting", gotKey)
	s.NotNil(gotValue)
}

func (s *RedisKVStoreTestSuite) TestSet_MarshalError() {
	orig := jsonMarshalKV
	jsonMarshalKV = func(v any) ([]byte, error) { return nil, fmt.Errorf("marshal boom") }
	defer func() { jsonMarshalKV = orig }()

	setCalled := false
	mc := &mockRedisClient{
		setFunc: func(ctx context.Context, key string, value any, exp time.Duration) *redis.StatusCmd {
			setCalled = true
			cmd := redis.NewStatusCmd(ctx)
			return cmd
		},
	}
	store := newMockStore(mc)

	store.Set(context.Background(), "key", "value", 0)
	s.False(setCalled, "redis Set should not be called when marshal fails")
}

func (s *RedisKVStoreTestSuite) TestDelete_Success() {
	mc := &mockRedisClient{
		delFunc: func(ctx context.Context, keys ...string) *redis.IntCmd {
			cmd := redis.NewIntCmd(ctx)
			cmd.SetVal(1)
			return cmd
		},
	}
	store := newMockStore(mc)

	deleted, err := store.Delete(context.Background(), "key")
	s.NoError(err)
	s.True(deleted)
}

func (s *RedisKVStoreTestSuite) TestDelete_NotFound() {
	mc := &mockRedisClient{
		delFunc: func(ctx context.Context, keys ...string) *redis.IntCmd {
			cmd := redis.NewIntCmd(ctx)
			cmd.SetVal(0)
			return cmd
		},
	}
	store := newMockStore(mc)

	deleted, err := store.Delete(context.Background(), "missing")
	s.NoError(err)
	s.False(deleted)
}

func (s *RedisKVStoreTestSuite) TestDelete_Error() {
	mc := &mockRedisClient{
		delFunc: func(ctx context.Context, keys ...string) *redis.IntCmd {
			cmd := redis.NewIntCmd(ctx)
			cmd.SetErr(fmt.Errorf("network error"))
			return cmd
		},
	}
	store := newMockStore(mc)

	_, err := store.Delete(context.Background(), "key")
	s.Error(err)
	s.Contains(err.Error(), "delete failed")
}

func (s *RedisKVStoreTestSuite) TestClose_Success() {
	mc := &mockRedisClient{
		closeFunc: func() error { return nil },
	}
	store := newMockStore(mc)

	err := store.Close()
	s.NoError(err)
}

func (s *RedisKVStoreTestSuite) TestClose_Error() {
	mc := &mockRedisClient{
		closeFunc: func() error { return fmt.Errorf("close failed") },
	}
	store := newMockStore(mc)

	err := store.Close()
	s.Error(err)
	s.Contains(err.Error(), "close failed")
}

func (s *RedisKVStoreTestSuite) TestNewRedisKVStore_Success() {
	orig := newRedisClientFn
	defer func() { newRedisClientFn = orig }()

	mc := &mockRedisClient{
		pingFunc: func(ctx context.Context) *redis.StatusCmd {
			cmd := redis.NewStatusCmd(ctx)
			cmd.SetVal("PONG")
			return cmd
		},
	}

	newRedisClientFn = func(opts *redis.Options) redisClient {
		return mc
	}

	store, err := NewRedisKVStore(context.Background(), "redis://localhost:6379")
	s.NoError(err)
	s.NotNil(store)
}
