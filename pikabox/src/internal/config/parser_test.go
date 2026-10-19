package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const sampleConfig = `
config manager 'main'
	option enabled '1'
	option final 'us_out'
	option dns_final 'udp'
	option probe_interval '5m'
	option test_url 'https://cp.cloudflare.com/'

config outbound 'vp1'
	option name 'NL vp1'
	option source 'trojan://secret@example.org:443#T'

config subscription 'sub1'
	option name 'Sub 1'
	option url 'https://example.com/sub'
	option filter 'type == "vless"'
	option count '3'

config outbound_group 'us_out'
	option name 'us-out'
	option mode 'fallback'
	list member 'vp1'
	list member 'sub1'
	list member 'direct'

config rule_group 'tg'
	option name 'Telegram'
	list rule 'domain_suffix:telegram.org'
	list rule 'ip_cidr:91.108.4.0/22'
	list rule 'remote:https://example.com/geosite-telegram.srs'

config rule_group 'ru'
	option name 'RU'
	list rule 'domain_suffix:ru'

config route 'route_1'
	option rule_group 'tg'
	option outbound 'us_out'

config route 'route_2'
	option rule_group 'ru'
	option outbound 'direct'

config dns_server 'udp'
	option type 'udp'
	option address '1.1.1.1'

config dns_rule 'tg_dns'
	list rule_group 'tg'
	option server 'udp'
`

