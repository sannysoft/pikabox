package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSetManagerEnabledAndRuntimeMode(t *testing.T) {
	path := writeConfig(t, "config manager 'main'\n\toption enabled '0'\n")
	if err := SetManagerEnabled(path, true); err != nil {
		t.Fatal(err)
	}
	if err := SetManagerRuntimeMode(path, "global"); err != nil {
		t.Fatal(err)
	}
	cfg := mustLoad(t, path)
	if !cfg.Manager.Enabled || cfg.Manager.RuntimeMode != "global" {
		t.Fatalf("manager = %#v", cfg.Manager)
	}
	if err := SetManagerRuntimeMode(path, "bogus"); err == nil {
		t.Fatal("expected invalid runtime mode error")
	}
	if err := SetManagerOption(path, "final", "nowhere"); err == nil {
		t.Fatal("expected validation to reject a missing final target")
	}
}

func TestUpsertRoundTrip(t *testing.T) {
	path := writeConfig(t, "config manager 'main'\n")
	if err := UpsertOutbound(path, Outbound{ID: "j", Enabled: true, Source: "{\n  \"type\": \"direct\"\n}"}); err != nil {
		t.Fatal(err)
	}
	if err := UpsertSubscription(path, Subscription{ID: "s", Enabled: true, URL: "https://e.com/s", Filter: ` type == "vless" `, Count: 2}); err != nil {
		t.Fatal(err)
	}
	if err := UpsertOutboundGroup(path, OutboundGroup{ID: "g", Enabled: true, Mode: "fallback", Members: []string{"s", "j"}}); err != nil {
		t.Fatal(err)
	}
	if err := UpsertRuleGroup(path, RuleGroup{ID: "rg", Enabled: true, Name: "R G", Rules: []string{"domain:a.com", "port:443"}}); err != nil {
		t.Fatal(err)
	}
	if err := MarkSubscription(path, "s", "2026-01-01T00:00:00Z", "boom\nsecond line"); err != nil {
		t.Fatal(err)
	}
	cfg := mustLoad(t, path)
	if src := cfg.Outbounds["j"].Source; src != `{"type":"direct"}` {
		t.Fatalf("JSON source must be stored compacted, got %q", src)
	}
	if s := cfg.Subscriptions["s"]; s.Filter != `type == "vless"` || s.Count != 2 || s.LastUpdate == "" || s.LastError != "boom" {
		t.Fatalf("subscription = %#v", s)
	}
	if g := cfg.OutboundGroups["g"]; !reflect.DeepEqual(g.Members, []string{"s", "j"}) || g.Mode != "fallback" {
		t.Fatalf("group = %#v", g)
	}
	if rg := cfg.RuleGroups["rg"]; rg.Name != "R G" || len(rg.Rules) != 2 {
		t.Fatalf("rule group = %#v", rg)
	}
	if err := UpsertOutbound(path, Outbound{ID: "bad", Enabled: true, Source: "{nope"}); err == nil {
		t.Fatal("invalid JSON source must be rejected")
	}
	if err := UpsertOutboundGroup(path, OutboundGroup{ID: "g2", Mode: "urltest", Members: []string{"ghost"}}); err == nil {
		t.Fatal("group with missing member must be rejected")
	}
	if err := MarkSubscription(path, "s", "", ""); err != nil {
		t.Fatal(err)
	}
	if mustLoad(t, path).Subscriptions["s"].LastError != "" {
		t.Fatal("empty message must clear last_error")
	}
}

func TestSetRoutingPreservesOrder(t *testing.T) {
	path := writeConfig(t, sampleConfig)
	routes := []Route{
		{Enabled: true, RuleGroup: "ru", Outbound: "direct"},
		{Enabled: false, RuleGroup: "tg", Outbound: "vp1"},
		{Enabled: true, RuleGroup: "tg", Outbound: "block"},
	}
	if err := SetRouting(path, "vp1", routes); err != nil {
		t.Fatal(err)
	}
	cfg := mustLoad(t, path)
	if cfg.Manager.Final != "vp1" || len(cfg.Routes) != 3 {
		t.Fatalf("final=%q routes=%#v", cfg.Manager.Final, cfg.Routes)
	}
	for i, want := range routes {
		got := cfg.Routes[i]
		if got.RuleGroup != want.RuleGroup || got.Outbound != want.Outbound || got.Enabled != want.Enabled {
			t.Fatalf("route %d = %#v, want %#v", i, got, want)
		}
	}
	if err := SetRouting(path, "", nil); err != nil {
		t.Fatal(err)
	}
	if cfg := mustLoad(t, path); cfg.Manager.Final != "" || len(cfg.Routes) != 0 {
		t.Fatalf("clearing failed: %#v", cfg.Routes)
	}
	if err := SetRouting(path, "", []Route{{Enabled: true, RuleGroup: "nope", Outbound: "direct"}}); err == nil {
		t.Fatal("route to missing rule group must be rejected")
	}
}

