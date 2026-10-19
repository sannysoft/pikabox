package main

import (
	"reflect"
	"testing"
	"time"

	managerconfig "github.com/sannysoft/pikabox/internal/config"
	"github.com/sannysoft/pikabox/internal/health"
)

func TestPickBySpeed(t *testing.T) {
	speeds := map[string]float64{"a": 40, "b": 45, "c": 60}
	latencies := map[string]int{"a": 100, "b": 50, "c": 200, "u": 30, "v": 90}
	speed := func(m string) float64 { return speeds[m] }
	latency := func(m string) int { return latencies[m] }
	all := func(string) bool { return true }
	none := func(string) bool { return false }
	only := func(ids ...string) func(string) bool {
		return func(m string) bool {
			for _, id := range ids {
				if id == m {
					return true
				}
			}
			return false
		}
	}
	cases := []struct {
		name           string
		members        []string
		active         string
		alive, cooling func(string) bool
		want           string
	}{
		{"fastest", []string{"a", "b", "c"}, "", all, none, "c"},
		{"keeps active within margin", []string{"a", "b"}, "a", all, none, "a"}, // 45 < 40*1.25
		{"switches past margin", []string{"a", "c"}, "a", all, none, "c"},       // 60 >= 50
		{"skips dead", []string{"a", "c"}, "", only("a"), none, "a"},
		{"skips set aside", []string{"a", "c"}, "c", all, only("c"), "a"},
		{"tested before untested", []string{"u", "a"}, "", all, none, "a"},
		{"untested by latency", []string{"v", "u"}, "", all, none, "u"},
		{"nothing usable: fallback rules", []string{"a", "c"}, "", all, all, "a"},
	}
	for _, c := range cases {
		if got := pickBySpeed(c.members, c.active, c.alive, c.cooling, speed, latency); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

func TestSpeedCandidates(t *testing.T) {
	cfg := managerconfig.DefaultConfig()
	cfg.Outbounds["vp3"] = managerconfig.Outbound{ID: "vp3", Enabled: true}
	cfg.Outbounds["hk"] = managerconfig.Outbound{ID: "hk", Enabled: true}
	cfg.Subscriptions["imp"] = managerconfig.Subscription{ID: "imp", Enabled: true}
	cfg.Subscriptions["fast"] = managerconfig.Subscription{ID: "fast", Enabled: true, SelectBy: "speed"}
	cfg.OutboundGroups["g"] = managerconfig.OutboundGroup{ID: "g", Enabled: true, Mode: "speed", Members: []string{"vp3", "imp"}}
	states := map[string]managerconfig.SubscriptionState{
		"imp":  {Matched: []string{"imp.1", "imp.2"}, Selected: []string{"imp.1"}},
		"fast": {Matched: []string{"fast.1", "fast.2"}},
	}
	ok := health.EndpointResult{Health: "ok"}
	delay := map[string]health.EndpointResult{"vp3": ok, "hk": ok, "imp.1": ok, "imp.2": ok, "fast.1": ok, "fast.2": {Health: "down"}}
	stored := map[string]managerconfig.ProbeResult{
		"imp.1": {SpeedMbps: 10, SpeedCheckedAt: time.Now().UTC().Format(time.RFC3339)}, // fresh: skipped
	}
	got := speedCandidates(&cfg, states, delay, stored)
	// vp3 (speed group), not hk (unused); imp's selected server only, already
	// fresh; every alive server of the speed-picked subscription.
	if want := []string{"fast.1", "vp3"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %v, want %v", got, want)
	}
}

func TestSelectFastestBySpeed(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	cands := []managerconfig.Candidate{{ID: "s.a"}, {ID: "s.b"}, {ID: "s.c"}, {ID: "s.d"}}
	results := map[string]managerconfig.ProbeResult{
		"s.a": {Health: "ok", LatencyMS: 20, SpeedMbps: 5, SpeedCheckedAt: now},
		"s.b": {Health: "ok", LatencyMS: 90, SpeedMbps: 80, SpeedCheckedAt: now},
		"s.c": {Health: "ok", LatencyMS: 10},                                                        // untested
		"s.d": {Health: "ok", LatencyMS: 50, SpeedMbps: 99, SpeedCheckedAt: "2000-01-01T00:00:00Z"}, // stale
	}
	if got := selectFastest(cands, results, 2, true, time.Hour); !reflect.DeepEqual(got, []string{"s.b", "s.a"}) {
		t.Fatalf("by speed = %v", got)
	}
	if got := selectFastest(cands, results, 2, false, time.Hour); !reflect.DeepEqual(got, []string{"s.c", "s.a"}) {
		t.Fatalf("by latency = %v", got)
	}
}