func writeConfig(t *testing.T, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pikabox")
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func mustLoad(t *testing.T, path string) *Config {
	t.Helper()
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return cfg
}

func TestLoadParsesNewModel(t *testing.T) {
	cfg := mustLoad(t, writeConfig(t, sampleConfig))

	if cfg.Manager.Final != "us_out" || cfg.Manager.DNSFinal != "udp" || cfg.Manager.ProbeInterval != "5m" || cfg.Manager.TestURL != "https://cp.cloudflare.com/" {
		t.Fatalf("manager = %#v", cfg.Manager)
	}
	if ob := cfg.Outbounds["vp1"]; !ob.Enabled || ob.Name != "NL vp1" || !strings.HasPrefix(ob.Source, "trojan://") {
		t.Fatalf("outbound = %#v", ob)
	}
	if sub := cfg.Subscriptions["sub1"]; sub.Count != 3 || sub.Filter != `type == "vless"` || sub.Format != "auto" || sub.UpdateInterval != "24h" {
		t.Fatalf("subscription = %#v", sub)
	}
	if g := cfg.OutboundGroups["us_out"]; g.Mode != "fallback" || !reflect.DeepEqual(g.Members, []string{"vp1", "sub1", "direct"}) {
		t.Fatalf("group = %#v", g)
	}
	if rg := cfg.RuleGroups["tg"]; len(rg.Rules) != 3 || rg.Name != "Telegram" {
		t.Fatalf("rule group = %#v", rg)
	}
	if len(cfg.Routes) != 2 || cfg.Routes[0].RuleGroup != "tg" || cfg.Routes[1].RuleGroup != "ru" || !cfg.Routes[0].Enabled {
		t.Fatalf("routes = %#v", cfg.Routes)
	}
	if r := cfg.DNSRules["tg_dns"]; !reflect.DeepEqual(r.RuleGroups, []string{"tg"}) {
		t.Fatalf("dns rule = %#v", r)
	}
	for id, want := range map[string]string{"us_out": "group", "vp1": "outbound", "sub1": "subscription", "direct": "builtin", "nope": ""} {
		if got := cfg.Kind(id); got != want {
			t.Errorf("Kind(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestLoadDefaultsGroupMode(t *testing.T) {
	cfg := mustLoad(t, writeConfig(t, "config manager 'main'\n\nconfig outbound_group 'g'\n"))
	if cfg.OutboundGroups["g"].Mode != "urltest" || !cfg.OutboundGroups["g"].Enabled || cfg.OutboundGroups["g"].Name != "g" {
		t.Fatalf("group = %#v", cfg.OutboundGroups["g"])
	}
}

func TestLoadRejectsDuplicateSectionNames(t *testing.T) {
	path := writeConfig(t, "config manager 'main'\n\nconfig outbound 'x'\n\toption source 'trojan://a@b:1'\n\nconfig outbound_group 'x'\n")
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), `section name "x" already used`) {
		t.Fatalf("err = %v, want duplicate name error", err)
	}
}

func TestLoadRejectsUnknownSectionsAndOptions(t *testing.T) {
	for _, data := range []string{
		"config manager 'main'\n\toption active_group 'home'\n",
		"config manager 'main'\n\nconfig group 'home'\n",
		"config manager 'main'\n\nconfig outbound 'x'\n\toption bogus '1'\n",
		"config manager 'main'\n\nconfig outbound\n",
	} {
		if _, err := Load(writeConfig(t, data)); err == nil {
			t.Errorf("expected error for %q", data)
		}
	}
}

func TestLoadAcceptsLegacySingletonSectionNames(t *testing.T) {
	path := writeConfig(t, `
config manager 'main'

config transparent 'transparent'
	option default_mode 'off'

config tun 'tun'
	option enabled '0'
`)
	mustLoad(t, path)
}

func TestLoadRejectsMalformedUCI(t *testing.T) {
	_, err := Load(writeConfig(t, "config manager 'main'\n\toption final 'home\n"))
	if err == nil || !strings.Contains(err.Error(), "unterminated quoted string") {
		t.Fatalf("err = %v, want unterminated quote", err)
	}
}

func TestLoadUnvalidatedReturnsInvalidRows(t *testing.T) {
	path := writeConfig(t, "config manager 'main'\n\nconfig route 'r'\n\toption rule_group 'missing'\n\toption outbound 'direct'\n")
	if _, err := Load(path); err == nil {
		t.Fatal("expected validated load to fail")
	}
	cfg, err := LoadUnvalidated(path)
	if err != nil || len(cfg.Routes) != 1 {
		t.Fatalf("unvalidated = %v, %v", cfg, err)
	}
}

func baseValid() Config {
	cfg := DefaultConfig()
	cfg.Outbounds["a"] = Outbound{ID: "a", Enabled: true, Source: "trojan://x@y:1"}
	cfg.RuleGroups["rg"] = RuleGroup{ID: "rg", Enabled: true, Rules: []string{"domain:x.com"}}
	return cfg
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Config)
		want   string // "" = valid
	}{
		{"valid", func(c *Config) {
			c.OutboundGroups["g"] = OutboundGroup{ID: "g", Enabled: true, Mode: "urltest", Members: []string{"a", "direct"}}
			c.Routes = []Route{{ID: "r", Enabled: true, RuleGroup: "rg", Outbound: "g"}}
			c.Manager.Final = "g"
		}, ""},
		{"missing member", func(c *Config) {
			c.OutboundGroups["g"] = OutboundGroup{ID: "g", Mode: "urltest", Members: []string{"ghost"}}
		}, `outbound_group.g.member references missing "ghost"`},
		{"bad mode", func(c *Config) {
			c.OutboundGroups["g"] = OutboundGroup{ID: "g", Mode: "roundrobin"}
		}, "mode must be urltest, fallback or speed"},
		{"cycle", func(c *Config) {
			c.OutboundGroups["g1"] = OutboundGroup{ID: "g1", Mode: "urltest", Members: []string{"g2"}}
			c.OutboundGroups["g2"] = OutboundGroup{ID: "g2", Mode: "urltest", Members: []string{"g1"}}
		}, "outbound groups form a cycle"},
		{"bad rule", func(c *Config) {
			c.RuleGroups["rg"] = RuleGroup{ID: "rg", Rules: []string{"geoip:ru"}}
		}, `unsupported type "geoip"`},
		{"route missing rule group", func(c *Config) {
			c.Routes = []Route{{ID: "r", RuleGroup: "nope", Outbound: "direct"}}
		}, `route.r.rule_group references missing "nope"`},
		{"route bad target", func(c *Config) {
			c.Routes = []Route{{ID: "r", RuleGroup: "rg", Outbound: "nowhere"}}
		}, `route.r.outbound references missing "nowhere"`},
		{"bad final", func(c *Config) { c.Manager.Final = "nowhere" }, "manager.main.final references missing"},
		{"bad filter", func(c *Config) {
			c.Subscriptions["s"] = Subscription{ID: "s", Format: "auto", Filter: "type ==="}
		}, "subscription.s.filter"},
		{"non-bool filter", func(c *Config) {
			c.Subscriptions["s"] = Subscription{ID: "s", Format: "auto", Filter: "port + 1"}
		}, "subscription.s.filter"},
		{"enabled outbound without source", func(c *Config) {
			c.Outbounds["b"] = Outbound{ID: "b", Enabled: true}
		}, "outbound.b.source is required"},
		{"dns detour proxy ok", func(c *Config) {
			c.DNSServers["d"] = DNSServer{ID: "d", Type: "udp", Address: "1.1.1.1", Detour: "proxy"}
		}, ""},
		{"dns detour block rejected", func(c *Config) {
			c.DNSServers["d"] = DNSServer{ID: "d", Type: "udp", Address: "1.1.1.1", Detour: "block"}
		}, "dns_server.d.detour references missing"},
		{"dns rule missing rule group", func(c *Config) {
			c.DNSServers["d"] = DNSServer{ID: "d", Type: "udp", Address: "1.1.1.1"}
			c.DNSRules["x"] = DNSRule{ID: "x", Server: "d", RuleGroups: []string{"nope"}}
		}, `dns_rule.x.rule_group references missing "nope"`},
		{"device bad target", func(c *Config) {
			c.Transparent.Devices = []Device{{ID: "tv", Enabled: true, Mode: "default", IPv4: "192.168.1.5", Group: "nowhere"}}
		}, "proxy_device.tv.group references missing target"},
	}
	for _, tc := range cases {
		cfg := baseValid()
		tc.mutate(&cfg)
		err := Validate(cfg)
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("%s: unexpected error %v", tc.name, err)
		case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
			t.Errorf("%s: err = %v, want %q", tc.name, err, tc.want)
		}
	}
}

