package metrics

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/suite"
)

type CollectorCoverageSuite struct {
	suite.Suite
	tmpDir string
}

func TestCollectorCoverage(t *testing.T) {
	suite.Run(t, new(CollectorCoverageSuite))
}

func (s *CollectorCoverageSuite) SetupTest() {
	dir, err := os.MkdirTemp("", "collector-test-*")
	s.Require().NoError(err)
	s.tmpDir = dir
}

func (s *CollectorCoverageSuite) TearDownTest() {
	os.RemoveAll(s.tmpDir)
}

// helper: write a temp file and return its path
func (s *CollectorCoverageSuite) writeFile(name, content string) string {
	p := filepath.Join(s.tmpDir, name)
	err := os.WriteFile(p, []byte(content), 0644)
	s.Require().NoError(err)
	return p
}

func (s *CollectorCoverageSuite) TestReadCPUTicks_ValidStatFile() {
	// /proc/self/stat format: pid (comm) state fields...
	// Fields after ')': state, ppid, pgrp, session, tty_nr, tpgid,
	// flags, minflt, cminflt, majflt, cmajflt, utime(11), stime(12) ...
	// We need at least 13 fields after the closing paren.
	content := "12345 (myprocess) S 1 12345 12345 0 -1 4194304 100 0 0 0 150 250 0 0 20 0 1 0 12345678 12345678 100 18446744073709551615 0 0 0 0 0 0 0 0 0 0 0 0 17 0 0 0 0 0 0"
	path := s.writeFile("stat", content)

	orig := procStatPath
	defer func() { procStatPath = orig }()
	procStatPath = path

	ticks, ok := readCPUTicks()
	s.True(ok)
	s.Equal(int64(400), ticks) // utime=150 + stime=250
}

func (s *CollectorCoverageSuite) TestReadCPUTicks_NoClosingParen() {
	content := "12345 (myprocess S 1 2 3"
	path := s.writeFile("stat_noparen", content)

	orig := procStatPath
	defer func() { procStatPath = orig }()
	procStatPath = path

	ticks, ok := readCPUTicks()
	s.False(ok)
	s.Equal(int64(0), ticks)
}

func (s *CollectorCoverageSuite) TestReadCPUTicks_ClosingParenAtEnd() {
	// closing paren at end of string, closingParenIdx+2 >= len(content)
	content := "12345 (myprocess)"
	path := s.writeFile("stat_parenend", content)

	orig := procStatPath
	defer func() { procStatPath = orig }()
	procStatPath = path

	ticks, ok := readCPUTicks()
	s.False(ok)
	s.Equal(int64(0), ticks)
}

func (s *CollectorCoverageSuite) TestReadCPUTicks_TooFewFields() {
	// Fewer than 13 fields after closing paren
	content := "12345 (myprocess) S 1 2 3 4 5 6 7 8 9"
	path := s.writeFile("stat_fewfields", content)

	orig := procStatPath
	defer func() { procStatPath = orig }()
	procStatPath = path

	ticks, ok := readCPUTicks()
	s.False(ok)
	s.Equal(int64(0), ticks)
}

func (s *CollectorCoverageSuite) TestReadCPUTicks_InvalidUtime() {
	// 13+ fields after paren but utime (field[11]) is not a number
	fields := make([]string, 14)
	for i := range fields {
		fields[i] = "0"
	}
	fields[0] = "S" // state
	fields[11] = "notanumber"
	fields[12] = "100"
	content := "12345 (myprocess) " + strings.Join(fields, " ")
	path := s.writeFile("stat_badutime", content)

	orig := procStatPath
	defer func() { procStatPath = orig }()
	procStatPath = path

	ticks, ok := readCPUTicks()
	s.False(ok)
	s.Equal(int64(0), ticks)
}

func (s *CollectorCoverageSuite) TestReadCPUTicks_InvalidStime() {
	fields := make([]string, 14)
	for i := range fields {
		fields[i] = "0"
	}
	fields[0] = "S"
	fields[11] = "100"
	fields[12] = "notanumber"
	content := "12345 (myprocess) " + strings.Join(fields, " ")
	path := s.writeFile("stat_badstime", content)

	orig := procStatPath
	defer func() { procStatPath = orig }()
	procStatPath = path

	ticks, ok := readCPUTicks()
	s.False(ok)
	s.Equal(int64(0), ticks)
}

