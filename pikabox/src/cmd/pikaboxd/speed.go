package main

import (
	"sort"
	"strings"
	"time"

	managerconfig "github.com/sannysoft/pikabox/internal/config"
	"github.com/sannysoft/pikabox/internal/health"
	"github.com/sannysoft/pikabox/internal/render"
)

// speedMargin is how much faster another member must be before a speed group
// leaves the one it uses: every switch cuts the group's open connections.
const speedMargin = 1.25

// speedURL is the configured speed test URL, or "" when the test is off.
func speedURL(cfg *managerconfig.Config) string {
	if u := strings.TrimSpace(cfg.Manager.SpeedTestURL); u != "off" {
		return u
	}
	return ""
}

// speedTTL is how long a speed result is trusted.
func speedTTL(cfg *managerconfig.Config) time.Duration {
	return durationOr(cfg.Manager.SpeedInterval, 3*time.Hour, 30*time.Minute)
}

// speedCandidates picks what to speed-test this round: every alive outbound
// that something chooses by speed (a speed group's members, expanded to
// their outbounds; every alive server of a speed-picked subscription) whose
// last result is older than speed_interval.
func speedCandidates(cfg *managerconfig.Config, states map[string]managerconfig.SubscriptionState, delay map[string]health.EndpointResult, stored map[string]managerconfig.ProbeResult) []string {
	want := map[string]bool{}
	var expand func(id string, seen map[string]bool)
	expand = func(id string, seen map[string]bool) {
		if seen[id] {
			return
		}
		seen[id] = true
		switch cfg.Kind(id) {
		case "outbound":
			want[id] = true
		case "subscription":
			state := states[id]
			tags := state.Selected
			if cfg.Subscriptions[id].SelectBy == "speed" {
				tags = state.Matched
			}
			for _, tag := range tags {
				want[tag] = true
			}
		case "group":
			for _, member := range cfg.OutboundGroups[id].Members {
				expand(member, seen)
			}
		}
	}
	for id, group := range cfg.OutboundGroups {
		if group.Enabled && group.Mode == "speed" {
			expand(id, map[string]bool{})
		}
	}
	for id, sub := range cfg.Subscriptions {
		if sub.Enabled && sub.SelectBy == "speed" {
			for _, tag := range states[id].Matched {
				want[tag] = true
			}
		}
	}
	ttl := speedTTL(cfg)
	var pick []string
	for tag := range want {
		if delay[tag].Health == "ok" && !stored[tag].SpeedFresh(ttl) {
			pick = append(pick, tag)
		}
	}
	sort.Strings(pick)
	return pick
}

// pickBySpeed returns the usable member with the highest measured speed;
// members without a result rank after measured ones, by latency. It stays on
// active unless the best is speedMargin faster. With no usable member it
// behaves like a fallback group (first alive even if set aside, else last).
func pickBySpeed(members []string, active string, alive, cooling func(string) bool, speed func(string) float64, latency func(string) int) string {
	better := func(a, b string) bool {
		sa, sb := speed(a), speed(b)
		if sa > 0 || sb > 0 {
			return sa > sb
		}
		la, lb := latency(a), latency(b)
		return la > 0 && (lb == 0 || la < lb)
	}
	best := ""
	for _, member := range members {
		if member == "direct" || member == "block" || !alive(member) || cooling(member) {
			continue
		}
		if best == "" || better(member, best) {
			best = member
		}
	}
	if best == "" {
		return pickFallback(members, alive, cooling)
	}
	if best != active && active != "" && alive(active) && !cooling(active) {
		if s := speed(active); s > 0 && speed(best) < s*speedMargin {
			return active
		}
	}
	return best
}

// memberSpeed is the best fresh speed among a member's outbounds (0 = none);
// memberLatency the lowest alive latency (0 = none).
func memberSpeed(plan render.Plan, results map[string]managerconfig.ProbeResult, ttl time.Duration, member string) float64 {
	best := 0.0
	for _, tag := range leafOutbounds(plan, member) {
		if r := results[tag]; r.Health == "ok" && r.SpeedFresh(ttl) && r.SpeedMbps > best {
			best = r.SpeedMbps
		}
	}
	return best
}

func memberLatency(plan render.Plan, results map[string]managerconfig.ProbeResult, member string) int {
	best := 0
	for _, tag := range leafOutbounds(plan, member) {
		if r := results[tag]; r.Health == "ok" && r.LatencyMS > 0 && (best == 0 || r.LatencyMS < best) {
			best = r.LatencyMS
		}
	}
	return best
}
