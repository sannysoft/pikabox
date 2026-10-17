package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	managerconfig "github.com/sannysoft/pikabox/internal/config"
	"github.com/sannysoft/pikabox/internal/dialstats"
	"github.com/sannysoft/pikabox/internal/render"
)

func TestPickFallback(t *testing.T) {
	set := func(ids ...string) func(string) bool {
		return func(id string) bool {
			for _, x := range ids {
				if x == id {
					return true
				}
			}
			return false
		}
	}
	cases := []struct {
		members        []string
		alive, cooling func(string) bool
		want           string
	}{
		{[]string{"a", "b"}, set("a", "b"), set(), "a"},
		{[]string{"a", "b"}, set("a", "b"), set("a"), "b"}, // a set aside
		{[]string{"a", "b"}, set("a"), set("a"), "a"},      // only a alive: used anyway
		{[]string{"a", "b"}, set(), set(), "b"},            // nothing alive: park on last
		{[]string{"a", "direct", "b"}, set("a", "b"), set("a"), "direct"},
	}
	for _, c := range cases {
		if got := pickFallback(c.members, c.alive, c.cooling); got != c.want {
			t.Errorf("pickFallback(%v) = %s, want %s", c.members, got, c.want)
		}
	}
}

func TestFailoverCheckSetsAsideAndBacksOff(t *testing.T) {
	saved := dialStats
	dialStats = dialstats.New()
	defer func() { dialStats = saved }()
	failover.until, failover.level = map[string]time.Time{}, map[string]int{}

	cfg := managerconfig.DefaultConfig()
	plan := render.Plan{Outbounds: []map[string]any{
		{"type": "vless", "tag": "vp3", "server": "84.32.149.31", "server_port": 443},
		{"type": "selector", "tag": "foreign", "outbounds": []string{"vp3"}},
	}}
	servers := outboundServers(plan)
	now := time.Unix(1_800_000_000, 0)
	next := 0
	feed := func(n, failed int) {
		for i := 0; i < n; i++ {
			next++
			id := fmt.Sprint(next)
			dialStats.Feed("INFO ["+id+" 0ms] outbound/vless[vp3]: outbound connection to x:443", now, servers)
			if i < failed {
				dialStats.Feed("ERROR ["+id+" 5s] connection: open connection to x:443 using outbound/vless[vp3]: dial tcp 84.32.149.31:443: i/o timeout", now, servers)
			}
		}
	}

	feed(20, 10) // too few connections to judge
	if reason := failoverCheck(cfg, plan, "vp3", now); reason != "" {
		t.Fatalf("judged on 20 connections: %s", reason)
	}
	feed(20, 0) // 40 connections, 25% failed
	reason := failoverCheck(cfg, plan, "vp3", now)
	if !strings.Contains(reason, "10 of 40") || !strings.Contains(reason, "set aside for 10m") {
		t.Fatalf("reason = %q", reason)
	}
	if until, cooling := coolingDown("vp3", now); !cooling || until != now.Add(10*time.Minute) {
		t.Fatalf("cooling = %v until %v", cooling, until)
	}
	if a, _ := dialStats.Stats([]string{"vp3"}, time.Hour, now); a != 0 {
		t.Fatalf("stats not reset: %d", a)
	}

	feed(40, 10) // fails again: the cooldown doubles
	if reason := failoverCheck(cfg, plan, "vp3", now); !strings.Contains(reason, "set aside for 20m") {
		t.Fatalf("second reason = %q", reason)
	}
	feed(40, 1) // healthy (2.5%): repeats forgotten
	if reason := failoverCheck(cfg, plan, "vp3", now); reason != "" || failover.level["vp3"] != 0 {
		t.Fatalf("healthy check: %q level %d", reason, failover.level["vp3"])
	}

	// The group's leaves are judged together; 0 disables.
	cfg.Manager.FailoverRate = 0
	feed(40, 40)
	if reason := failoverCheck(cfg, plan, "foreign", now); reason != "" {
		t.Fatalf("disabled but judged: %s", reason)
	}
	cfg.Manager.FailoverRate = 5
	if reason := failoverCheck(cfg, plan, "foreign", now); !strings.Contains(reason, "41 of 80") {
		t.Fatalf("group reason = %q", reason)
	}
}

func TestLineLevel(t *testing.T) {
	for line, want := range map[string]int{
		"+0000 2026-10-17 17:54:28 ERROR [1 5s] connection: x": logLevels["error"],
		"+0000 2026-10-17 17:54:28 WARN dns: x":                logLevels["warn"],
		"+0000 2026-10-17 17:54:28 INFO [1 0ms] outbound/x":    logLevels["info"],
		"FATAL decode config":                                  logLevels["fatal"],
	} {
		if got := lineLevel(line); got != want {
			t.Errorf("lineLevel(%q) = %d, want %d", line, got, want)
		}
	}
}
