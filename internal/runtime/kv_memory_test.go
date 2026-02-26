package runtime

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/suite"
)

type MemoryKVStoreTestSuite struct {
	suite.Suite
}

func TestMemoryKVStore(t *testing.T) {
	suite.Run(t, new(MemoryKVStoreTestSuite))
}

func (s *MemoryKVStoreTestSuite) SetupTest() { // required by convention
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

func (s *MemoryKVStoreTestSuite) TestOverwrite_ReplacesValue() {
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
	synctest.Test(s.T(), func(t *testing.T) {
		st := NewMemoryKVStore()
		ctx := context.Background()

		st.Set(ctx, "key1", "value1", 10*time.Millisecond)

		val, found := st.Get(ctx, "key1")
		s.True(found)
		s.Equal("value1", val)

		time.Sleep(20 * time.Millisecond)

		_, found = st.Get(ctx, "key1")
		s.False(found)
	})
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
	synctest.Test(s.T(), func(t *testing.T) {
		st := NewMemoryKVStore()
		ctx := context.Background()

		st.Set(ctx, "key1", "value1", 10*time.Millisecond)
		st.Set(ctx, "key1", "value2", 0)

		time.Sleep(20 * time.Millisecond)

		val, found := st.Get(ctx, "key1")
		s.True(found)
		s.Equal("value2", val)
	})
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
	synctest.Test(s.T(), func(t *testing.T) {
		st := NewMemoryKVStore()
		ctx := context.Background()

		st.Set(ctx, "key1", "value1", 10*time.Millisecond)

		time.Sleep(20 * time.Millisecond)

		deleted, err := st.Delete(ctx, "key1")
		s.NoError(err)
		s.False(deleted)
	})
}
