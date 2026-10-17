package main

import (
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	managerconfig "github.com/sannysoft/pikabox/internal/config"
	"github.com/sannysoft/pikabox/internal/dialstats"
	"github.com/sannysoft/pikabox/internal/render"
)

// dialStats counts connections and server-side failures per outbound from
// sing-box's log; the fallback controller sets aside a member that fails too
// often (see failoverCheck).
var dialStats = dialstats.New()

const maxSingBoxLog = 2 << 20 // bytes kept in tmpfs before the log is emptied

var logLevels = map[string]int{"trace": 0, "debug": 1, "info": 2, "warn": 3, "error": 4, "fatal": 5, "panic": 6}

// startDialStats follows sing-box's log: every line feeds the counters, and
// lines at the configured log_level or above go on to the system log, as when
// sing-box wrote to stderr.
func startDialStats(configPath string) {
	var (
		mu      sync.Mutex
		servers map[string]string
		level   = logLevels["warn"]
	)
	refresh := func() {
		cfg, err := managerconfig.Load(configPath)
		if err != nil {
			return
		}
		next := outboundServers(render.BuildPlan(*cfg))
		mu.Lock()
		servers = next
		if l, known := logLevels[cfg.Manager.LogLevel]; known {
			level = l
		}
		mu.Unlock()
	}
	refresh()
	go func() {
		for range time.Tick(30 * time.Second) {
			refresh()
		}
	}()
	go dialstats.Follow(render.SingBoxLogPath, maxSingBoxLog, func(line string) {
		mu.Lock()
		current, minLevel := servers, level
		mu.Unlock()
		dialStats.Feed(line, time.Now(), current)
		if lineLevel(line) >= minLevel {
			fmt.Fprintln(os.Stderr, line)
		}
	})
}

// lineLevel finds the level word in "+0000 2026-10-17 17:54:28 ERROR [...] ...".
func lineLevel(line string) int {
	for i, field := range strings.Fields(line) {
		if i > 3 {
			break
		}
		if l, known := logLevels[strings.ToLower(field)]; known {
			return l
		}
	}
	return logLevels["info"]
}

// outboundServers maps each proxy outbound tag to its server "host:port".
func outboundServers(plan render.Plan) map[string]string {
	servers := map[string]string{}
	for _, outbound := range plan.Outbounds {
		tag, _ := outbound["tag"].(string)
		server, _ := outbound["server"].(string)
		if tag == "" || server == "" {
			continue
		}
		servers[tag] = net.JoinHostPort(server, fmt.Sprint(outbound["server_port"]))
	}
	return servers
}

// leafOutbounds expands a group or subscription tag into the proxy outbounds
// that can carry its traffic; an outbound is its own leaf.
func leafOutbounds(plan render.Plan, tag string) []string {
	children := map[string][]string{}
	for _, outbound := range plan.Outbounds {
		id, _ := outbound["tag"].(string)
		if members, isGroup := outbound["outbounds"].([]string); isGroup {
			children[id] = members
		}
	}
	var leaves []string
	seen := map[string]bool{}
	var walk func(string)
	walk = func(id string) {
		if seen[id] {
			return
		}
		seen[id] = true
		if members, isGroup := children[id]; isGroup {
			for _, member := range members {
				walk(member)
			}
			return
		}
		leaves = append(leaves, id)
	}
	walk(tag)
	return leaves
}

// failover remembers members set aside for failing too often.
var failover = struct {
	sync.Mutex
	until map[string]time.Time // member → when it may be tried again
	level map[string]int       // repeats so far; the cooldown doubles each time
}{until: map[string]time.Time{}, level: map[string]int{}}

const maxFailoverCooldown = time.Hour

// coolingDown reports whether member is set aside, and until when.
func coolingDown(member string, now time.Time) (time.Time, bool) {
	failover.Lock()
	defer failover.Unlock()
	until, found := failover.until[member]
	return until, found && now.Before(until)
}

// failoverCheck judges a fallback group's active member on its recent
// connections. It returns a message when the member was set aside.
func failoverCheck(cfg managerconfig.Config, plan render.Plan, member string, now time.Time) string {
	m := cfg.Manager
	if m.FailoverRate <= 0 || member == "" || member == "direct" || member == "block" {
		return ""
	}
	window := durationOr(m.FailoverWindow, 5*time.Minute, time.Minute)
	leaves := leafOutbounds(plan, member)
	attempts, failures := dialStats.Stats(leaves, window, now)
	if attempts < m.FailoverMinAttempts {
		return ""
	}
	failover.Lock()
	defer failover.Unlock()
	share := float64(failures) * 100 / float64(attempts)
	if share <= m.FailoverRate {
		delete(failover.level, member) // healthy over a full sample: forget repeats
		return ""
	}
	cooldown := durationOr(m.FailoverCooldown, 10*time.Minute, time.Minute) << failover.level[member]
	if cooldown > maxFailoverCooldown || cooldown <= 0 {
		cooldown = maxFailoverCooldown
	}
	failover.until[member] = now.Add(cooldown)
	failover.level[member]++
	dialStats.Reset(leaves) // judged afresh when tried again
	return fmt.Sprintf("%d of %d connections (%.1f%%) failed to reach the server in the last %s; set aside for %s",
		failures, attempts, share, shortDuration(window), shortDuration(cooldown))
}

// pickFallback returns the first usable member in written order: direct/block
// always, otherwise one that is alive and not set aside. When no such member
// is left the first alive one is used even if set aside; when none is alive
// the group parks on its last member.
func pickFallback(members []string, alive func(string) bool, cooling func(string) bool) string {
	firstAlive := ""
	for _, member := range members {
		if member == "direct" || member == "block" {
			return member
		}
		if !alive(member) {
			continue
		}
		if !cooling(member) {
			return member
		}
		if firstAlive == "" {
			firstAlive = member
		}
	}
	if firstAlive != "" {
		return firstAlive
	}
	return members[len(members)-1]
}

// dialView is an outbound's recent connection record for the UI.
type dialView struct {
	Attempts     int    `json:"attempts"`
	Failures     int    `json:"failures"`
	Window       string `json:"window"`
	CoolingUntil string `json:"cooling_until,omitempty"`
}

func dialViewFor(cfg managerconfig.Config, tag string) *dialView {
	now := time.Now()
	window := durationOr(cfg.Manager.FailoverWindow, 5*time.Minute, time.Minute)
	attempts, failures := dialStats.Stats([]string{tag}, window, now)
	view := &dialView{Attempts: attempts, Failures: failures, Window: shortDuration(window)}
	if until, cooling := coolingDown(tag, now); cooling {
		view.CoolingUntil = until.UTC().Format(time.RFC3339)
	}
	return view
}

// shortDuration prints 5m0s as 5m and 1h0m0s as 1h.
func shortDuration(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}
