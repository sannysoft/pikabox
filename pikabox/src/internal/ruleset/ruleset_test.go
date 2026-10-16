package ruleset

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	managerconfig "github.com/sannysoft/pikabox/internal/config"
)

func TestDownloadWritesRuleset(t *testing.T) {
	originalTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("srs-data")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})
	t.Cleanup(func() {
		http.DefaultClient.Transport = originalTransport
	})

	path := filepath.Join(t.TempDir(), "geoip-test.srs")
	result, err := Download(context.Background(), managerconfig.RuleSet{
		ID:     "geoip-test",
		Format: "binary",
		URL:    "https://example.com/geoip-test.srs",
		Path:   path,
	}, "")
	if err != nil {
		t.Fatalf("download ruleset: %v", err)
	}
	if result.Path != path || result.Bytes != int64(len("srs-data")) {
		t.Fatalf("result = %#v", result)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read downloaded ruleset: %v", err)
	}
	if string(data) != "srs-data" {
		t.Fatalf("downloaded data = %q", data)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestDueUsesCachedFileAge(t *testing.T) {
	now := time.Now()
	entry := managerconfig.RuleSet{ID: "geoip-test", URL: "https://example.com/geoip-test.srs", Path: filepath.Join(t.TempDir(), "x.srs")}
	if !Due(entry, now) {
		t.Fatal("missing cache file must be due")
	}
	if err := os.WriteFile(entry.Path, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if Due(entry, now) {
		t.Fatal("fresh cache file must not be due")
	}
	old := now.Add(-169 * time.Hour)
	if err := os.Chtimes(entry.Path, old, old); err != nil {
		t.Fatal(err)
	}
	if !Due(entry, now) {
		t.Fatal("week-old cache file must be due")
	}
}
