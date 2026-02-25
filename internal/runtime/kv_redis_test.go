package runtime

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type RedisKVStoreTestSuite struct {
	suite.Suite
}

func TestRedisKVStore(t *testing.T) {
	suite.Run(t, new(RedisKVStoreTestSuite))
}

func (s *RedisKVStoreTestSuite) TestGetSetDelete() {
	url := os.Getenv("REDIS_TEST_URL")
	if url == "" {
		s.T().Skip("REDIS_TEST_URL not set, skipping Redis tests")
	}

	store, err := NewRedisKVStore(url)
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

	store, err := NewRedisKVStore(url)
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

	store, err := NewRedisKVStore(url)
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
