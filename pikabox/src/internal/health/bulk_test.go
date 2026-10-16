package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBulkDownloadDetectsStallAfterFirstKilobytes(t *testing.T) {
	bulkStall = 300 * time.Millisecond
	defer func() { bulkStall = 6 * time.Second }()
	release := make(chan struct{})
	defer close(release)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "262144")
		if r.URL.Path == "/throttled" {
			// Like DPI cutting a tunnel at ~16 KB: send some, then nothing.
			_, _ = w.Write(make([]byte, 16<<10))
			w.(http.Flusher).Flush()
			select {
			case <-release:
			case <-r.Context().Done():
			}
			return
		}
		_, _ = w.Write(make([]byte, 256<<10))
	}))
	defer srv.Close()

	ok := bulkDownload(context.Background(), srv.Client(), srv.URL+"/fine")
	if !ok.OK || ok.Bytes != 256<<10 {
		t.Fatalf("full download = %#v", ok)
	}
	bad := bulkDownload(context.Background(), srv.Client(), srv.URL+"/throttled")
	if bad.OK || bad.Bytes != 16<<10 || !strings.Contains(bad.Error, "stalls after 16 KB") {
		t.Fatalf("throttled download = %#v", bad)
	}
}
