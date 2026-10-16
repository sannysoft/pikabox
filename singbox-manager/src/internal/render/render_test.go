package render

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	managerconfig "github.com/openwrt-singbox/singbox-manager/internal/config"
)

type document struct {
	DNS       dnsConfig        `json:"dns"`
	Inbounds  []map[string]any `json:"inbounds"`
	Outbounds []map[string]any `json:"outbounds"`
	Route     struct {
		Rules                 []map[string]any `json:"rules"`
		RuleSet               []map[string]any `json:"rule_set"`
		Final                 string           `json:"final"`
		DefaultDomainResolver string           `json:"default_domain_resolver"`
	} `json:"route"`
	Experimental map[string]any `json:"experimental"`
}

func renderDoc(t *testing.T, cfg managerconfig.Config) (document, []byte) {
	t.Helper()
	data, err := Render(cfg)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	var doc document
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, data)
	}
	return doc, data
}

func (doc document) outbound(tag string) map[string]any {
	for _, outbound := range doc.Outbounds {
		if outbound["tag"] == tag {
			return outbound
		}
	}
	return nil
}

// isolate points subscription state at a temp dir so tests never read /etc.
func isolate(t *testing.T) {
	t.Helper()
	managerconfig.StateDir, managerconfig.RunDir, managerconfig.RuleSetDir = t.TempDir(), t.TempDir(), t.TempDir()
}

func select_(t *testing.T, sub string, ids ...string) {
	t.Helper()
	state := managerconfig.SubscriptionState{Selected: ids}
	for _, id := range ids {
		state.Candidates = append(state.Candidates, managerconfig.Candidate{ID: id, Outbound: map[string]any{
			"type": "trojan", "tag": id, "server": id + ".example.com", "server_port": 443, "password": "p",
		}})
		state.Matched = append(state.Matched, id)
	}
	if err := managerconfig.SaveSubscriptionState(sub, state); err != nil {
		t.Fatal(err)
	}
}

func strings_(v any) []string {
	var out []string
	for _, item := range v.([]any) {
		out = append(out, item.(string))
	}
	return out
}

func baseConfig() managerconfig.Config {
	cfg := managerconfig.DefaultConfig()
	cfg.Outbounds["vp1"] = managerconfig.Outbound{ID: "vp1", Enabled: true, Source: "vless://u@vp1.example.com:443?security=reality&pbk=PK&sid=ab&sni=www.mozilla.org&flow=xtls-rprx-vision#vp1"}
	cfg.Outbounds["js"] = managerconfig.Outbound{ID: "js", Enabled: true, Source: `{"type":"hysteria2","tag":"ignored","server":"hy.example.com","server_port":443,"password":"p"}`}
	return cfg
}

func TestBuildPlanOutboundsAndSubscriptions(t *testing.T) {
	isolate(t)
	cfg := baseConfig()
	cfg.Outbounds["broken"] = managerconfig.Outbound{ID: "broken", Enabled: true, Source: "nope://x"}
	cfg.Outbounds["off"] = managerconfig.Outbound{ID: "off", Enabled: false, Source: "trojan://a@b:1"}
	cfg.Subscriptions["sub1"] = managerconfig.Subscription{ID: "sub1", Enabled: true}
	cfg.Subscriptions["empty"] = managerconfig.Subscription{ID: "empty", Enabled: true}
	select_(t, "sub1", "sub1.aaa", "sub1.bbb")

	plan := BuildPlan(cfg)
	for _, tag := range []string{"direct", "block", "vp1", "js", "sub1", "sub1.aaa", "sub1.bbb"} {
		if !plan.Rendered[tag] {
			t.Errorf("%s not rendered", tag)
		}
	}
	for _, tag := range []string{"broken", "off", "empty"} {
		if plan.Rendered[tag] {
			t.Errorf("%s must not render", tag)
		}
	}
	joined := strings.Join(plan.Warnings, "\n")
	if !strings.Contains(joined, "outbound broken skipped") || !strings.Contains(joined, "subscription empty has no alive outbounds") {
		t.Fatalf("warnings = %v", plan.Warnings)
	}

	doc, _ := renderDoc(t, cfg)
	if ob := doc.outbound("vp1"); ob["type"] != "vless" || ob["flow"] != "xtls-rprx-vision" {
		t.Fatalf("vp1 = %#v", ob)
	}
	if ob := doc.outbound("js"); ob["type"] != "hysteria2" || ob["server"] != "hy.example.com" {
		t.Fatalf("json outbound must render with tag = id: %#v", ob)
	}
	sub := doc.outbound("sub1")
	if sub["type"] != "urltest" || !reflect.DeepEqual(strings_(sub["outbounds"]), []string{"sub1.aaa", "sub1.bbb"}) {
		t.Fatalf("subscription group = %#v", sub)
	}
}

