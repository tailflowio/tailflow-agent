package metrics

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ProcessMetrics struct {
	CPUPercent float64 `json:"cpu_percent"`
	RSSKB      int64   `json:"rss_kb"`
	Goroutines int     `json:"goroutines"`
	HeapMB     float64 `json:"heap_mb"`
	NetRxBytes int64   `json:"net_rx_bytes"`
	NetTxBytes int64   `json:"net_tx_bytes"`
	UptimeS    int64   `json:"uptime_s"`
	Available  bool    `json:"available"`
}

type Collector struct {
	mu           sync.RWMutex
	snap         ProcessMetrics
	startAt      time.Time
	prevCPUTicks int64
	prevTime     time.Time
}

func New() *Collector {
	return &Collector{
		startAt: time.Now(),
	}
}

func (c *Collector) Start(ctx context.Context) {
	c.collect()

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.collect()
			}
		}
	}()
}

func (c *Collector) Snapshot() ProcessMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.snap
}

func (c *Collector) collect() {
	var memStats runtime.MemStats

	runtime.ReadMemStats(&memStats)

	metrics := ProcessMetrics{
		Goroutines: runtime.NumGoroutine(),
		HeapMB:     float64(memStats.HeapAlloc) / (1024 * 1024),
		UptimeS:    int64(time.Since(c.startAt).Seconds()),
	}

	cpuTicks, cpuOK := readCPUTicks()
	rss, rssOK := readRSSKB()
	rx, tx, netOK := readNetDev()

	if cpuOK || rssOK || netOK {
		metrics.Available = true
	}

	if rssOK {
		metrics.RSSKB = rss
	}

	if netOK {
		metrics.NetRxBytes = rx
		metrics.NetTxBytes = tx
	}

	if cpuOK {
		c.updateCPUPercent(&metrics, cpuTicks)
	}

	c.mu.Lock()
	c.snap = metrics
	c.mu.Unlock()
}

func (c *Collector) updateCPUPercent(metrics *ProcessMetrics, cpuTicks int64) {
	now := time.Now()

	if c.prevTime.IsZero() {
		c.prevCPUTicks = cpuTicks
		c.prevTime = now

		return
	}

	deltaTicks := cpuTicks - c.prevCPUTicks
	elapsed := now.Sub(c.prevTime).Seconds()

	// CLK_TCK is 100 on Linux
	if elapsed > 0 {
		metrics.CPUPercent = float64(deltaTicks) / (elapsed * 100) * 100
	}

	c.prevCPUTicks = cpuTicks
	c.prevTime = now
}

func readCPUTicks() (int64, bool) {
	data, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return 0, false
	}

	// comm field (field 2) is in parens and may contain spaces
	statContent := string(data)
	closingParenIdx := strings.LastIndex(statContent, ")")

	if closingParenIdx < 0 || closingParenIdx+2 >= len(statContent) {
		return 0, false
	}

	fields := strings.Fields(statContent[closingParenIdx+2:])
	if len(fields) < 13 {
		return 0, false
	}

	utime, err1 := strconv.ParseInt(fields[11], 10, 64)
	stime, err2 := strconv.ParseInt(fields[12], 10, 64)

	if err1 != nil || err2 != nil {
		return 0, false
	}

	return utime + stime, true
}

func readRSSKB() (int64, bool) {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return 0, false
	}

	defer f.Close()

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, false
		}

		rssValue, parseErr := strconv.ParseInt(fields[1], 10, 64)
		if parseErr != nil {
			return 0, false
		}

		return rssValue, true
	}

	return 0, false
}

func readNetDev() (rx int64, tx int64, ok bool) {
	f, err := os.Open("/proc/self/net/dev")
	if err != nil {
		return 0, 0, false
	}

	defer f.Close()

	scanner := bufio.NewScanner(f)
	headerLines := 2
	lineNum := 0

	for scanner.Scan() {
		lineNum++

		if lineNum <= headerLines {
			continue
		}

		parts := strings.SplitN(scanner.Text(), ":", 2)
		if len(parts) != 2 {
			continue
		}

		fields := strings.Fields(parts[1])
		if len(fields) < 10 {
			continue
		}

		rxBytes, err1 := strconv.ParseInt(fields[0], 10, 64)
		txBytes, err2 := strconv.ParseInt(fields[8], 10, 64)

		if err1 != nil || err2 != nil {
			continue
		}

		rx += rxBytes
		tx += txBytes
		ok = true
	}

	return rx, tx, ok
}

func (m ProcessMetrics) String() string {
	return fmt.Sprintf("cpu=%.1f%% rss=%dkB goroutines=%d heap=%.1fMB rx=%d tx=%d uptime=%ds available=%v",
		m.CPUPercent, m.RSSKB, m.Goroutines, m.HeapMB, m.NetRxBytes, m.NetTxBytes, m.UptimeS, m.Available)
}
