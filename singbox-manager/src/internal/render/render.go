package render

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"

	managerconfig "github.com/openwrt-singbox/singbox-manager/internal/config"
	"github.com/openwrt-singbox/singbox-manager/internal/subscription"
)

type singBoxConfig struct {
	Log          logConfig           `json:"log"`
	DNS          *dnsConfig          `json:"dns,omitempty"`
	Inbounds     []map[string]any    `json:"inbounds"`
	Outbounds    []map[string]any    `json:"outbounds"`
	Route        routeConfig         `json:"route"`
	Experimental *experimentalConfig `json:"experimental,omitempty"`
}

// experimentalConfig enables sing-box's on-disk cache. The cache stores
// downloaded rule-sets so that, once fetched, they survive restarts and the
// daemon does not depend on reaching the network at every startup.
type experimentalConfig struct {
	CacheFile cacheFileConfig `json:"cache_file"`
	ClashAPI  *clashAPIConfig `json:"clash_api,omitempty"`
}

type cacheFileConfig struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path,omitempty"`
}

// CacheFilePath is where sing-box persists its cache (downloaded rule-sets,
// FakeIP mappings, etc.).
const CacheFilePath = "/etc/singbox-manager/cache.db"

type logConfig struct {
	Level     string `json:"level"`
	Timestamp bool   `json:"timestamp"`
}

type dnsConfig struct {
	Servers []map[string]any `json:"servers,omitempty"`
	Rules   []map[string]any `json:"rules,omitempty"`
	Final   string           `json:"final,omitempty"`
}

type clashAPIConfig struct {
	ExternalController string `json:"external_controller"`
}

type routeConfig struct {
	Rules                 []map[string]any `json:"rules,omitempty"`
	RuleSet               []map[string]any `json:"rule_set,omitempty"`
	Final                 string           `json:"final"`
	AutoDetectInterface   bool             `json:"auto_detect_interface"`
	DefaultDomainResolver string           `json:"default_domain_resolver,omitempty"`
}

// Plan is the resolved outbound graph for one config: which targets render,
// and the members of every group (needed by the fallback controller).
type Plan struct {
	Outbounds []map[string]any
	// Rendered holds every tag present in Outbounds (plus direct/block).
	Rendered map[string]bool
	// Fallback maps each fallback-mode group tag to its ordered members.
	Fallback map[string][]string
	Warnings []string
}

const testInterval = "3m"

