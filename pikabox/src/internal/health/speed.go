package health

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

var (
	// speedWindow is how long the transfer is timed once data flows: long
	// enough to get past TCP slow start, short enough to keep traffic small.
	speedWindow = 4 * time.Second
	// speedFirstByte bounds the wait for the first byte (handshake + request).
	speedFirstByte = 10 * time.Second
	// speedMaxBytes caps a test even on a fast link.
	speedMaxBytes int64 = 10_000_000
)

// SpeedResult is one timed download through an outbound.
type SpeedResult struct {
	Mbps  float64 `json:"mbps"`
	Bytes int64   `json:"bytes"`
	Error string  `json:"error,omitempty"`
}

// speedCheck downloads rawURL through the probe's per-outbound inbound as user
// tag and reports the throughput.
func speedCheck(ctx context.Context, port int, tag, rawURL string) SpeedResult {
	proxy := &url.URL{Scheme: "http", Host: "127.0.0.1:" + strconv.Itoa(port), User: url.UserPassword(tag, tag)}
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true}}
	return speedDownload(ctx, client, rawURL)
}

// speedDownload times the body of rawURL from its first byte for speedWindow
// (or until speedMaxBytes or the end of the body) and returns Mb/s. Timing
// from the first byte keeps the server's latency out of the figure.
func speedDownload(ctx context.Context, client *http.Client, rawURL string) SpeedResult {
	ctx, cancel := context.WithTimeout(ctx, speedFirstByte+speedWindow+time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return SpeedResult{Error: err.Error()}
	}
	resp, err := client.Do(req)
	if err != nil {
		return SpeedResult{Error: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return SpeedResult{Error: "speed test URL answered " + resp.Status}
	}

	buf := make([]byte, 64<<10)
	var (
		total int64
		start time.Time
	)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 && start.IsZero() {
			start = time.Now() // the first chunk only starts the clock
		} else {
			total += int64(n)
		}
		if !start.IsZero() && (time.Since(start) >= speedWindow || total >= speedMaxBytes) {
			break
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			if total == 0 {
				return SpeedResult{Error: rerr.Error()}
			}
			break // timed out mid-transfer: what arrived still measures the speed
		}
	}
	elapsed := time.Since(start)
	if start.IsZero() || total == 0 || elapsed <= 0 {
		return SpeedResult{Error: "no data received"}
	}
	return SpeedResult{Mbps: float64(total) * 8 / 1e6 / elapsed.Seconds(), Bytes: total}
}

// String formats r for logs.
func (r SpeedResult) String() string {
	if r.Error != "" {
		return "error: " + r.Error
	}
	return fmt.Sprintf("%.1f Mb/s (%d KB)", r.Mbps, r.Bytes/1024)
}
