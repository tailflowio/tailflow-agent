package metrics

import (
	"context"
	"runtime"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/suite"
)

type CollectorTestSuite struct {
	suite.Suite
}

func TestCollector(t *testing.T) {
	suite.Run(t, new(CollectorTestSuite))
}

func (s *CollectorTestSuite) SetupTest() {
	// required by convention
}

func (s *CollectorTestSuite) TestNew_InitializesCollector() {
	c := New()

	s.Require().NotNil(c)
	s.False(c.startAt.IsZero())
}

func (s *CollectorTestSuite) TestSnapshot_ReturnsDefaultValues() {
	c := New()
	snap := c.Snapshot()

	s.Equal(float64(0), snap.CPUPercent)
	s.Equal(int64(0), snap.RSSKB)
	s.Equal(0, snap.Goroutines)
	s.Equal(float64(0), snap.HeapMB)
	s.Equal(int64(0), snap.NetRxBytes)
	s.Equal(int64(0), snap.NetTxBytes)
	s.False(snap.Available)
}

func (s *CollectorTestSuite) TestStart_ContextCancel() {
	synctest.Test(s.T(), func(t *testing.T) {
		c := New()
		ctx, cancel := context.WithCancel(context.Background())

		c.Start(ctx)

		cancel()
		synctest.Wait()
	})
}

func (s *CollectorTestSuite) TestReadCPUTicks_ReturnsFalseOnMissingFile() {
	original := procStatPath
	defer func() { procStatPath = original }()

	procStatPath = "/nonexistent/proc/stat"

	ticks, ok := readCPUTicks()

	s.Equal(int64(0), ticks)
	s.False(ok)
}

func (s *CollectorTestSuite) TestReadRSSKB_ReturnsFalseOnMissingFile() {
	original := procStatusPath
	defer func() { procStatusPath = original }()

	procStatusPath = "/nonexistent/proc/status"

	rss, ok := readRSSKB()

	s.Equal(int64(0), rss)
	s.False(ok)
}

func (s *CollectorTestSuite) TestReadNetDev_ReturnsFalseOnMissingFile() {
	original := procNetDevPath
	defer func() { procNetDevPath = original }()

	procNetDevPath = "/nonexistent/proc/net/dev"

	rx, tx, ok := readNetDev()

	s.Equal(int64(0), rx)
	s.Equal(int64(0), tx)
	s.False(ok)
}

func (s *CollectorTestSuite) TestSnapshot_ReflectsGoroutineCount() {
	c := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c.Start(ctx)

	snap := c.Snapshot()

	s.GreaterOrEqual(snap.Goroutines, 1)
	s.LessOrEqual(snap.Goroutines, runtime.NumGoroutine()+5)
}

func (s *CollectorTestSuite) TestCollect_SetsAvailableTrue() {
	c := New()
	c.collect()

	snap := c.Snapshot()

	if runtime.GOOS == "linux" {
		s.True(snap.Available)
	} else {
		s.False(snap.Available)
	}

	s.GreaterOrEqual(snap.Goroutines, 1)
	s.Greater(snap.HeapMB, float64(0))
}
