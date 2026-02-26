package server

import (
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type CronSchedulerTestSuite struct {
	suite.Suite
	logger *slog.Logger
}

func TestCronScheduler(t *testing.T) {
	suite.Run(t, new(CronSchedulerTestSuite))
}

func (s *CronSchedulerTestSuite) SetupTest() {
	s.logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func (s *CronSchedulerTestSuite) TestAddAndRun() {
	cs := NewCronScheduler(s.logger)

	var count atomic.Int32
	err := cs.Add("* * * * * *", func() { // every second (cron/v3 supports seconds with 6 fields via cron.SecondOptional)
		count.Add(1)
	})
	// Standard cron only supports 5 fields; 6 fields should fail.
	// Use a valid 5-field expression instead.
	if err != nil {
		// Retry with standard 5-field expression using @every
		count.Store(0)
		err = cs.Add("@every 1s", func() {
			count.Add(1)
		})
	}
	s.Require().NoError(err)

	cs.Start()
	defer cs.Stop()

	s.Eventually(func() bool {
		return count.Load() >= int32(2)
	}, 5*time.Second, 50*time.Millisecond)
}

func (s *CronSchedulerTestSuite) TestInvalidSpec() {
	cs := NewCronScheduler(s.logger)

	err := cs.Add("invalid cron", func() {})
	s.Error(err)
}

func (s *CronSchedulerTestSuite) TestStop_AfterStart() {
	cs := NewCronScheduler(s.logger)

	cs.Start()
	s.NotPanics(func() {
		cs.Stop()
	}, "Stop after Start should not panic")
}

func (s *CronSchedulerTestSuite) TestNextRun_Valid() {
	// Use a standard 5-field cron expression: every minute
	next := NextRun("* * * * *")
	s.NotNil(next, "NextRun should return a non-nil time for a valid spec")
	s.True(next.After(time.Now().Add(-1*time.Second)), "next run should be in the future")
}

func (s *CronSchedulerTestSuite) TestNextRun_Invalid() {
	next := NextRun("invalid cron expression")
	s.Nil(next, "NextRun should return nil for an invalid spec")
}