// BuildPlan renders outbounds, subscriptions (as urltest over their selected
// candidates), and outbound groups. Anything that cannot render — a broken
// source, a subscription with nothing selected, a group left without members —
// is dropped with a warning instead of failing the whole config, and
// references to it resolve away (see Resolve).
func BuildPlan(cfg managerconfig.Config) Plan {
	plan := Plan{
		Outbounds: []map[string]any{{"type": "direct", "tag": "direct"}, {"type": "block", "tag": "block"}},
		Rendered:  map[string]bool{"direct": true, "block": true},
		Fallback:  map[string][]string{},
	}
	warn := func(format string, args ...any) { plan.Warnings = append(plan.Warnings, fmt.Sprintf(format, args...)) }
	testURL := firstNonEmptyString(cfg.Manager.TestURL, "https://www.gstatic.com/generate_204")

	for _, id := range sortedKeys(cfg.Outbounds) {
		outbound := cfg.Outbounds[id]
		if !outbound.Enabled {
			continue
		}
		item, err := subscription.ParseSource(outbound.Source)
		if err != nil {
			warn("outbound %s skipped: %v", id, err)
			continue
		}
		item.Outbound["tag"] = id
		plan.Outbounds = append(plan.Outbounds, item.Outbound)
		plan.Rendered[id] = true
	}

	for _, id := range sortedKeys(cfg.Subscriptions) {
		sub := cfg.Subscriptions[id]
		if !sub.Enabled {
			continue
		}
		selected := managerconfig.LoadSubscriptionState(id).SelectedOutbounds()
		if len(selected) == 0 {
			warn("subscription %s has no alive outbounds selected yet", id)
			continue
		}
		tags := make([]string, 0, len(selected))
		for _, outbound := range selected {
			tag := stringValue(outbound["tag"])
			plan.Outbounds = append(plan.Outbounds, outbound)
			plan.Rendered[tag] = true
			tags = append(tags, tag)
		}
		plan.Outbounds = append(plan.Outbounds, urltest(id, tags, testURL))
		plan.Rendered[id] = true
	}

	// A fallback group starts on its first member not known to be down, so a
	// (re)start does not sit on a dead member until the controller's next pass.
	probes := managerconfig.LoadProbeResults()
	firstAlive := func(members []string) string {
		for _, member := range members {
			if result, probed := probes[member]; !probed || result.Health == "ok" {
				return member
			}
		}
		return members[0]
	}

	// Groups may nest, so render them depth-first: a member group must be known
	// to render before the group that contains it is built.
	state := map[string]int{}
	var visit func(id string)
	visit = func(id string) {
		if state[id] != 0 {
			return
		}
		state[id] = 1
		group := cfg.OutboundGroups[id]
		var members []string
		for _, member := range group.Members {
			if _, isGroup := cfg.OutboundGroups[member]; isGroup {
				visit(member)
			}
			if plan.Rendered[member] {
				members = append(members, member)
			}
		}
		state[id] = 2
		if !group.Enabled {
			return
		}
		if len(members) == 0 {
			warn("outbound group %s has no available members", id)
			return
		}
		if group.Mode == "fallback" {
			plan.Outbounds = append(plan.Outbounds, map[string]any{
				"type":                        "selector",
				"tag":                         id,
				"outbounds":                   members,
				"default":                     firstAlive(members),
				"interrupt_exist_connections": true,
			})
			plan.Fallback[id] = members
		} else {
			plan.Outbounds = append(plan.Outbounds, urltest(id, members, testURL))
		}
		plan.Rendered[id] = true
	}
	for _, id := range sortedKeys(cfg.OutboundGroups) {
		visit(id)
	}
	return plan
}

func urltest(tag string, members []string, testURL string) map[string]any {
	return map[string]any{
		"type":                        "urltest",
		"tag":                         tag,
		"outbounds":                   members,
		"url":                         testURL,
		"interval":                    testInterval,
		"tolerance":                   50,
		"interrupt_exist_connections": true,
	}
}

// Final is the route.final tag: the configured final target when it renders,
// otherwise direct (fail open, so a broken proxy setup never cuts the LAN off).
func (plan Plan) Final(cfg managerconfig.Config) string {
	if cfg.Manager.RuntimeMode == "direct" {
		return "direct"
	}
	if plan.Rendered[cfg.Manager.Final] {
		return cfg.Manager.Final
	}
	return "direct"
}

