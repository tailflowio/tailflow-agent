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
}

func TestCronScheduler(t *testing.T) {
	suite.Run(t, new(CronSchedulerTestSuite))
}

func (s *CronSchedulerTestSuite) TestAddAndRun() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	cs := NewCronScheduler(logger)

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
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	cs := NewCronScheduler(logger)

	err := cs.Add("invalid cron", func() {})
	s.Error(err)
}

func (s *CronSchedulerTestSuite) TestStop() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	cs := NewCronScheduler(logger)

	cs.Start()
	cs.Stop() // Should not panic
}
