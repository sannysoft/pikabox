package main

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	managerconfig "github.com/sannysoft/pikabox/internal/config"
	"github.com/sannysoft/pikabox/internal/health"
	"github.com/sannysoft/pikabox/internal/render"
)

func TestCmdlineHasArgMatchesExactRuntimeConfig(t *testing.T) {
	cmdline := []byte("/usr/bin/sing-box\x00run\x00-c\x00/var/run/sing-box/config.json\x00")
	if !cmdlineHasArg(cmdline, "/var/run/sing-box/config.json") {
		t.Fatal("expected runtime config argument to match")
	}
	if cmdlineHasArg(cmdline, "/tmp/other-config.json") {
		t.Fatal("unexpected match for other config")
	}
}

func TestCollectDevicesDropsInfraAndKeepsHosts(t *testing.T) {
	leases := []Device{
		{IP: "192.168.1.10", MAC: "aa:bb:cc:dd:ee:10", Name: "laptop", Source: "dhcp"},
		{IP: "192.168.1.1", MAC: "aa:bb:cc:dd:ee:01", Name: "router", Source: "dhcp"},
	}
	arp := []Device{
		{IP: "192.168.1.20", MAC: "aa:bb:cc:dd:ee:20", Source: "arp"},
		{IP: "192.168.1.10", MAC: "aa:bb:cc:dd:ee:10", Source: "arp"},  // dup of lease
		{IP: "192.168.1.255", MAC: "ff:ff:ff:ff:ff:ff", Source: "arp"}, // broadcast
		{IP: "224.0.0.251", MAC: "01:00:5e:00:00:fb", Source: "arp"},   // multicast
		{IP: "169.254.1.5", MAC: "aa:bb:cc:dd:ee:fe", Source: "arp"},   // link-local
	}
	skip := map[string]bool{
		"192.168.1.1":   true, // gateway / router self
		"192.168.1.255": true, // directed broadcast
	}

	got := collectDevices(leases, arp, skip)
	if len(got) != 2 {
		t.Fatalf("collectDevices returned %d devices, want 2: %#v", len(got), got)
	}
	if got[0].IP != "192.168.1.10" || got[1].IP != "192.168.1.20" {
		t.Fatalf("devices = %#v, want sorted 192.168.1.10, 192.168.1.20", got)
	}
}

func TestBroadcastIP(t *testing.T) {
	cases := []struct {
		cidr string
		want string
	}{
		{"192.168.1.1/24", "192.168.1.255"},
		{"10.0.0.1/8", "10.255.255.255"},
		{"172.16.5.4/30", "172.16.5.7"},
		{"fe80::1/64", ""}, // IPv6 has no broadcast
	}
	for _, tc := range cases {
		ip, ipNet, err := net.ParseCIDR(tc.cidr)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.cidr, err)
		}
		ipNet.IP = ip // use the host address, not the network address
		if got := broadcastIP(ipNet); got != tc.want {
			t.Errorf("broadcastIP(%q) = %q, want %q", tc.cidr, got, tc.want)
		}
	}
}

func TestDefaultGatewaysParsesLittleEndianHex(t *testing.T) {
	dir := t.TempDir()
	routePath := filepath.Join(dir, "route")
	// Default route via 192.168.2.1; the LAN route has no gateway (00000000).
	route := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"eth0\t00000000\t0102A8C0\t0003\t0\t0\t0\t00000000\t0\t0\t0\n" +
		"br-lan\t0002A8C0\t00000000\t0001\t0\t0\t0\t00FFFFFF\t0\t0\t0\n"
	if err := os.WriteFile(routePath, []byte(route), 0644); err != nil {
		t.Fatalf("write route: %v", err)
	}

	gws := defaultGateways(routePath)
	if len(gws) != 1 || gws[0] != "192.168.2.1" {
		t.Fatalf("defaultGateways = %#v, want [192.168.2.1]", gws)
	}
}

func TestIsPickableHostIP(t *testing.T) {
	pickable := []string{"192.168.1.10", "10.0.0.5", "2001:db8::5"}
	for _, ip := range pickable {
		if !isPickableHostIP(ip) {
			t.Errorf("isPickableHostIP(%q) = false, want true", ip)
		}
	}
	rejected := []string{"", "not-an-ip", "0.0.0.0", "127.0.0.1", "255.255.255.255", "224.0.0.1", "169.254.0.1", "fe80::1", "::1"}
	for _, ip := range rejected {
		if isPickableHostIP(ip) {
			t.Errorf("isPickableHostIP(%q) = true, want false", ip)
		}
	}
}

