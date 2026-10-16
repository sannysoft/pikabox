package health

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// DefaultBulkURL serves 256 KiB: well past the ~16-20 KB point where DPI
// throttling of a tunnel typically freezes a transfer.
const DefaultBulkURL = "https://speed.cloudflare.com/__down?bytes=262144"

const bulkConcurrency = 4 // bulk tests share the router's uplink

var (
	bulkStall = 6 * time.Second  // no bytes for this long = stalled
	bulkTotal = 25 * time.Second // whole download budget
)

// BulkResult is one bulk download through an outbound. Delay probes move a
// few hundred bytes and pass even on a tunnel that DPI throttles; this pulls
// enough data to hit that throttling.
type BulkResult struct {
	OK    bool   `json:"ok"`
	Bytes int64  `json:"bytes"`
	KBps  int    `json:"kbps"`
	Error string `json:"error,omitempty"`
}

// bulkCheck downloads rawURL through the probe's bulk inbound as user tag.
func bulkCheck(ctx context.Context, port int, tag, rawURL string) BulkResult {
	proxy := &url.URL{Scheme: "http", Host: "127.0.0.1:" + strconv.Itoa(port), User: url.UserPassword(tag, tag)}
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true}}
	return bulkDownload(ctx, client, rawURL)
}

// bulkDownload fetches rawURL with client, failing on a stall or cut-off.
func bulkDownload(ctx context.Context, client *http.Client, rawURL string) BulkResult {
	ctx, cancel := context.WithTimeout(ctx, bulkTotal)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return BulkResult{Error: err.Error()}
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return BulkResult{Error: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return BulkResult{Error: "test URL answered " + resp.Status}
	}

	// A watchdog cancels the request when no bytes arrive for bulkStall.
	var mu sync.Mutex
	last := time.Now()
	stalled := false
	done := make(chan struct{})
	defer close(done)
	go func() {
		tick := time.NewTicker(500 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				mu.Lock()
				if time.Since(last) > bulkStall {
					stalled = true
					mu.Unlock()
					cancel()
					return
				}
				mu.Unlock()
			}
		}
	}()

	var total int64
	buf := make([]byte, 32<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			total += int64(n)
			mu.Lock()
			last = time.Now()
			mu.Unlock()
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			mu.Lock()
			wasStalled := stalled
			mu.Unlock()
			result := BulkResult{Bytes: total, KBps: kbps(total, time.Since(start))}
			if wasStalled {
				result.Error = fmt.Sprintf("stalls after %d KB", total/1024)
			} else {
				result.Error = fmt.Sprintf("cut off after %d KB: %v", total/1024, rerr)
			}
			return result
		}
	}
	if resp.ContentLength > 0 && total < resp.ContentLength {
		return BulkResult{Bytes: total, KBps: kbps(total, time.Since(start)), Error: fmt.Sprintf("cut off after %d KB", total/1024)}
	}
	return BulkResult{OK: true, Bytes: total, KBps: kbps(total, time.Since(start))}
}

func kbps(bytes int64, elapsed time.Duration) int {
	if elapsed <= 0 {
		return 0
	}
	return int(float64(bytes) / 1024 / elapsed.Seconds())
}