func TestDeleteTargetCascades(t *testing.T) {
	data := sampleConfig + `
config dns_server 'doh'
	option type 'doh'
	option address 'https://dns.example/dns-query'
	option detour 'us_out'

config proxy_device 'tv'
	option ipv4 '192.168.1.5'
	option group 'us_out'
`
	path := writeConfig(t, data)
	if err := DeleteTarget(path, "outbound", "vp1"); err != nil {
		t.Fatal(err)
	}
	cfg := mustLoad(t, path)
	if _, ok := cfg.Outbounds["vp1"]; ok {
		t.Fatal("outbound still present")
	}
	if !reflect.DeepEqual(cfg.OutboundGroups["us_out"].Members, []string{"sub1", "direct"}) {
		t.Fatalf("members = %v", cfg.OutboundGroups["us_out"].Members)
	}

	if err := DeleteTarget(path, "outbound_group", "us_out"); err != nil {
		t.Fatal(err)
	}
	cfg = mustLoad(t, path)
	if cfg.Manager.Final != "" {
		t.Fatalf("final = %q, want cleared", cfg.Manager.Final)
	}
	if len(cfg.Routes) != 1 || cfg.Routes[0].Outbound != "direct" {
		t.Fatalf("routes = %#v, want only the direct route left", cfg.Routes)
	}
	if cfg.DNSServers["doh"].Detour != "" {
		t.Fatalf("dns detour = %q, want cleared", cfg.DNSServers["doh"].Detour)
	}
	if cfg.Transparent.Devices[0].Group != "" {
		t.Fatalf("device group = %q, want cleared", cfg.Transparent.Devices[0].Group)
	}
	if err := DeleteTarget(path, "outbound", "ghost"); err == nil {
		t.Fatal("deleting a missing target must fail")
	}
}

func TestDeleteRuleGroupCascades(t *testing.T) {
	data := sampleConfig + `
config dns_rule 'mixed'
	list rule_group 'tg'
	list source_ip '192.168.1.20'
	option server 'udp'
`
	path := writeConfig(t, data)
	if err := DeleteRuleGroup(path, "tg"); err != nil {
		t.Fatal(err)
	}
	cfg := mustLoad(t, path)
	if len(cfg.Routes) != 1 || cfg.Routes[0].RuleGroup != "ru" {
		t.Fatalf("routes = %#v", cfg.Routes)
	}
	if _, ok := cfg.DNSRules["tg_dns"]; ok {
		t.Fatal("dns rule left with nothing to match must be dropped")
	}
	if r, ok := cfg.DNSRules["mixed"]; !ok || len(r.RuleGroups) != 0 || len(r.Sources) != 1 {
		t.Fatalf("mixed dns rule = %#v", r)
	}
}

func TestDeleteDNSServerCleansRulesAndFinal(t *testing.T) {
	path := writeConfig(t, sampleConfig)
	if err := DeleteDNSServer(path, "udp"); err != nil {
		t.Fatal(err)
	}
	cfg := mustLoad(t, path)
	if _, ok := cfg.DNSServers["udp"]; ok {
		t.Fatal("dns server still exists")
	}
	if _, ok := cfg.DNSRules["tg_dns"]; ok {
		t.Fatal("dependent dns rule should be removed")
	}
	if cfg.Manager.DNSFinal != "" {
		t.Fatalf("dns_final = %q, want empty", cfg.Manager.DNSFinal)
	}
}

func TestEditRejectsInvalidResultAndKeepsFile(t *testing.T) {
	path := writeConfig(t, sampleConfig)
	before, _ := os.ReadFile(path)
	if err := UpsertRuleGroup(path, RuleGroup{ID: "tg", Enabled: true, Rules: []string{"bogus:1"}}); err == nil {
		t.Fatal("expected validation error")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("failed edit must not rewrite the file")
	}
	if !strings.Contains(string(after), "telegram.org") {
		t.Fatal("sanity")
	}
}

func TestSubscriptionFetchOptionsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfg")
	if err := os.WriteFile(path, []byte("config manager 'main'\n"), 0644); err != nil {
		t.Fatal(err)
	}
	want := Subscription{ID: "imp", Enabled: true, Name: "imp", URL: "https://e.com/sub/x", Format: "auto", Count: 3, UserAgent: "Incy/1.0", HWID: true}
	if err := UpsertSubscription(path, want); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Subscriptions["imp"]; got.UserAgent != "Incy/1.0" || !got.HWID {
		t.Fatalf("user_agent/hwid lost: %#v", got)
	}
}

func TestDeviceIDIsStable(t *testing.T) {
	StateDir = t.TempDir()
	a, err := DeviceID()
	if err != nil || len(a) != 32 {
		t.Fatalf("DeviceID = %q, %v", a, err)
	}
	if b, _ := DeviceID(); b != a {
		t.Fatalf("HWID changed between calls: %q vs %q", a, b)
	}
}