func TestSelectFastest(t *testing.T) {
	candidates := []managerconfig.Candidate{{ID: "s.a"}, {ID: "s.b"}, {ID: "s.c"}, {ID: "s.d"}}
	results := map[string]managerconfig.ProbeResult{
		"s.a": {Health: "ok", LatencyMS: 300},
		"s.b": {Health: "down"},
		"s.c": {Health: "ok", LatencyMS: 100},
		"s.d": {Health: "ok", LatencyMS: 200},
	}
	cases := []struct {
		count int
		want  []string
	}{
		{2, []string{"s.c", "s.d"}},
		{0, []string{"s.c", "s.d", "s.a"}},
		{10, []string{"s.c", "s.d", "s.a"}},
	}
	for _, tc := range cases {
		if got := selectFastest(candidates, results, tc.count); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("selectFastest(count=%d) = %v, want %v", tc.count, got, tc.want)
		}
	}
	if got := selectFastest(candidates, nil, 3); len(got) != 0 {
		t.Errorf("no results must select nothing, got %v", got)
	}
}

func TestSameSet(t *testing.T) {
	cases := []struct {
		a, b []string
		want bool
	}{
		{[]string{"a", "b"}, []string{"b", "a"}, true},
		{nil, []string{}, true},
		{[]string{"a"}, []string{"a", "b"}, false},
		{[]string{"a", "b"}, []string{"a", "c"}, false},
	}
	for _, tc := range cases {
		if got := sameSet(tc.a, tc.b); got != tc.want {
			t.Errorf("sameSet(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestHealthGraphColumnsAndEdges(t *testing.T) {
	managerconfig.StateDir, managerconfig.RunDir = t.TempDir(), t.TempDir()
	cfg := managerconfig.DefaultConfig()
	cfg.Manager.Final = "main"
	cfg.Outbounds["a"] = managerconfig.Outbound{ID: "a", Enabled: true, Name: "a", Source: `{"type":"socks","server":"1.2.3.4","server_port":1080}`}
	cfg.Outbounds["spare"] = managerconfig.Outbound{ID: "spare", Enabled: true, Name: "spare", Source: `{"type":"socks","server":"1.2.3.5","server_port":1080}`}
	cfg.Subscriptions["s"] = managerconfig.Subscription{ID: "s", Enabled: true, Name: "s", URL: "https://e/s", Format: "auto"}
	cfg.OutboundGroups["main"] = managerconfig.OutboundGroup{ID: "main", Enabled: true, Name: "main", Mode: "fallback", Members: []string{"a", "s"}}
	cfg.RuleGroups["ru"] = managerconfig.RuleGroup{ID: "ru", Enabled: true, Name: "ru", Rules: []string{"domain_suffix:ru"}}
	cfg.Routes = []managerconfig.Route{{ID: "route_1", Enabled: true, RuleGroup: "ru", Outbound: "a"}}
	cand := managerconfig.Candidate{ID: "s.x", Name: "srv", Outbound: map[string]any{"type": "socks", "tag": "s.x", "server": "9.9.9.9", "server_port": 1080}}
	if err := managerconfig.SaveSubscriptionState("s", managerconfig.SubscriptionState{Candidates: []managerconfig.Candidate{cand}, Matched: []string{"s.x"}, Selected: []string{"s.x"}}); err != nil {
		t.Fatal(err)
	}
	plan := render.BuildPlan(cfg)
	items := []healthItem{{ID: "main", Kind: "group"}, {ID: "s", Kind: "subscription"}, {ID: "a", Kind: "outbound"}, {ID: "spare", Kind: "outbound"}}
	nodes, edges := healthGraph(&cfg, plan, items, nil, nil)
	col := map[string]int{}
	unused := map[string]bool{}
	for _, n := range nodes {
		col[n.ID], unused[n.ID] = n.Column, n.Unused
	}
	// a is both a route target (column 1) and a member of main (column 2):
	// it sits at its deepest position, once.
	want := map[string]int{"route:route_1": 0, "route:final": 0, "main": 1, "a": 2, "s": 2, "s.x": 3}
	for id, c := range want {
		if col[id] != c {
			t.Errorf("%s column = %d, want %d (all: %v)", id, col[id], c, col)
		}
	}
	if !unused["spare"] || unused["a"] || unused["s.x"] {
		t.Errorf("only spare is unreachable from routes: %v", unused)
	}
	links := map[string]bool{}
	for _, e := range edges {
		links[e.From+">"+e.To] = true
	}
	for _, l := range []string{"route:route_1>a", "route:final>main", "main>a", "main>s", "s>s.x"} {
		if !links[l] {
			t.Errorf("missing edge %s in %v", l, links)
		}
	}
}

func TestImportConfigDryRunThenApply(t *testing.T) {
	managerconfig.StateDir, managerconfig.RunDir, BackupDir = t.TempDir(), t.TempDir(), t.TempDir()
	dir := t.TempDir()
	path := filepath.Join(dir, "pikabox")
	current := "config manager 'main'\n\toption enabled '1'\n"
	if err := os.WriteFile(path, []byte(current), 0644); err != nil {
		t.Fatal(err)
	}
	incoming := "config manager 'main'\n\toption enabled '0'\n\toption final 'vp1'\n\nconfig outbound 'vp1'\n\toption source '{\"type\":\"socks\",\"server\":\"1.2.3.4\",\"server_port\":1080}'\n"
	params, _ := json.Marshal(map[string]any{"text": incoming, "dry_run": true})
	res := importConfigRPC(path, params)
	if res["ok"] != true || res["summary"].(importSummary).Outbounds != 1 {
		t.Fatalf("dry run = %#v", res)
	}
	if data, _ := os.ReadFile(path); string(data) != current {
		t.Fatal("a dry run must not touch the config")
	}
	if bad := importConfigRPC(path, json.RawMessage(`{"text":"config bogus x\n"}`)); bad["ok"] != false {
		t.Fatalf("invalid file must be rejected: %#v", bad)
	}
	params, _ = json.Marshal(map[string]any{"text": incoming})
	if res := importConfigRPC(path, params); res["ok"] != true || res["backup"] == "" {
		t.Fatalf("import = %#v", res)
	}
	cfg, err := managerconfig.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, has := cfg.Outbounds["vp1"]; !has || !cfg.Manager.Enabled {
		t.Fatalf("imported config must apply but keep this router's enabled=1: %#v", cfg.Manager)
	}
	if backups := listBackups(); len(backups) != 1 {
		t.Fatalf("backups = %v", backups)
	}
}

func TestSettingsPayloadAcceptsNumbers(t *testing.T) {
	p, err := decode[settingsPayload](json.RawMessage(`{"settings":{"mixed_port":2081,"log_level":"warn"}}`), "settings")
	if err != nil || p.MixedPort == nil || *p.MixedPort != "2081" || *p.LogLevel != "warn" || p.TestURL != nil {
		t.Fatalf("payload = %#v, %v", p, err)
	}
}

func TestStalledCandidatesAreNotSelected(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	cands := []managerconfig.Candidate{{ID: "s.fast"}, {ID: "s.mid"}, {ID: "s.slow"}}
	results := map[string]managerconfig.ProbeResult{
		"s.fast": {Health: "ok", LatencyMS: 50, Bulk: "stalled", BulkCheckedAt: now},
		"s.mid":  {Health: "ok", LatencyMS: 90, Bulk: "ok", BulkCheckedAt: now},
		"s.slow": {Health: "ok", LatencyMS: 300},
	}
	if got := selectFastest(cands, results, 2); !reflect.DeepEqual(got, []string{"s.mid", "s.slow"}) {
		t.Fatalf("selected = %v: a stalled server must be skipped despite lowest latency", got)
	}
	old := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	results["s.fast"] = managerconfig.ProbeResult{Health: "ok", LatencyMS: 50, Bulk: "stalled", BulkCheckedAt: old}
	if got := selectFastest(cands, results, 1); !reflect.DeepEqual(got, []string{"s.fast"}) {
		t.Fatalf("an expired stall verdict must not exclude a server: %v", got)
	}
}

func TestMergeProbeKeepsBulkVerdictUnlessRetested(t *testing.T) {
	prev := managerconfig.ProbeResult{Bulk: "ok", BulkKBps: 900, BulkCheckedAt: "2026-01-01T00:00:00Z"}
	got := mergeProbe(prev, health.EndpointResult{Health: "ok", LatencyMS: 80}, "2026-01-02T00:00:00Z")
	if got.Bulk != "ok" || got.BulkKBps != 900 || got.LatencyMS != 80 {
		t.Fatalf("delay-only merge lost the bulk verdict: %#v", got)
	}
	got = mergeProbe(prev, health.EndpointResult{Health: "ok", Bulk: &health.BulkResult{Bytes: 16384, Error: "stalls after 16 KB"}}, "2026-01-02T00:00:00Z")
	if got.Bulk != "stalled" || got.BulkError != "stalls after 16 KB" || got.BulkCheckedAt != "2026-01-02T00:00:00Z" {
		t.Fatalf("bulk merge = %#v", got)
	}
}
