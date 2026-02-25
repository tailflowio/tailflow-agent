package lock

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
)

type LockTestSuite struct {
	suite.Suite
}

func TestLock(t *testing.T) {
	suite.Run(t, new(LockTestSuite))
}

func (s *LockTestSuite) TestMemoryLocker_Acquire() {
	l := NewMemoryLocker()

	release, acquired, err := l.Acquire(context.Background(), "key1")
	s.Require().NoError(err)
	s.True(acquired)
	s.NotNil(release)

	// Second acquire should fail
	_, acquired2, err := l.Acquire(context.Background(), "key1")
	s.Require().NoError(err)
	s.False(acquired2)

	// Release and try again
	release()
	release3, acquired3, err := l.Acquire(context.Background(), "key1")
	s.Require().NoError(err)
	s.True(acquired3)
	release3()
}

func (s *LockTestSuite) TestMemoryLocker_DifferentKeys() {
	l := NewMemoryLocker()

	r1, a1, _ := l.Acquire(context.Background(), "key1")
	s.True(a1)

	r2, a2, _ := l.Acquire(context.Background(), "key2")
	s.True(a2)

	r1()
	r2()
}

func (s *LockTestSuite) TestMemoryDeduplicator() {
	d := NewMemoryDeduplicator()

	dup, err := d.IsDuplicate(context.Background(), "event-1")
	s.Require().NoError(err)
	s.False(dup)

	dup2, err := d.IsDuplicate(context.Background(), "event-1")
	s.Require().NoError(err)
	s.True(dup2)

	dup3, err := d.IsDuplicate(context.Background(), "event-2")
	s.Require().NoError(err)
	s.False(dup3)
}