func TestBuildPlanGroups(t *testing.T) {
	isolate(t)
	cfg := baseConfig()
	cfg.Subscriptions["sub1"] = managerconfig.Subscription{ID: "sub1", Enabled: true}
	cfg.Subscriptions["empty"] = managerconfig.Subscription{ID: "empty", Enabled: true}
	select_(t, "sub1", "sub1.aaa")
	cfg.OutboundGroups["inner"] = managerconfig.OutboundGroup{ID: "inner", Enabled: true, Mode: "urltest", Members: []string{"js", "sub1"}}
	cfg.OutboundGroups["us_out"] = managerconfig.OutboundGroup{ID: "us_out", Enabled: true, Mode: "fallback", Members: []string{"vp1", "empty", "inner", "direct"}}
	cfg.OutboundGroups["dead"] = managerconfig.OutboundGroup{ID: "dead", Enabled: true, Mode: "urltest", Members: []string{"empty"}}
	cfg.OutboundGroups["outer"] = managerconfig.OutboundGroup{ID: "outer", Enabled: true, Mode: "urltest", Members: []string{"dead", "us_out"}}
	cfg.OutboundGroups["off"] = managerconfig.OutboundGroup{ID: "off", Enabled: false, Mode: "urltest", Members: []string{"vp1"}}

	plan := BuildPlan(cfg)
	if !reflect.DeepEqual(plan.Fallback["us_out"], []string{"vp1", "inner", "direct"}) {
		t.Fatalf("fallback members = %v (empty subscription dropped, order kept)", plan.Fallback["us_out"])
	}
	if plan.Rendered["dead"] || plan.Rendered["off"] {
		t.Fatal("group without members and disabled group must not render")
	}
	doc, _ := renderDoc(t, cfg)
	us := doc.outbound("us_out")
	if us["type"] != "selector" || us["default"] != "vp1" {
		t.Fatalf("fallback group = %#v", us)
	}
	if inner := doc.outbound("inner"); inner["type"] != "urltest" || !reflect.DeepEqual(strings_(inner["outbounds"]), []string{"js", "sub1"}) {
		t.Fatalf("inner = %#v", inner)
	}
	if outer := doc.outbound("outer"); !reflect.DeepEqual(strings_(outer["outbounds"]), []string{"us_out"}) {
		t.Fatalf("outer must drop the dead nested group: %#v", outer)
	}
	if doc.outbound("dead") != nil {
		t.Fatal("dead group rendered")
	}
}

func TestFinalFallsBackToDirect(t *testing.T) {
	isolate(t)
	cfg := baseConfig()
	cases := []struct {
		final, mode, want string
	}{
		{"vp1", "rule", "vp1"},
		{"vp1", "global", "vp1"},
		{"vp1", "direct", "direct"},
		{"", "rule", "direct"},
		{"ghost", "rule", "direct"},
		{"block", "rule", "block"},
	}
	for _, tc := range cases {
		cfg.Manager.Final, cfg.Manager.RuntimeMode = tc.final, tc.mode
		if doc, _ := renderDoc(t, cfg); doc.Route.Final != tc.want {
			t.Errorf("final=%q mode=%q → %q, want %q", tc.final, tc.mode, doc.Route.Final, tc.want)
		}
	}
}

