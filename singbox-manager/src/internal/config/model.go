package config

import "sort"

const (
	DefaultSocketPath    = "/var/run/singbox-manager/singbox-managerd.sock"
	DefaultSingBoxBinary = "/usr/bin/sing-box"
)

type Config struct {
	Manager        Manager
	Outbounds      map[string]Outbound
	Subscriptions  map[string]Subscription
	OutboundGroups map[string]OutboundGroup
	RuleGroups     map[string]RuleGroup
	// Routes is ordered: first match wins, exactly as written in UCI.
	Routes      []Route
	DNSServers  map[string]DNSServer
	DNSRules    map[string]DNSRule
	Transparent Transparent
	TUN         TUN
}

type Manager struct {
	Enabled        bool
	Paused         bool
	LogLevel       string
	RuntimeMode    string
	SingBoxBinary  string
	SocketPath     string
	APIListen      string
	MixedListen    string
	MixedPort      int
	TProxyPort     int
	DNSPort        int
	UpdateInterval string
	// Final is the route.final target: an outbound group, an outbound, or
	// direct/block. Empty means direct.
	Final string
	// DNSFinal is the DNS server used when no DNS rule matches.
	DNSFinal string
	// ProbeInterval is how often outbounds and subscription candidates are
	// delay-tested (and subscription top-N re-selected).
	ProbeInterval string
	// TestURL is fetched through each outbound by every delay test.
	TestURL string
}

// Active reports whether the manager should keep sing-box and its firewall rules
// up. Paused is a management-only hold: the daemon keeps serving the UI/RPC (so
// nodes can be imported and tested), but the data path stays down so the admin's
// own LAN is never diverted until the proxy is explicitly started again. It is
// persisted in UCI, so the supervisor honours it across reboots and respawns.
func (m Manager) Active() bool {
	return m.Enabled && !m.Paused
}

// Outbound is a single user-defined proxy. Source is either a share URI
// (vless://, trojan://, ss://, ...) or a sing-box outbound JSON object.
type Outbound struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
	Name    string `json:"name"`
	Source  string `json:"source"`
}

// Subscription is a remote list of outbounds. Every fetched outbound is run
// through Filter (an expr expression, empty = keep all), the survivors are
// delay-tested, and the Count fastest alive ones are used wherever the
// subscription is referenced (0 = all alive).
type Subscription struct {
	ID             string `json:"id"`
	Enabled        bool   `json:"enabled"`
	Name           string `json:"name"`
	URL            string `json:"url"`
	Format         string `json:"format"`
	Filter         string `json:"filter"`
	Count          int    `json:"count"`
	UpdateInterval string `json:"update_interval"`
	LastUpdate     string `json:"last_update"`
	LastError      string `json:"last_error"`
}

// OutboundGroup is a named, ordered set of members. A member is the ID of an
// outbound, a subscription (expanding to its selected outbounds, fastest
// first), or another outbound group. Mode "urltest" uses the fastest member;
// "fallback" uses the first alive member in the written order.
type OutboundGroup struct {
	ID      string   `json:"id"`
	Enabled bool     `json:"enabled"`
	Name    string   `json:"name"`
	Mode    string   `json:"mode"`
	Members []string `json:"members"`
}

// RuleGroup is a named set of match rules, ORed together. Each rule is
// "<type>:<value>", where type is a sing-box route rule field (domain,
// domain_suffix, ip_cidr, port, ...) or "remote" for a rule-set URL.
type RuleGroup struct {
	ID      string   `json:"id"`
	Enabled bool     `json:"enabled"`
	Name    string   `json:"name"`
	Rules   []string `json:"rules"`
}

// Route sends traffic matching a rule group to an outbound target (outbound
// group, outbound, direct, or block).
type Route struct {
	ID        string `json:"id"`
	Enabled   bool   `json:"enabled"`
	RuleGroup string `json:"rule_group"`
	Outbound  string `json:"outbound"`
}

