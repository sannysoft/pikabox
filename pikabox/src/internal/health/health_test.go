package health

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
)

func TestTestURLMeasuresSuccessfulHTTP(t *testing.T) {
	previous := httpClient
	httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Body:       io.NopCloser(bytes.NewReader(nil)),
			Request:    req,
		}, nil
	})}
	defer func() {
		httpClient = previous
	}()

	result, err := TestURL(context.Background(), "https://example.com/generate_204")
	if err != nil {
		t.Fatalf("test url: %v", err)
	}
	if result.Health != "ok" {
		t.Fatalf("health = %q, want ok", result.Health)
	}
	if result.LatencyMS <= 0 {
		t.Fatalf("latency = %d, want positive", result.LatencyMS)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