func TestRenderRoutes(t *testing.T) {
	isolate(t)
	cfg := baseConfig()
	cfg.Manager.Final = "vp1"
	cfg.RuleGroups["tg"] = managerconfig.RuleGroup{ID: "tg", Enabled: true, Rules: []string{
		"domain_suffix:telegram.org", "domain_suffix:t.me", "ip_cidr:91.108.4.0/22", "ip_cidr:149.154.167.50",
		"port:443", "remote:https://example.com/geoip-telegram.srs", "ip_is_private:false",
	}}
	cfg.RuleGroups["ru"] = managerconfig.RuleGroup{ID: "ru", Enabled: true, Rules: []string{"domain_suffix:ru", "domain_suffix:su"}}
	cfg.RuleGroups["off"] = managerconfig.RuleGroup{ID: "off", Enabled: false, Rules: []string{"domain:x.com"}}
	cfg.RuleGroups["ru_ghost"] = managerconfig.RuleGroup{ID: "ru_ghost", Enabled: true, Rules: []string{"domain_suffix:ru"}}
	cfg.RuleGroups["tg_off"] = cfg.RuleGroups["tg"]
	cfg.RuleGroups["tg_block"] = cfg.RuleGroups["tg"]
	set := managerconfig.RemoteRuleSet("https://example.com/geoip-telegram.srs")
	if err := os.WriteFile(set.Path, []byte("srs"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg.Routes = []managerconfig.Route{
		{ID: "route_1", Enabled: true, RuleGroup: "ru", Outbound: "direct"},
		{ID: "route_2", Enabled: true, RuleGroup: "tg", Outbound: "js"},
		{ID: "route_3", Enabled: true, RuleGroup: "ru_ghost", Outbound: "ghost_group"},
		{ID: "route_4", Enabled: false, RuleGroup: "tg_off", Outbound: "block"},
		{ID: "route_5", Enabled: true, RuleGroup: "off", Outbound: "block"},
		{ID: "route_6", Enabled: true, RuleGroup: "tg_block", Outbound: "block"},
	}
	doc, data := renderDoc(t, cfg)
	rules := doc.Route.Rules
	if len(rules) != 4 || rules[0]["action"] != "sniff" {
		t.Fatalf("want sniff + 3 routes, got:\n%s", data)
	}
	ru := rules[1]
	if ru["outbound"] != "direct" || !reflect.DeepEqual(strings_(ru["domain_suffix"]), []string{"ru", "su"}) || ru["type"] != nil {
		t.Fatalf("single-type rule must not be wrapped: %#v", ru)
	}
	tg := rules[2]
	if tg["type"] != "logical" || tg["mode"] != "or" || tg["outbound"] != "js" {
		t.Fatalf("multi-type rule must be logical or: %#v", tg)
	}
	sub := tg["rules"].([]any)
	if len(sub) != 5 {
		t.Fatalf("sub-rules = %#v", sub)
	}
	cidrs := strings_(sub[1].(map[string]any)["ip_cidr"])
	if !reflect.DeepEqual(cidrs, []string{"91.108.4.0/22", "149.154.167.50/32"}) {
		t.Fatalf("ip_cidr = %v", cidrs)
	}
	if ports := sub[2].(map[string]any)["port"].([]any); ports[0] != float64(443) {
		t.Fatalf("port must render as a number: %#v", ports)
	}
	if rs := strings_(sub[3].(map[string]any)["rule_set"]); !reflect.DeepEqual(rs, []string{set.ID}) {
		t.Fatalf("rule_set = %v", rs)
	}
	if sub[4].(map[string]any)["ip_is_private"] != false {
		t.Fatalf("ip_is_private = %#v", sub[4])
	}
	if rules[3]["outbound"] != "block" {
		t.Fatalf("order must be preserved: %#v", rules[3])
	}
	if len(doc.Route.RuleSet) != 1 || doc.Route.RuleSet[0]["tag"] != set.ID || doc.Route.RuleSet[0]["type"] != "local" ||
		doc.Route.RuleSet[0]["path"] != set.Path || doc.Route.RuleSet[0]["format"] != "binary" {
		t.Fatalf("rule_set = %#v", doc.Route.RuleSet)
	}

	cfg.Manager.RuntimeMode = "global"
	if doc, _ := renderDoc(t, cfg); len(doc.Route.Rules) != 1 {
		t.Fatalf("global mode must render no routes: %#v", doc.Route.Rules)
	}
}

func TestRenderSkipsUncachedRemoteRuleSet(t *testing.T) {
	isolate(t)
	cfg := baseConfig()
	cfg.RuleGroups["tg"] = managerconfig.RuleGroup{ID: "tg", Enabled: true, Rules: []string{"domain_suffix:t.me", "remote:https://e.com/x.json"}}
	cfg.Routes = []managerconfig.Route{{ID: "route_1", Enabled: true, RuleGroup: "tg", Outbound: "block"}}
	doc, data := renderDoc(t, cfg)
	if len(doc.Route.RuleSet) != 0 || doc.Route.Rules[1]["domain_suffix"] == nil || doc.Route.Rules[1]["type"] != nil {
		t.Fatalf("uncached remote set must be left out (sing-box would fail to start):\n%s", data)
	}
}

func TestRenderDevicePinRoute(t *testing.T) {
	isolate(t)
	cfg := baseConfig()
	cfg.Transparent.Devices = []managerconfig.Device{
		{ID: "tv", Enabled: true, IPv4: "192.168.1.5", IPv6: "fd00::5", Group: "js"},
		{ID: "off", Enabled: false, IPv4: "192.168.1.6", Group: "js"},
		{ID: "ghost", Enabled: true, IPv4: "192.168.1.7", Group: "nowhere"},
	}
	doc, _ := renderDoc(t, cfg)
	if len(doc.Route.Rules) != 2 {
		t.Fatalf("rules = %#v", doc.Route.Rules)
	}
	pin := doc.Route.Rules[1]
	if pin["outbound"] != "js" || !reflect.DeepEqual(strings_(pin["source_ip_cidr"]), []string{"192.168.1.5/32", "fd00::5/128"}) {
		t.Fatalf("pin = %#v", pin)
	}
}

func TestRenderDNS(t *testing.T) {
	isolate(t)
	cfg := baseConfig()
	cfg.Manager.Final = "vp1"
	cfg.Manager.DNSFinal = "remote_doh"
	cfg.DNSServers["local_udp"] = managerconfig.DNSServer{ID: "local_udp", Enabled: true, Type: "udp", Address: "223.5.5.5", Detour: "direct"}
	cfg.DNSServers["remote_doh"] = managerconfig.DNSServer{ID: "remote_doh", Enabled: true, Type: "doh", Address: "https://1.1.1.1/dns-query", Detour: "proxy"}
	cfg.DNSServers["via_js"] = managerconfig.DNSServer{ID: "via_js", Enabled: true, Type: "tls", Address: "8.8.8.8", Detour: "js"}
	cfg.DNSServers["via_ghost"] = managerconfig.DNSServer{ID: "via_ghost", Enabled: true, Type: "udp", Address: "9.9.9.9", Detour: "ghost"}
	cfg.RuleGroups["ru"] = managerconfig.RuleGroup{ID: "ru", Enabled: true, Rules: []string{"domain_suffix:ru"}}
	cfg.DNSRules["a_ru"] = managerconfig.DNSRule{ID: "a_ru", Enabled: true, RuleGroups: []string{"ru"}, Server: "local_udp"}
	cfg.DNSRules["b_dev"] = managerconfig.DNSRule{ID: "b_dev", Enabled: true, Sources: []string{"192.168.1.20"}, RuleGroups: []string{"ru"}, Server: "via_js"}

	doc, data := renderDoc(t, cfg)
	servers := map[string]map[string]any{}
	for _, server := range doc.DNS.Servers {
		servers[server["tag"].(string)] = server
	}
	if servers["remote_doh"]["detour"] != "vp1" {
		t.Fatalf(`detour "proxy" must resolve to final: %#v`, servers["remote_doh"])
	}
	if servers["via_js"]["detour"] != "js" {
		t.Fatalf("explicit detour: %#v", servers["via_js"])
	}
	for _, tag := range []string{"local_udp", "via_ghost"} {
		if _, has := servers[tag]["detour"]; has {
			t.Fatalf("%s: direct/unrendered detour must be omitted: %#v", tag, servers[tag])
		}
	}
	if doc.DNS.Final != "remote_doh" {
		t.Fatalf("dns final = %q", doc.DNS.Final)
	}
	if len(doc.DNS.Rules) != 2 || doc.DNS.Rules[0]["server"] != "local_udp" || doc.DNS.Rules[0]["domain_suffix"] == nil || doc.DNS.Rules[0]["outbound"] != nil {
		t.Fatalf("dns rules:\n%s", data)
	}
	if and := doc.DNS.Rules[1]; and["type"] != "logical" || and["mode"] != "and" || and["server"] != "via_js" {
		t.Fatalf("source+group dns rule must AND: %#v", and)
	}
	if doc.Route.DefaultDomainResolver != "local_udp" || doc.outbound("vp1")["domain_resolver"] != "local_udp" {
		t.Fatalf("proxy hostnames must bootstrap through the direct resolver:\n%s", data)
	}
	if doc.outbound("direct")["domain_resolver"] != nil {
		t.Fatal("the built-in direct outbound relies on route.default_domain_resolver")
	}
}

func TestRenderInjectsLocalBootstrapWhenNoDirectResolver(t *testing.T) {
	isolate(t)
	cfg := baseConfig()
	cfg.DNSServers["remote_doh"] = managerconfig.DNSServer{ID: "remote_doh", Enabled: true, Type: "doh", Address: "https://1.1.1.1/dns-query", Detour: "proxy"}
	doc, data := renderDoc(t, cfg)
	if doc.Route.DefaultDomainResolver != syntheticBootstrapTag {
		t.Fatalf("default domain resolver = %q", doc.Route.DefaultDomainResolver)
	}
	found := false
	for _, server := range doc.DNS.Servers {
		if server["tag"] == syntheticBootstrapTag && server["type"] == "local" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing local bootstrap:\n%s", data)
	}
}

func TestRenderInboundsHaveNoLegacySniff(t *testing.T) {
	isolate(t)
	cfg := baseConfig()
	cfg.Transparent.DefaultMode = "tproxy"
	cfg.Transparent.LANIfnames = []string{"br-lan"}
	cfg.Transparent.DNSHijack = true
	cfg.DNSServers["local_udp"] = managerconfig.DNSServer{ID: "local_udp", Enabled: true, Type: "udp", Address: "223.5.5.5"}

	doc, data := renderDoc(t, cfg)
	if strings.Contains(string(data), `"sniff": true`) {
		t.Fatalf("sing-box 1.13 rejects inbound sniff fields:\n%s", data)
	}
	var dnsIn map[string]any
	for _, inbound := range doc.Inbounds {
		if inbound["tag"] == "dns-in" {
			dnsIn = inbound
		}
	}
	if dnsIn == nil || dnsIn["type"] != "direct" || dnsIn["listen_port"] != float64(cfg.Manager.DNSPort) {
		t.Fatalf("missing dns-in inbound:\n%s", data)
	}
	rules := doc.Route.Rules
	if len(rules) < 3 || rules[0]["action"] != "sniff" || rules[1]["action"] != "hijack-dns" || rules[1]["inbound"] == nil ||
		rules[2]["action"] != "hijack-dns" || rules[2]["protocol"] != "dns" {
		t.Fatalf("want sniff then hijack-dns rules:\n%s", data)
	}

	cfg.Transparent.DNSHijack = false
	if _, data := renderDoc(t, cfg); strings.Contains(string(data), `"dns-in"`) || strings.Contains(string(data), "hijack-dns") {
		t.Fatalf("hijack disabled but DNS capture still rendered:\n%s", data)
	}
}

func TestRenderTUNInbound(t *testing.T) {
	isolate(t)
	cfg := baseConfig()
	cfg.TUN.Enabled = true
	cfg.TUN.AutoRedirect = false
	_, data := renderDoc(t, cfg)
	for _, want := range []string{`"type": "tun"`, `"interface_name": "singbox0"`, `"172.19.0.1/30"`, `"fdfe:dcba:9876::1/126"`, `"auto_route": true`, `"auto_redirect": false`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("missing %q:\n%s", want, data)
		}
	}
}

func TestRenderClashAPI(t *testing.T) {
	isolate(t)
	cfg := baseConfig()
	doc, _ := renderDoc(t, cfg)
	api, _ := doc.Experimental["clash_api"].(map[string]any)
	if api["external_controller"] != "127.0.0.1:9090" {
		t.Fatalf("clash_api = %#v", doc.Experimental)
	}
	cfg.Manager.APIListen = ""
	if doc, _ := renderDoc(t, cfg); doc.Experimental["clash_api"] != nil {
		t.Fatal("empty api_listen must disable the clash API")
	}
}

func TestBuildProbeConfig(t *testing.T) {
	data, err := BuildProbeConfig(map[string]map[string]any{
		"b": {"type": "trojan", "tag": "wrong", "server": "b"},
		"a": {"type": "trojan", "server": "a"},
	}, ProbeAPI{Listen: "127.0.0.1:1234", Secret: "s"})
	if err != nil {
		t.Fatal(err)
	}
	var doc document
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	var tags []string
	for _, ob := range doc.Outbounds {
		tags = append(tags, ob["tag"].(string))
	}
	if !reflect.DeepEqual(tags, []string{"direct", "a", "b"}) {
		t.Fatalf("tags = %v (sorted, tag forced to key)", tags)
	}
	api := doc.Experimental["clash_api"].(map[string]any)
	if api["external_controller"] != "127.0.0.1:1234" || api["secret"] != "s" {
		t.Fatalf("api = %#v", api)
	}
}

func TestRenderPackageDefaultConfig(t *testing.T) {
	isolate(t)
	cfg, err := managerconfig.Load(filepath.Clean("../../../files/etc/config/singbox-manager"))
	if err != nil {
		t.Fatalf("load package default config: %v", err)
	}
	if doc, data := renderDoc(t, *cfg); doc.Route.Final == "" || len(doc.Outbounds) < 2 {
		t.Fatalf("default config rendered oddly:\n%s", data)
	}
}

func TestRenderWithoutDNSServersDefinesBootstrap(t *testing.T) {
	isolate(t)
	cfg := baseConfig()
	cfg.DNSServers = map[string]managerconfig.DNSServer{}
	doc, data := renderDoc(t, cfg)
	if len(doc.DNS.Servers) != 1 || doc.DNS.Servers[0]["tag"] != doc.Route.DefaultDomainResolver {
		t.Fatalf("the referenced bootstrap resolver must be defined:\n%s", data)
	}
}

func TestRouteLabelsAlignWithRules(t *testing.T) {
	isolate(t)
	cfg := baseConfig()
	cfg.RuleGroups["ru"] = managerconfig.RuleGroup{ID: "ru", Enabled: true, Rules: []string{"domain_suffix:ru"}}
	cfg.RuleGroups["su"] = managerconfig.RuleGroup{ID: "su", Enabled: true, Rules: []string{"domain_suffix:su"}}
	cfg.RuleGroups["rf"] = managerconfig.RuleGroup{ID: "rf", Enabled: true, Rules: []string{"domain_suffix:xn--p1ai"}}
	cfg.Routes = []managerconfig.Route{
		{ID: "route_1", Enabled: true, RuleGroup: "ru", Outbound: "direct"},
		{ID: "route_2", Enabled: false, RuleGroup: "su", Outbound: "block"},
		{ID: "route_3", Enabled: true, RuleGroup: "rf", Outbound: "block"},
	}
	doc, _ := renderDoc(t, cfg)
	labels := RouteLabels(cfg)
	if len(labels) != len(doc.Route.Rules) {
		t.Fatalf("%d labels for %d rules", len(labels), len(doc.Route.Rules))
	}
	if labels[0].Kind != "sniff" || labels[1].ID != "route_1" || labels[2].ID != "route_3" || labels[2].Group != "rf" {
		t.Fatalf("labels = %#v", labels)
	}
}
