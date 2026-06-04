package metrics

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

func (s *CollectorCoverageSuite) TestCollect_WithFakeProcFiles() {
	origStat := procStatPath
	origStatus := procStatusPath
	origNet := procNetDevPath
	defer func() {
		procStatPath = origStat
		procStatusPath = origStatus
		procNetDevPath = origNet
	}()

	// Valid stat file
	statContent := "12345 (test) S 1 12345 12345 0 -1 4194304 100 0 0 0 150 250 0 0 20 0 1 0 12345678 12345678 100 18446744073709551615 0 0 0 0 0 0 0 0 0 0 0 0 17 0 0 0 0 0 0"
	procStatPath = s.writeFile("stat", statContent)

	// Valid status file
	statusContent := "Name:\ttest\nVmRSS:\t4096 kB\n"
	procStatusPath = s.writeFile("status", statusContent)

	// Valid net/dev file
	netContent := `Inter-|   Receive
 face |bytes
    lo: 1000 10 0 0 0 0 0 0 2000 20 0 0 0 0 0 0
`
	procNetDevPath = s.writeFile("netdev", netContent)

	c := New()
	c.collect()

	snap := c.Snapshot()
	s.True(snap.Available)
	s.Equal(int64(4096), snap.RSSKB)
	s.Equal(int64(1000), snap.NetRxBytes)
	s.Equal(int64(2000), snap.NetTxBytes)
	s.GreaterOrEqual(snap.Goroutines, 1)
	s.Greater(snap.HeapMB, float64(0))

	// Second collect should compute CPU percent
	c.prevTime = time.Now().Add(-1 * time.Second)
	c.collect()

	snap2 := c.Snapshot()
	s.True(snap2.Available)
}

func (s *CollectorCoverageSuite) TestCollect_OnlyRSSAvailable() {
	origStat := procStatPath
	origStatus := procStatusPath
	origNet := procNetDevPath
	defer func() {
		procStatPath = origStat
		procStatusPath = origStatus
		procNetDevPath = origNet
	}()

	procStatPath = "/nonexistent"
	procNetDevPath = "/nonexistent"

	statusContent := "Name:\ttest\nVmRSS:\t2048 kB\n"
	procStatusPath = s.writeFile("status_only", statusContent)

	c := New()
	c.collect()

	snap := c.Snapshot()
	s.True(snap.Available)
	s.Equal(int64(2048), snap.RSSKB)
	s.Equal(int64(0), snap.NetRxBytes)
	s.Equal(int64(0), snap.NetTxBytes)
}

func (s *CollectorCoverageSuite) TestCollect_OnlyNetAvailable() {
	origStat := procStatPath
	origStatus := procStatusPath
	origNet := procNetDevPath
	defer func() {
		procStatPath = origStat
		procStatusPath = origStatus
		procNetDevPath = origNet
	}()

	procStatPath = "/nonexistent"
	procStatusPath = "/nonexistent"

	netContent := `Inter-|   Receive
 face |bytes
    lo: 500 10 0 0 0 0 0 0 700 20 0 0 0 0 0 0
`
	procNetDevPath = s.writeFile("netdev_only", netContent)

	c := New()
	c.collect()

	snap := c.Snapshot()
	s.True(snap.Available)
	s.Equal(int64(0), snap.RSSKB)
	s.Equal(int64(500), snap.NetRxBytes)
	s.Equal(int64(700), snap.NetTxBytes)
}

func (s *CollectorCoverageSuite) TestCollect_OnlyCPUAvailable() {
	origStat := procStatPath
	origStatus := procStatusPath
	origNet := procNetDevPath
	defer func() {
		procStatPath = origStat
		procStatusPath = origStatus
		procNetDevPath = origNet
	}()

	procStatusPath = "/nonexistent"
	procNetDevPath = "/nonexistent"

	statContent := "12345 (test) S 1 12345 12345 0 -1 4194304 100 0 0 0 100 200 0 0 20 0 1 0 12345678 12345678 100 18446744073709551615 0 0 0 0 0 0 0 0 0 0 0 0 17 0 0 0 0 0 0"
	procStatPath = s.writeFile("stat_only", statContent)

	c := New()
	c.collect()

	snap := c.Snapshot()
	s.True(snap.Available)
}

func (s *CollectorCoverageSuite) TestString_FormatsCorrectly() {
	m := ProcessMetrics{
		CPUPercent: 42.5,
		RSSKB:      8192,
		Goroutines: 10,
		HeapMB:     3.5,
		NetRxBytes: 1000,
		NetTxBytes: 2000,
		UptimeS:    120,
		Available:  true,
	}

	str := m.String()

	s.Contains(str, "cpu=42.5%")
	s.Contains(str, "rss=8192kB")
	s.Contains(str, "goroutines=10")
	s.Contains(str, "heap=3.5MB")
	s.Contains(str, "rx=1000")
	s.Contains(str, "tx=2000")
	s.Contains(str, "uptime=120s")
	s.Contains(str, "available=true")
}

func (s *CollectorCoverageSuite) TestString_ZeroValues() {
	m := ProcessMetrics{}

	str := m.String()

	s.Contains(str, "cpu=0.0%")
	s.Contains(str, "available=false")
}

func (s *CollectorCoverageSuite) TestStart_TickerTrigger() {
	synctest.Test(s.T(), func(t *testing.T) {
		c := New()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		c.Start(ctx)

		// Advance time by 2s to trigger at least one ticker-based collect
		time.Sleep(2 * time.Second)

		snap := c.Snapshot()
		// After ticking, goroutines/heap should be populated
		if snap.Goroutines < 1 {
			t.Errorf("expected goroutines >= 1, got %d", snap.Goroutines)
		}

		cancel()
		synctest.Wait()
	})
}
