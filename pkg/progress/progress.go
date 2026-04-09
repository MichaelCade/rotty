package progress

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"
)

// DefaultInterval is the rate at which the progress line is refreshed.
const DefaultInterval = 500 * time.Millisecond

// Counter is a thread-safe progress reporter that writes to stderr.
type Counter struct {
	files int64
	bytes int64
	start time.Time
	done  chan struct{}
}

// New creates and starts a Counter that prints progress every interval.
// Call Stop() when the operation is complete to print a final line.
func New(interval time.Duration) *Counter {
	c := &Counter{
		start: time.Now(),
		done:  make(chan struct{}),
	}
	go c.run(interval)
	return c
}

func (c *Counter) run(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.print(false)
		case <-c.done:
			return
		}
	}
}

func (c *Counter) print(final bool) {
	files := atomic.LoadInt64(&c.files)
	bytes := atomic.LoadInt64(&c.bytes)
	elapsed := time.Since(c.start).Truncate(time.Second)

	rate := float64(0)
	if secs := time.Since(c.start).Seconds(); secs > 0 {
		rate = float64(files) / secs
	}

	label := "Scanning"
	if final {
		label = "Scan complete"
	}

	fmt.Fprintf(os.Stderr, "\r\033[K[rottyctl] %s: %s files | %s | %.0f files/s | %s",
		label,
		formatCount(files),
		formatBytes(bytes),
		rate,
		elapsed,
	)
	if final {
		fmt.Fprintln(os.Stderr)
	}
}

// Update records a new running total. Called from the scanner progress callback.
func (c *Counter) Update(files, bytes int64) {
	atomic.StoreInt64(&c.files, files)
	atomic.StoreInt64(&c.bytes, bytes)
}

// Stop halts the ticker and prints the final summary line.
func (c *Counter) Stop() {
	close(c.done)
	c.print(true)
}

func formatCount(n int64) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

func formatBytes(b int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
		TB = 1024 * GB
	)
	switch {
	case b >= TB:
		return fmt.Sprintf("%.2f TB", float64(b)/TB)
	case b >= GB:
		return fmt.Sprintf("%.2f GB", float64(b)/GB)
	case b >= MB:
		return fmt.Sprintf("%.2f MB", float64(b)/MB)
	case b >= KB:
		return fmt.Sprintf("%.2f KB", float64(b)/KB)
	default:
		return fmt.Sprintf("%d B", b)
	}
}