// Node is the structured form of a share URI. It is a parse intermediate:
// subscriptions and outbounds convert it to sing-box outbound JSON at once.
type Node struct {
	ID        string `json:"id"`
	Enabled   bool   `json:"enabled"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Address   string `json:"address"`
	Server    string `json:"server"`
	Port      int    `json:"port"`
	UUID      string `json:"uuid"`
	Password  string `json:"password"`
	Method    string `json:"method"`
	Security  string `json:"security"`
	TLS       bool   `json:"tls"`
	Flow      string `json:"flow"`
	Transport string `json:"transport"`
	Host      string `json:"host"`
	Path      string `json:"path"`
	SNI       string `json:"sni"`
	ALPN      string `json:"alpn"`
	Insecure  bool   `json:"insecure"`
	// Reality (VLESS Reality) parameters. PublicKey/ShortID come from the share
	// link's pbk/sid; Fingerprint is the uTLS fingerprint (fp, e.g. "chrome").
	RealityPublicKey string `json:"reality_public_key"`
	RealityShortID   string `json:"reality_short_id"`
	Fingerprint      string `json:"fingerprint"`
	Congestion       string `json:"congestion"`
	UDPRelayMode     string `json:"udp_relay_mode"`
	Tag              string `json:"tag"`
}

// DNSServer is an upstream resolver definition shared across groups.
type DNSServer struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Address string `json:"address"`
	Detour  string `json:"detour"`
}

// DNSRule selects a DNS server for traffic that matches a set of source IPs
// and/or rule sets. It renders to a sing-box dns.rules entry.
type DNSRule struct {
	ID         string   `json:"id"`
	Enabled    bool     `json:"enabled"`
	Name       string   `json:"name"`
	Sources    []string `json:"sources"`
	RuleGroups []string `json:"rule_groups"`
	Server     string   `json:"server"`
}

// RuleSet is a remote rule-set derived from a rule group's "remote:<url>"
// rule. It is not a UCI section: the daemon caches it on disk under Path.
type RuleSet struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Format string `json:"format"`
	Path   string `json:"path"`
}

// Transparent configures transparent proxying for LAN clients. A global
// DefaultMode applies to every LAN client that is not listed as a device;
// each Device may override that default (including opting out via "bypass").
type Transparent struct {
	DefaultMode  string // off | tproxy
	LANIfnames   []string
	Devices      []Device
	BypassSubnet []string // destination subnets that always egress directly
	DNSHijack    bool
	KillSwitch   bool
}

// Device pins a specific LAN client to a transparent-proxy mode. It is matched
// by MAC address and/or source IP. Mode "default" inherits Transparent.DefaultMode.
type Device struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
	Name    string `json:"name"`
	MAC     string `json:"mac"`
	IPv4    string `json:"ipv4"`
	IPv6    string `json:"ipv6"`
	Mode    string `json:"mode"` // default | tproxy | bypass
	// BypassUDP sends the device's UDP traffic directly while its TCP is still
	// tproxied. Useful for gaming consoles, where proxied UDP breaks NAT type
	// and adds latency. Only meaningful when the device's effective mode is
	// tproxy; ignored otherwise.
	BypassUDP bool `json:"bypass_udp"`
	// Group optionally pins the device's traffic to an outbound target
	// (rendered as a source-IP route rule ahead of the ordered routes).
	Group string `json:"group"`
}

// Effective resolves a device's mode, expanding "default"/"" to the global
// DefaultMode. The result is one of tproxy, bypass, or off.
func (t Transparent) Effective(d Device) string {
	mode := d.Mode
	if mode == "" || mode == "default" {
		mode = t.DefaultMode
	}
	if mode == "" {
		return "off"
	}
	return mode
}

// uses reports whether the given transparent mode is in effect for the global
// default or for any enabled device.
func (t Transparent) uses(mode string) bool {
	if t.DefaultMode == mode {
		return true
	}
	for _, device := range t.Devices {
		if device.Enabled && t.Effective(device) == mode {
			return true
		}
	}
	return false
}

// Active reports whether any traffic is transparently proxied at all.
func (t Transparent) Active() bool {
	return t.uses("tproxy")
}

// UsesTProxy reports whether the tproxy inbound and policy routing are needed.
func (cfg Config) UsesTProxy() bool { return cfg.Transparent.uses("tproxy") }

// UDPBypassDevices returns the enabled devices whose UDP must egress directly
// (BypassUDP set) while their effective mode is tproxy.
func (t Transparent) UDPBypassDevices() []Device {
	var devices []Device
	for _, device := range t.Devices {
		if device.Enabled && device.BypassUDP && t.Effective(device) == "tproxy" {
			devices = append(devices, device)
		}
	}
	return devices
}

type TUN struct {
	Enabled      bool
	AutoRoute    bool
	AutoRedirect bool
	Inet4Address string
	Inet6Address string
}

func DefaultConfig() Config {
	return Config{
		Manager: Manager{
			Enabled:        false,
			LogLevel:       "info",
			RuntimeMode:    "rule",
			SingBoxBinary:  DefaultSingBoxBinary,
			SocketPath:     DefaultSocketPath,
			APIListen:      "127.0.0.1:9090",
			MixedListen:    "0.0.0.0",
			MixedPort:      2080,
			TProxyPort:     7893,
			DNSPort:        1053,
			UpdateInterval: "24h",
			ProbeInterval:  "10m",
			TestURL:        "https://www.gstatic.com/generate_204",
		},
		Outbounds:      map[string]Outbound{},
		Subscriptions:  map[string]Subscription{},
		OutboundGroups: map[string]OutboundGroup{},
		RuleGroups:     map[string]RuleGroup{},
		DNSServers:     map[string]DNSServer{},
		DNSRules:       map[string]DNSRule{},
		Transparent: Transparent{
			DefaultMode: "off",
		},
		TUN: TUN{
			Enabled:      false,
			AutoRoute:    true,
			AutoRedirect: true,
			Inet4Address: "172.19.0.1/30",
			Inet6Address: "fdfe:dcba:9876::1/126",
		},
	}
}

// EnabledDNSRules returns the enabled DNS rules ordered by ID so first-match
// precedence is deterministic.
func (cfg Config) EnabledDNSRules() []DNSRule {
	rules := make([]DNSRule, 0, len(cfg.DNSRules))
	for _, rule := range cfg.DNSRules {
		if rule.Enabled {
			rules = append(rules, rule)
		}
	}
	sortByID(rules, func(r DNSRule) string { return r.ID })
	return rules
}

// Kind reports what an outbound target ID refers to: "builtin" (direct/block),
// "group", "outbound", "subscription", or "" when unknown. UCI section names
// are unique per package, so an ID never resolves to two kinds.
func (cfg Config) Kind(id string) string {
	switch {
	case id == "direct" || id == "block":
		return "builtin"
	case hasKey(cfg.OutboundGroups, id):
		return "group"
	case hasKey(cfg.Outbounds, id):
		return "outbound"
	case hasKey(cfg.Subscriptions, id):
		return "subscription"
	}
	return ""
}

func hasKey[T any](m map[string]T, key string) bool {
	_, ok := m[key]
	return ok
}

func sortByID[T any](items []T, id func(T) string) {
	sort.SliceStable(items, func(i, j int) bool { return id(items[i]) < id(items[j]) })
}
