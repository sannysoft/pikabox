package health

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSpeedDownloadTimesFromFirstByte(t *testing.T) {
	savedWindow, savedMax := speedWindow, speedMaxBytes
	speedWindow, speedMaxBytes = 500*time.Millisecond, 1<<40
	defer func() { speedWindow, speedMaxBytes = savedWindow, savedMax }()

	// 64 KB every 50 ms ≈ 10.5 Mb/s, after a 300 ms wait that must not count.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		chunk := []byte(strings.Repeat("x", 64<<10))
		for i := 0; i < 40; i++ {
			if _, err := w.Write(chunk); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			time.Sleep(50 * time.Millisecond)
		}
	}))
	defer srv.Close()

	r := speedDownload(t.Context(), srv.Client(), srv.URL)
	if r.Error != "" {
		t.Fatalf("error: %s", r.Error)
	}
	if r.Mbps < 7 || r.Mbps > 14 {
		t.Fatalf("speed = %.1f Mb/s, want about 10.5", r.Mbps)
	}
}

func TestSpeedDownloadCapsBytes(t *testing.T) {
	savedMax := speedMaxBytes
	speedMaxBytes = 256 << 10
	defer func() { speedMaxBytes = savedMax }()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat("x", 8<<20)))
	}))
	defer srv.Close()

	r := speedDownload(t.Context(), srv.Client(), srv.URL)
	if r.Error != "" || r.Bytes < speedMaxBytes || r.Bytes > speedMaxBytes+128<<10 {
		t.Fatalf("result = %+v, want about %d bytes", r, speedMaxBytes)
	}
}

func TestSpeedDownloadErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	if r := speedDownload(t.Context(), srv.Client(), srv.URL); !strings.Contains(r.Error, "403") {
		t.Fatalf("error = %q", r.Error)
	}
}