func Render(cfg managerconfig.Config) ([]byte, error) {
	plan := BuildPlan(cfg)
	final := plan.Final(cfg)
	resolvers := domainResolvers{bootstrap: bootstrapResolverTag(enabledDNSServers(cfg))}
	applyOutboundDomainResolvers(plan.Outbounds, resolvers)

	experimental := &experimentalConfig{CacheFile: cacheFileConfig{Enabled: true, Path: CacheFilePath}}
	if cfg.Manager.APIListen != "" {
		experimental.ClashAPI = &clashAPIConfig{ExternalController: cfg.Manager.APIListen}
	}
	document := singBoxConfig{
		Log:          logConfig{Level: cfg.Manager.LogLevel, Timestamp: true},
		DNS:          renderDNS(cfg, plan, final),
		Inbounds:     renderInbounds(cfg),
		Outbounds:    plan.Outbounds,
		Route:        renderRoute(cfg, plan, final, resolvers),
		Experimental: experimental,
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func renderInbounds(cfg managerconfig.Config) []map[string]any {
	inbounds := []map[string]any{{
		"type":        "mixed",
		"tag":         "mixed-in",
		"listen":      cfg.Manager.MixedListen,
		"listen_port": cfg.Manager.MixedPort,
	}}
	if cfg.UsesTProxy() {
		inbounds = append(inbounds, map[string]any{
			"type":        "tproxy",
			"tag":         "tproxy-in",
			"listen":      "::",
			"listen_port": cfg.Manager.TProxyPort,
		})
	}
	// DNS aimed at the router is nat-redirected here (tproxy can't capture it
	// while dnsmasq owns the local socket); the hijack-dns route rule answers it.
	if dnsHijackEnabled(cfg) {
		inbounds = append(inbounds, map[string]any{
			"type":        "direct",
			"tag":         dnsInboundTag,
			"listen":      "::",
			"listen_port": cfg.Manager.DNSPort,
		})
	}
	if cfg.TUN.Enabled {
		addresses := []string{}
		if cfg.TUN.Inet4Address != "" {
			addresses = append(addresses, cfg.TUN.Inet4Address)
		}
		if cfg.TUN.Inet6Address != "" {
			addresses = append(addresses, cfg.TUN.Inet6Address)
		}
		inbounds = append(inbounds, map[string]any{
			"type":           "tun",
			"tag":            "tun-in",
			"interface_name": "singbox0",
			"auto_route":     cfg.TUN.AutoRoute,
			"auto_redirect":  cfg.TUN.AutoRedirect,
			"address":        addresses,
		})
	}
	return inbounds
}

// matchRule builds the match part of a route/DNS rule from rule groups: every
// rule type becomes one sub-rule and the sub-rules are ORed (a logical rule),
// because sing-box ANDs different fields inside a single default rule. Remote
// URLs become one rule_set sub-rule. It returns nil when nothing can match.
func matchRule(cfg managerconfig.Config, groupIDs []string, sets map[string]managerconfig.RuleSet) map[string]any {
	values := map[string][]string{}
	var types []string
	var ruleSets []string
	private := ""
	for _, groupID := range groupIDs {
		group, ok := cfg.RuleGroups[groupID]
		if !ok || !group.Enabled {
			continue
		}
		for _, rule := range group.Rules {
			typ, value, err := managerconfig.ParseRule(rule)
			if err != nil {
				continue
			}
			switch typ {
			case "remote":
				set := managerconfig.RemoteRuleSet(value)
				// Only cached sets are referenced: sing-box fails to start
				// when it cannot fetch a remote set, so the daemon downloads
				// it (direct, else through the tunnel) and reloads.
				if !fileExists(set.Path) {
					continue
				}
				if _, dup := sets[set.ID]; !dup {
					sets[set.ID] = set
				}
				ruleSets = appendUnique(ruleSets, set.ID)
			case "ip_is_private":
				private = value
			default:
				if _, seen := values[typ]; !seen {
					types = append(types, typ)
				}
				values[typ] = appendUnique(values[typ], value)
			}
		}
	}
	var rules []map[string]any
	for _, typ := range types {
		rules = append(rules, map[string]any{typ: ruleValues(typ, values[typ])})
	}
	if len(ruleSets) > 0 {
		rules = append(rules, map[string]any{"rule_set": ruleSets})
	}
	if private != "" {
		rules = append(rules, map[string]any{"ip_is_private": private == "true"})
	}
	switch len(rules) {
	case 0:
		return nil
	case 1:
		return rules[0]
	}
	anyRules := make([]any, len(rules))
	for i, rule := range rules {
		anyRules[i] = rule
	}
	return map[string]any{"type": "logical", "mode": "or", "rules": anyRules}
}

// ruleValues converts rule values to the JSON type sing-box expects.
func ruleValues(typ string, values []string) any {
	switch typ {
	case "port", "source_port":
		ports := make([]int, 0, len(values))
		for _, value := range values {
			port, _ := strconv.Atoi(value)
			ports = append(ports, port)
		}
		return ports
	case "ip_cidr", "source_ip_cidr":
		return normalizeSourceCIDRs(values)
	}
	return values
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func withAction(match map[string]any, outbound string) map[string]any {
	rule := map[string]any{}
	for k, v := range match {
		rule[k] = v
	}
	rule["outbound"] = outbound
	return rule
}

func renderRoute(cfg managerconfig.Config, plan Plan, final string, resolvers domainResolvers) routeConfig {
	route := routeConfig{
		Final:                 final,
		AutoDetectInterface:   true,
		DefaultDomainResolver: resolvers.bootstrap,
		// sing-box 1.13 dropped inbound-level sniffing; it is a route action.
		Rules: []map[string]any{{"action": "sniff"}},
	}
	if dnsHijackEnabled(cfg) {
		route.Rules = append(route.Rules,
			map[string]any{"inbound": []string{dnsInboundTag}, "action": "hijack-dns"},
			map[string]any{"protocol": "dns", "action": "hijack-dns"},
		)
	}

	sets := map[string]managerconfig.RuleSet{}
	if cfg.Manager.RuntimeMode == "rule" {
		for _, device := range cfg.Transparent.Devices {
			if !device.Enabled || device.Group == "" || !plan.Rendered[device.Group] {
				continue
			}
			var sources []string
			for _, ip := range []string{device.IPv4, device.IPv6} {
				if ip != "" {
					sources = append(sources, ip)
				}
			}
			if len(sources) > 0 {
				route.Rules = append(route.Rules, map[string]any{"source_ip_cidr": normalizeSourceCIDRs(sources), "outbound": device.Group})
			}
		}
		for _, entry := range cfg.Routes {
			if !entry.Enabled || !plan.Rendered[entry.Outbound] {
				continue
			}
			if match := matchRule(cfg, []string{entry.RuleGroup}, sets); match != nil {
				route.Rules = append(route.Rules, withAction(match, entry.Outbound))
			}
		}
	}
	// DNS rules render in every mode, so their rule-sets must be defined too.
	for _, rule := range cfg.EnabledDNSRules() {
		matchRule(cfg, rule.RuleGroups, sets)
	}
	for _, id := range sortedKeys(sets) {
		route.RuleSet = append(route.RuleSet, renderRuleSet(sets[id]))
	}
	return route
}

func renderRuleSet(set managerconfig.RuleSet) map[string]any {
	return map[string]any{"type": "local", "tag": set.ID, "format": set.Format, "path": set.Path}
}

func renderDNS(cfg managerconfig.Config, plan Plan, final string) *dnsConfig {
	servers := enabledDNSServers(cfg)
	if len(servers) == 0 {
		return nil
	}
	dns := &dnsConfig{}
	for _, server := range servers {
		entry := renderDNSServer(server)
		detour := server.Detour
		if detour == "proxy" {
			detour = final
		}
		// A detour to the plain direct outbound is the default (and deprecated
		// as explicit config); a detour to a target that did not render falls
		// back to direct as well.
		if detour == "" || detour == "direct" || !plan.Rendered[detour] {
			delete(entry, "detour")
		} else {
			entry["detour"] = detour
		}
		dns.Servers = append(dns.Servers, entry)
	}
	// Guarantee a proxy-free resolver for bootstrapping outbound server
	// hostnames. Without one, resolving a proxy node's own server would route
	// through that very node — a loop sing-box cannot break.
	if !hasDirectResolver(servers) {
		dns.Servers = append(dns.Servers, map[string]any{"type": "local", "tag": syntheticBootstrapTag})
	}

	sets := map[string]managerconfig.RuleSet{}
	for _, rule := range cfg.EnabledDNSRules() {
		if server, ok := cfg.DNSServers[rule.Server]; !ok || !server.Enabled {
			continue
		}
		var parts []map[string]any
		if sources := normalizeSourceCIDRs(rule.Sources); len(sources) > 0 {
			parts = append(parts, map[string]any{"source_ip_cidr": sources})
		}
		if match := matchRule(cfg, rule.RuleGroups, sets); match != nil {
			parts = append(parts, match)
		}
		var entry map[string]any
		switch len(parts) {
		case 0:
			continue
		case 1:
			entry = withAction(parts[0], "")
		default:
			entry = map[string]any{"type": "logical", "mode": "and", "rules": []any{parts[0], parts[1]}}
		}
		delete(entry, "outbound")
		entry["server"] = rule.Server
		dns.Rules = append(dns.Rules, entry)
	}

	dns.Final = servers[0].ID
	if server, ok := cfg.DNSServers[cfg.Manager.DNSFinal]; ok && server.Enabled {
		dns.Final = server.ID
	}
	return dns
}

type domainResolvers struct {
	// bootstrap is the DNS server used to resolve outbound server hostnames and
	// to back route.default_domain_resolver. It must never route through the
	// proxy: resolving a proxy node's own server through that same node is an
	// unbreakable loop (see bootstrapResolverTag / hasDirectResolver).
	bootstrap string
}

// syntheticBootstrapTag is the tag of the local resolver renderDNS injects when
// the config defines no proxy-free server to bootstrap outbound hostnames.
const syntheticBootstrapTag = "domain-bootstrap"

// ProbeAPI describes the Clash API controller embedded in a probe config so an
// external caller can delay-test each outbound through it.
type ProbeAPI struct {
	Listen string
	Secret string
}

// BuildProbeConfig renders a minimal sing-box config whose sole purpose is
// delay-testing the given outbounds (keyed by tag) through the Clash API's
// /proxies/{tag}/delay — the real end-to-end path (TLS/transport/auth).
func BuildProbeConfig(outbounds map[string]map[string]any, api ProbeAPI) ([]byte, error) {
	list := []map[string]any{{"type": "direct", "tag": "direct"}}
	for _, tag := range sortedKeys(outbounds) {
		outbound := map[string]any{}
		for k, v := range outbounds[tag] {
			outbound[k] = v
		}
		outbound["tag"] = tag
		list = append(list, outbound)
	}
	document := map[string]any{
		"log":       map[string]any{"level": "error", "timestamp": true},
		"outbounds": list,
		"experimental": map[string]any{
			"clash_api": map[string]any{"external_controller": api.Listen, "secret": api.Secret},
		},
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func enabledDNSServers(cfg managerconfig.Config) []managerconfig.DNSServer {
	servers := make([]managerconfig.DNSServer, 0, len(cfg.DNSServers))
	for _, server := range cfg.DNSServers {
		if server.Enabled {
			servers = append(servers, server)
		}
	}
	for i := 1; i < len(servers); i++ {
		for j := i; j > 0 && servers[j-1].ID > servers[j].ID; j-- {
			servers[j-1], servers[j] = servers[j], servers[j-1]
		}
	}
	return servers
}

// hasDirectResolver reports whether any enabled server resolves without routing
// through the proxy (detour empty or "direct"), making it safe for bootstrapping
// outbound server hostnames.
func hasDirectResolver(servers []managerconfig.DNSServer) bool {
	for _, server := range servers {
		if server.Detour == "" || server.Detour == "direct" {
			return true
		}
	}
	return false
}

// defaultResolver picks the group's preferred resolver, falling back to the
// first enabled server.
func bootstrapResolverTag(servers []managerconfig.DNSServer) string {
	var fallback string
	for _, server := range servers {
		if server.Detour != "" && server.Detour != "direct" {
			continue
		}
		switch normalizeDNSType(server.Type) {
		case "udp", "tcp":
			return server.ID
		}
		if fallback == "" {
			fallback = server.ID
		}
	}
	if fallback != "" {
		return fallback
	}
	return syntheticBootstrapTag
}

// enabledRuleSetIDs filters a list of rule-set references to those that exist
// and are enabled, preserving order.
func applyOutboundDomainResolvers(outbounds []map[string]any, resolvers domainResolvers) {
	if resolvers.bootstrap == "" {
		return
	}
	for _, outbound := range outbounds {
		if !supportsDomainResolver(outbound) {
			continue
		}
		outbound["domain_resolver"] = resolvers.bootstrap
	}
}

func supportsDomainResolver(outbound map[string]any) bool {
	switch stringValue(outbound["type"]) {
	case "selector", "urltest", "block", "direct":
		return false
	default:
		return true
	}
}

func renderDNSServer(server managerconfig.DNSServer) map[string]any {
	dnsType := normalizeDNSType(server.Type)
	host, port, path := parseDNSAddress(server.Address, dnsType)
	entry := map[string]any{
		"type": dnsType,
		"tag":  server.ID,
	}
	if host != "" {
		entry["server"] = host
	}
	if port > 0 {
		entry["server_port"] = port
	}
	if path != "" && dnsType == "https" {
		entry["path"] = path
	}
	if dnsType == "https" || dnsType == "tls" || dnsType == "quic" {
		entry["tls"] = map[string]any{"enabled": true}
	}
	if server.Detour != "" {
		entry["detour"] = server.Detour
	}
	return entry
}

func normalizeDNSType(value string) string {
	switch value {
	case "doh", "https":
		return "https"
	case "dot", "tls":
		return "tls"
	case "doq", "quic":
		return "quic"
	case "tcp":
		return "tcp"
	default:
		return "udp"
	}
}

func parseDNSAddress(address string, dnsType string) (string, int, string) {
	if address == "" {
		return "", 0, ""
	}
	parsed, err := url.Parse(address)
	if err == nil && parsed.Scheme != "" {
		host := parsed.Hostname()
		port := parseOptionalPort(parsed.Port())
		if host == "" && parsed.Opaque != "" {
			host, port = splitDNSHostPort(parsed.Opaque)
		}
		return host, valueOrDefaultPort(port, defaultDNSPort(dnsType)), parsed.EscapedPath()
	}
	host, port := splitDNSHostPort(address)
	return host, valueOrDefaultPort(port, defaultDNSPort(dnsType)), ""
}

func splitDNSHostPort(address string) (string, int) {
	host, portText, err := net.SplitHostPort(address)
	if err == nil {
		return host, parseOptionalPort(portText)
	}
	if strings.Count(address, ":") == 1 {
		host, portText, err := net.SplitHostPort(address)
		if err == nil {
			return host, parseOptionalPort(portText)
		}
	}
	return address, 0
}

func parseOptionalPort(value string) int {
	if value == "" {
		return 0
	}
	port, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return port
}

func valueOrDefaultPort(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func defaultDNSPort(dnsType string) int {
	switch dnsType {
	case "https":
		return 443
	case "tls", "quic":
		return 853
	default:
		return 53
	}
}

func normalizeSourceCIDRs(sources []string) []string {
	cidrs := make([]string, 0, len(sources))
	for _, source := range sources {
		source = strings.TrimSpace(source)
		if source == "" {
			continue
		}
		if strings.Contains(source, "/") {
			cidrs = append(cidrs, source)
			continue
		}
		if ip := net.ParseIP(source); ip != nil && ip.To4() != nil {
			cidrs = append(cidrs, source+"/32")
			continue
		}
		cidrs = append(cidrs, source+"/128")
	}
	return cidrs
}

// dnsInboundTag is the direct inbound the firewall redirects router-bound DNS to.
const dnsInboundTag = "dns-in"

func dnsHijackEnabled(cfg managerconfig.Config) bool {
	return cfg.Transparent.Active() && cfg.Transparent.DNSHijack
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func stringValue(value any) string {
	if str, ok := value.(string); ok {
		return str
	}
	return ""
}
