package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestShouldRestart(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	old := now.Add(-time.Hour)
	cases := []struct {
		name                          string
		unknownSince, started, lastRe time.Time
		want                          bool
	}{
		{"healthy", time.Time{}, old, time.Time{}, false},
		{"unknown long enough", now.Add(-2 * time.Minute), old, time.Time{}, true},
		{"unknown too briefly", now.Add(-time.Minute), old, time.Time{}, false},
		{"just started", now.Add(-2 * time.Minute), now.Add(-time.Minute), time.Time{}, false},
		{"restarted recently", now.Add(-2 * time.Minute), old, now.Add(-5 * time.Minute), false},
	}
	for _, c := range cases {
		if got := shouldRestart(c.unknownSince, c.started, c.lastRe, now); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestExitHealthReadable(t *testing.T) {
	serve := func(proxies string, delayStatus int) *clashAPI {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/delay") {
				w.WriteHeader(delayStatus)
				w.Write([]byte(`{"message":"timeout"}`))
				return
			}
			w.Write([]byte(proxies))
		}))
		t.Cleanup(srv.Close)
		return &clashAPI{base: srv.URL, client: srv.Client()}
	}
	group := `{"proxies":{"g":{"now":"x"},"x":{"history":[]}}}`
	if !exitHealthReadable(serve(`{"proxies":{"g":{"now":"x"},"x":{"history":[{"delay":120}]}}}`, 200), "g", "http://t") {
		t.Error("exit with a delay result must be readable")
	}
	if !exitHealthReadable(serve(group, http.StatusGatewayTimeout), "g", "http://t") {
		t.Error("an untested exit whose test fails is down, not unknown")
	}
	if !exitHealthReadable(serve(`{"proxies":{"direct":{}}}`, 200), "direct", "http://t") {
		t.Error("direct exit must be readable")
	}
	dead := &clashAPI{base: "http://127.0.0.1:1", client: http.DefaultClient}
	if exitHealthReadable(dead, "g", "http://t") {
		t.Error("API not answering must be unknown")
	}
}