func (s *CollectorCoverageSuite) TestReadCPUTicks_CommFieldWithSpaces() {
	// comm field can contain spaces and parens: (my (complex) process)
	content := "12345 (my (complex) process) S 1 12345 12345 0 -1 4194304 100 0 0 0 200 300 0 0 20 0 1 0 12345678 12345678 100 18446744073709551615 0 0 0 0 0 0 0 0 0 0 0 0 17 0 0 0 0 0 0"
	path := s.writeFile("stat_spaces", content)

	orig := procStatPath
	defer func() { procStatPath = orig }()
	procStatPath = path

	ticks, ok := readCPUTicks()
	s.True(ok)
	s.Equal(int64(500), ticks) // 200 + 300
}

func (s *CollectorCoverageSuite) TestReadRSSKB_ValidFile() {
	content := `Name:	myprocess
VmPeak:	123456 kB
VmSize:	654321 kB
VmRSS:	8192 kB
VmData:	4096 kB
`
	path := s.writeFile("status", content)

	orig := procStatusPath
	defer func() { procStatusPath = orig }()
	procStatusPath = path

	rss, ok := readRSSKB()
	s.True(ok)
	s.Equal(int64(8192), rss)
}

func (s *CollectorCoverageSuite) TestReadRSSKB_NoVmRSSLine() {
	content := `Name:	myprocess
VmPeak:	123456 kB
VmSize:	654321 kB
VmData:	4096 kB
`
	path := s.writeFile("status_norss", content)

	orig := procStatusPath
	defer func() { procStatusPath = orig }()
	procStatusPath = path

	rss, ok := readRSSKB()
	s.False(ok)
	s.Equal(int64(0), rss)
}

func (s *CollectorCoverageSuite) TestReadRSSKB_VmRSSInvalidValue() {
	content := `VmRSS:	notanumber kB
`
	path := s.writeFile("status_badrss", content)

	orig := procStatusPath
	defer func() { procStatusPath = orig }()
	procStatusPath = path

	rss, ok := readRSSKB()
	s.False(ok)
	s.Equal(int64(0), rss)
}

func (s *CollectorCoverageSuite) TestReadRSSKB_VmRSSTooFewFields() {
	// VmRSS: line with only one field (just "VmRSS:")
	content := "VmRSS:\n"
	path := s.writeFile("status_fewfields", content)

	orig := procStatusPath
	defer func() { procStatusPath = orig }()
	procStatusPath = path

	rss, ok := readRSSKB()
	s.False(ok)
	s.Equal(int64(0), rss)
}

func (s *CollectorCoverageSuite) TestReadNetDev_ValidFile() {
	content := `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 1000       10    0    0    0     0          0         0     2000       20    0    0    0     0       0          0
  eth0: 5000       50    0    0    0     0          0         0     8000       80    0    0    0     0       0          0
`
	path := s.writeFile("netdev", content)

	orig := procNetDevPath
	defer func() { procNetDevPath = orig }()
	procNetDevPath = path

	rx, tx, ok := readNetDev()
	s.True(ok)
	s.Equal(int64(6000), rx)  // 1000 + 5000
	s.Equal(int64(10000), tx) // 2000 + 8000
}

func (s *CollectorCoverageSuite) TestReadNetDev_SingleInterface() {
	content := `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
  eth0: 12345       50    0    0    0     0          0         0     67890       80    0    0    0     0       0          0
`
	path := s.writeFile("netdev_single", content)

	orig := procNetDevPath
	defer func() { procNetDevPath = orig }()
	procNetDevPath = path

	rx, tx, ok := readNetDev()
	s.True(ok)
	s.Equal(int64(12345), rx)
	s.Equal(int64(67890), tx)
}

func (s *CollectorCoverageSuite) TestReadNetDev_NoColon() {
	// Lines without a colon separator should be skipped
	content := `Inter-|   Receive
 face |bytes
 eth0  12345       50    0    0    0     0          0         0     67890       80    0    0    0     0       0          0
`
	path := s.writeFile("netdev_nocolon", content)

	orig := procNetDevPath
	defer func() { procNetDevPath = orig }()
	procNetDevPath = path

	rx, tx, ok := readNetDev()
	s.False(ok)
	s.Equal(int64(0), rx)
	s.Equal(int64(0), tx)
}

