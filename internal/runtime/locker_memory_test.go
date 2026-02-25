package runtime

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type MemoryLockerTestSuite struct {
	suite.Suite
}

func TestMemoryLocker(t *testing.T) {
	suite.Run(t, new(MemoryLockerTestSuite))
}

func (s *MemoryLockerTestSuite) TestLockUnlock() {
	l := NewMemoryLocker()
	ctx := context.Background()

	err := l.Lock(ctx, "key1", 1*time.Second)
	s.Require().NoError(err)

	err = l.Unlock(ctx, "key1")
	s.Require().NoError(err)
}

func (s *MemoryLockerTestSuite) TestLockTimeout() {
	l := NewMemoryLocker()
	ctx := context.Background()

	err := l.Lock(ctx, "key1", 1*time.Second)
	s.Require().NoError(err)

	// Second lock should timeout
	err = l.Lock(ctx, "key1", 100*time.Millisecond)
	s.Error(err)
	s.Contains(err.Error(), "timeout")
}

func (s *MemoryLockerTestSuite) TestUnlockNotLocked() {
	l := NewMemoryLocker()
	ctx := context.Background()

	err := l.Unlock(ctx, "key1")
	s.Error(err)
	s.Contains(err.Error(), "not locked")
}

func (s *MemoryLockerTestSuite) TestConcurrentAccess() {
	l := NewMemoryLocker()
	ctx := context.Background()

	counter := 0
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := l.Lock(ctx, "counter", 5*time.Second)
			if err != nil {
				return
			}
			counter++
			l.Unlock(ctx, "counter")
		}()
	}

	wg.Wait()
	s.Equal(10, counter)
}
