package export

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/runtime"
)

type MemoryClaimerTestSuite struct {
	suite.Suite

	context context.Context
	claimer *MemoryClaimer
}

func TestMemoryClaimer(t *testing.T) {
	suite.Run(t, new(MemoryClaimerTestSuite))
}

func (s *MemoryClaimerTestSuite) SetupTest() {
	s.context = context.Background()
	s.claimer = NewMemoryClaimer()
}

func (s *MemoryClaimerTestSuite) TestMemoryClaimer_SecondClaimDeduplicates() {
	first, err := s.claimer.ClaimExecution(s.context, "exec-A", "wf", "key-1")
	s.Require().NoError(err)
	s.True(first.Claimed)

	second, err := s.claimer.ClaimExecution(s.context, "exec-B", "wf", "key-1")
	s.Require().NoError(err)
	s.False(second.Claimed)
	s.Equal("exec-A", second.ExistingExecutionID)
	s.Equal(runtime.StatusRunning, second.ExistingStatus)
}

func (s *MemoryClaimerTestSuite) TestMemoryClaimer_DistinctKeysClaim() {
	first, err := s.claimer.ClaimExecution(s.context, "exec-A", "wf", "key-1")
	s.Require().NoError(err)
	s.True(first.Claimed)

	second, err := s.claimer.ClaimExecution(s.context, "exec-B", "wf", "key-2")
	s.Require().NoError(err)
	s.True(second.Claimed)
}

func (s *MemoryClaimerTestSuite) TestMemoryClaimer_DistinctWorkflowsClaim() {
	first, err := s.claimer.ClaimExecution(s.context, "exec-A", "wf-1", "key-1")
	s.Require().NoError(err)
	s.True(first.Claimed)

	second, err := s.claimer.ClaimExecution(s.context, "exec-B", "wf-2", "key-1")
	s.Require().NoError(err)
	s.True(second.Claimed)
}

func (s *MemoryClaimerTestSuite) TestMemoryClaimer_ConcurrentSingleWinner() {
	const goroutines = 64

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		winners int
	)

	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()

			result, err := s.claimer.ClaimExecution(s.context, "exec", "wf", "key")
			if err != nil {
				return
			}

			if result.Claimed {
				mu.Lock()
				winners++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	s.Equal(1, winners)
}