func TestValidateRejectsTransparentAndTUNConflict(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Transparent.DefaultMode = "tproxy"
	cfg.Transparent.LANIfnames = []string{"br-lan"}
	cfg.TUN.Enabled = true
	if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), "cannot both be enabled") {
		t.Fatalf("err = %v, want mutual exclusion", err)
	}
}

func TestValidateRejectsInvalidTransparent(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Transparent.DefaultMode = "tproxy" // active, but no LAN interface
	cfg.Transparent.BypassSubnet = []string{"not-a-cidr"}
	cfg.Transparent.Devices = []Device{
		{ID: "bad", Enabled: true, Mode: "tproxy", MAC: "not-a-mac", IPv4: "::1"},
		{ID: "empty", Enabled: true, Mode: "default"},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error")
	}
	for _, want := range []string{
		"transparent.main.lan_ifname is required",
		"transparent.main.bypass_subnet is invalid",
		"proxy_device.bad.mac is invalid",
		"proxy_device.bad.ipv4 is invalid",
		"proxy_device.empty needs at least one of mac, ipv4, or ipv6",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want %q", err, want)
		}
	}
}

func TestValidateRejectsInvalidTUNAddresses(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TUN.Inet4Address = "fdfe:dcba:9876::1/126"
	cfg.TUN.Inet6Address = "172.19.0.1/30"
	err := Validate(cfg)
	for _, want := range []string{"tun.main.inet4_address is invalid", "tun.main.inet6_address is invalid"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want %q", err, want)
		}
	}
}

func TestParseRule(t *testing.T) {
	cases := []struct {
		in, typ, value string
		ok             bool
	}{
		{"domain_suffix:example.com", "domain_suffix", "example.com", true},
		{" ip_cidr : 10.0.0.0/8 ", "ip_cidr", "10.0.0.0/8", true},
		{"ip_cidr:10.0.0.1", "ip_cidr", "10.0.0.1", true},
		{"ip_cidr:nope", "", "", false},
		{"port:443", "port", "443", true},
		{"port:70000", "", "", false},
		{"ip_is_private:true", "ip_is_private", "true", true},
		{"ip_is_private:yes", "", "", false},
		{"remote:https://example.com/x.srs", "remote", "https://example.com/x.srs", true},
		{"remote:ftp://example.com/x.srs", "", "", false},
		{"domain_regex:^a:b$", "domain_regex", "^a:b$", true},
		{"geosite:ru", "", "", false},
		{"domain:", "", "", false},
		{"nocolon", "", "", false},
	}
	for _, tc := range cases {
		typ, value, err := ParseRule(tc.in)
		if (err == nil) != tc.ok || typ != tc.typ || value != tc.value {
			t.Errorf("ParseRule(%q) = %q, %q, %v", tc.in, typ, value, err)
		}
	}
}