func (s *CollectorCoverageSuite) TestReadNetDev_TooFewFields() {
	content := `Inter-|   Receive
 face |bytes
  eth0: 12345 50 0 0
`
	path := s.writeFile("netdev_fewfields", content)

	orig := procNetDevPath
	defer func() { procNetDevPath = orig }()
	procNetDevPath = path

	rx, tx, ok := readNetDev()
	s.False(ok)
	s.Equal(int64(0), rx)
	s.Equal(int64(0), tx)
}

func (s *CollectorCoverageSuite) TestReadNetDev_InvalidRxBytes() {
	content := `Inter-|   Receive
 face |bytes
  eth0: notanum 50 0 0 0 0 0 0 67890 80 0 0 0 0 0 0
`
	path := s.writeFile("netdev_badrx", content)

	orig := procNetDevPath
	defer func() { procNetDevPath = orig }()
	procNetDevPath = path

	rx, tx, ok := readNetDev()
	s.False(ok)
	s.Equal(int64(0), rx)
	s.Equal(int64(0), tx)
}

func (s *CollectorCoverageSuite) TestReadNetDev_InvalidTxBytes() {
	content := `Inter-|   Receive
 face |bytes
  eth0: 12345 50 0 0 0 0 0 0 notanum 80 0 0 0 0 0 0
`
	path := s.writeFile("netdev_badtx", content)

	orig := procNetDevPath
	defer func() { procNetDevPath = orig }()
	procNetDevPath = path

	rx, tx, ok := readNetDev()
	s.False(ok)
	s.Equal(int64(0), rx)
	s.Equal(int64(0), tx)
}

func (s *CollectorCoverageSuite) TestReadNetDev_MixedValidAndInvalid() {
	// One valid interface and one with parse errors
	content := `Inter-|   Receive
 face |bytes
    lo: 1000 10 0 0 0 0 0 0 2000 20 0 0 0 0 0 0
  eth0: bad 50 0 0 0 0 0 0 67890 80 0 0 0 0 0 0
`
	path := s.writeFile("netdev_mixed", content)

	orig := procNetDevPath
	defer func() { procNetDevPath = orig }()
	procNetDevPath = path

	rx, tx, ok := readNetDev()
	s.True(ok)
	s.Equal(int64(1000), rx)
	s.Equal(int64(2000), tx)
}

func (s *CollectorCoverageSuite) TestUpdateCPUPercent_FirstCall_SetsPrev() {
	c := New()

	metrics := &ProcessMetrics{}
	c.updateCPUPercent(metrics, 500)

	s.Equal(float64(0), metrics.CPUPercent, "first call should not set CPU percent")
	s.Equal(int64(500), c.prevCPUTicks)
	s.False(c.prevTime.IsZero())
}

func (s *CollectorCoverageSuite) TestUpdateCPUPercent_SecondCall_CalculatesPercent() {
	c := New()

	// First call to set baseline
	metrics1 := &ProcessMetrics{}
	c.updateCPUPercent(metrics1, 1000)

	// Manually set prevTime in the past to have a meaningful elapsed time
	c.prevTime = time.Now().Add(-1 * time.Second)

	metrics2 := &ProcessMetrics{}
	c.updateCPUPercent(metrics2, 1100) // delta of 100 ticks

	// With CLK_TCK=100 and ~1s elapsed: (100 / (1.0 * 100)) * 100 = 100%
	s.Greater(metrics2.CPUPercent, float64(0))
}

func (s *CollectorCoverageSuite) TestUpdateCPUPercent_ZeroElapsed() {
	c := New()

	metrics1 := &ProcessMetrics{}
	c.updateCPUPercent(metrics1, 1000)

	// Set prevTime to now so elapsed is effectively zero
	c.prevTime = time.Now()

	metrics2 := &ProcessMetrics{}
	c.updateCPUPercent(metrics2, 1100)

	// With zero or near-zero elapsed, it may compute a very large value or
	// the guard (elapsed > 0) might prevent it. Either way, it shouldn't panic.
	// The important thing is this doesn't crash.
}

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
