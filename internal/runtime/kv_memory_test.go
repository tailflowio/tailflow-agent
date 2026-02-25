package runtime

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type MemoryKVStoreTestSuite struct {
	suite.Suite
}

func TestMemoryKVStore(t *testing.T) {
	suite.Run(t, new(MemoryKVStoreTestSuite))
}

func (s *MemoryKVStoreTestSuite) TestGetNotFound() {
	st := NewMemoryKVStore()
	ctx := context.Background()

	val, found := st.Get(ctx, "missing")
	s.False(found)
	s.Nil(val)
}

func (s *MemoryKVStoreTestSuite) TestSetAndGet() {
	st := NewMemoryKVStore()
	ctx := context.Background()

	st.Set(ctx, "key1", "value1", 0)

	val, found := st.Get(ctx, "key1")
	s.True(found)
	s.Equal("value1", val)
}

func (s *MemoryKVStoreTestSuite) TestOverwrite() {
	st := NewMemoryKVStore()
	ctx := context.Background()

	st.Set(ctx, "key1", "old", 0)
	st.Set(ctx, "key1", "new", 0)

	val, found := st.Get(ctx, "key1")
	s.True(found)
	s.Equal("new", val)
}

func (s *MemoryKVStoreTestSuite) TestConcurrentAccess() {
	st := NewMemoryKVStore()
	ctx := context.Background()

	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			st.Set(ctx, "key", i, 0)
			st.Get(ctx, "key")
		}(i)
	}

	wg.Wait()

	_, found := st.Get(ctx, "key")
	s.True(found)
}

func (s *MemoryKVStoreTestSuite) TestTTL_Expired() {
	st := NewMemoryKVStore()
	ctx := context.Background()

	st.Set(ctx, "key1", "value1", 10*time.Millisecond)

	// Should be found immediately
	val, found := st.Get(ctx, "key1")
	s.True(found)
	s.Equal("value1", val)

	// Wait for expiration
	s.Eventually(func() bool {
		_, found := st.Get(ctx, "key1")
		return !found
	}, 2*time.Second, 10*time.Millisecond)
}

func (s *MemoryKVStoreTestSuite) TestTTL_NotExpired() {
	st := NewMemoryKVStore()
	ctx := context.Background()

	st.Set(ctx, "key1", "value1", 1*time.Hour)

	val, found := st.Get(ctx, "key1")
	s.True(found)
	s.Equal("value1", val)
}

func (s *MemoryKVStoreTestSuite) TestTTL_ZeroPermanent() {
	st := NewMemoryKVStore()
	ctx := context.Background()

	st.Set(ctx, "key1", "value1", 0)

	val, found := st.Get(ctx, "key1")
	s.True(found)
	s.Equal("value1", val)
}

func (s *MemoryKVStoreTestSuite) TestOverwriteResetsTTL() {
	st := NewMemoryKVStore()
	ctx := context.Background()

	// Set with short TTL
	st.Set(ctx, "key1", "value1", 10*time.Millisecond)

	// Overwrite with no TTL (permanent)
	st.Set(ctx, "key1", "value2", 0)

	// Should still exist because TTL was reset (wait long enough for original TTL to expire)
	time.Sleep(20 * time.Millisecond)

	val, found := st.Get(ctx, "key1")
	s.True(found)
	s.Equal("value2", val)
}

func (s *MemoryKVStoreTestSuite) TestDelete_Existing() {
	st := NewMemoryKVStore()
	ctx := context.Background()

	st.Set(ctx, "key1", "value1", 0)

	deleted, err := st.Delete(ctx, "key1")
	s.NoError(err)
	s.True(deleted)

	_, found := st.Get(ctx, "key1")
	s.False(found)
}

func (s *MemoryKVStoreTestSuite) TestDelete_NonExistent() {
	st := NewMemoryKVStore()
	ctx := context.Background()

	deleted, err := st.Delete(ctx, "missing")
	s.NoError(err)
	s.False(deleted)
}

func (s *MemoryKVStoreTestSuite) TestDelete_Expired() {
	st := NewMemoryKVStore()
	ctx := context.Background()

	st.Set(ctx, "key1", "value1", 10*time.Millisecond)

	s.Eventually(func() bool {
		deleted, err := st.Delete(ctx, "key1")
		return err == nil && !deleted
	}, 2*time.Second, 10*time.Millisecond)
}