func TestRemoteRuleSetFormatAndStableID(t *testing.T) {
	srs := RemoteRuleSet("https://example.com/geosite-ru.SRS?token=1")
	if srs.Format != "binary" || !strings.HasSuffix(srs.Path, ".srs") || !strings.HasPrefix(srs.ID, "rs-") {
		t.Fatalf("srs = %#v", srs)
	}
	src := RemoteRuleSet("https://example.com/rules.json")
	if src.Format != "source" || !strings.HasSuffix(src.Path, ".json") {
		t.Fatalf("source = %#v", src)
	}
	if again := RemoteRuleSet("https://example.com/rules.json"); again.ID != src.ID {
		t.Fatal("ID must be stable for the same URL")
	}
	if srs.ID == src.ID {
		t.Fatal("different URLs must get different IDs")
	}
}

func TestRemoteRuleSetsDedupAndSkipDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RuleGroups["a"] = RuleGroup{ID: "a", Enabled: true, Rules: []string{"remote:https://e.com/x.srs", "domain:x"}}
	cfg.RuleGroups["b"] = RuleGroup{ID: "b", Enabled: true, Rules: []string{"remote:https://e.com/x.srs"}}
	cfg.RuleGroups["c"] = RuleGroup{ID: "c", Enabled: false, Rules: []string{"remote:https://e.com/y.srs"}}
	sets := cfg.RemoteRuleSets()
	if len(sets) != 1 || sets[0].URL != "https://e.com/x.srs" {
		t.Fatalf("sets = %#v", sets)
	}
}

func TestFilter(t *testing.T) {
	reality := FilterEnv{Name: "NL Amsterdam 1", Type: "vless", Server: "nl.example.com", Port: 443,
		Outbound: map[string]any{"type": "vless", "tls": map[string]any{"enabled": true, "reality": map[string]any{"enabled": true}}}}
	plain := FilterEnv{Name: "US 2", Type: "shadowsocks", Server: "us.example.com", Port: 8388, Outbound: map[string]any{"type": "shadowsocks"}}
	cases := []struct {
		filter     string
		real, ssok bool
	}{
		{"", true, true},
		{`type == "vless"`, true, false},
		{`name matches "(?i)^nl"`, true, false},
		{`port == 443 || port == 8388`, true, true},
		{`!(name contains "US")`, true, false},
		{`server endsWith ".example.com"`, true, true},
		{`outbound.tls.reality.enabled == true`, true, false}, // missing nested field on ss → rejected, no panic
	}
	for _, tc := range cases {
		keep, err := Filter(tc.filter)
		if err != nil {
			t.Fatalf("Filter(%q): %v", tc.filter, err)
		}
		if got := keep(reality); got != tc.real {
			t.Errorf("%q on reality = %v, want %v", tc.filter, got, tc.real)
		}
		if got := keep(plain); got != tc.ssok {
			t.Errorf("%q on ss = %v, want %v", tc.filter, got, tc.ssok)
		}
	}
	if _, err := Filter("name =="); err == nil {
		t.Fatal("expected compile error")
	}
	if err := CompileFilter(`"str"`); err == nil {
		t.Fatal("non-bool filter must be rejected")
	}
}

func TestStateRoundTrip(t *testing.T) {
	StateDir, RunDir = t.TempDir(), t.TempDir()
	state := SubscriptionState{
		Candidates: []Candidate{
			{ID: "s.a", Name: "A", Outbound: map[string]any{"tag": "s.a"}},
			{ID: "s.b", Name: "B", Outbound: map[string]any{"tag": "s.b"}},
			{ID: "s.c", Name: "C", Outbound: map[string]any{"tag": "s.c"}},
		},
		Matched:  []string{"s.c", "s.a"},
		Selected: []string{"s.c", "gone"},
	}
	if err := SaveSubscriptionState("s", state); err != nil {
		t.Fatal(err)
	}
	got := LoadSubscriptionState("s")
	if m := got.MatchedCandidates(); len(m) != 2 || m[0].ID != "s.a" || m[1].ID != "s.c" {
		t.Fatalf("matched = %#v", m)
	}
	if sel := got.SelectedOutbounds(); len(sel) != 1 || sel[0]["tag"] != "s.c" {
		t.Fatalf("selected = %#v", sel)
	}
	DeleteSubscriptionState("s")
	if len(LoadSubscriptionState("s").Candidates) != 0 {
		t.Fatal("state not deleted")
	}
	if err := SaveProbeResults(map[string]ProbeResult{"a": {Health: "ok", LatencyMS: 5}}); err != nil {
		t.Fatal(err)
	}
	if LoadProbeResults()["a"].LatencyMS != 5 {
		t.Fatal("probe results not round-tripped")
	}
}
