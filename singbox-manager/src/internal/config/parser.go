package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type section struct {
	typ     string
	name    string
	line    int
	options map[string]string
	lists   map[string][]string
}

func Load(path string) (*Config, error) {
	cfg, err := LoadUnvalidated(path)
	if err != nil {
		return nil, err
	}
	if err := Validate(*cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func LoadUnvalidated(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	sections, err := parseUCI(string(data))
	if err != nil {
		return nil, err
	}

	cfg := DefaultConfig()
	if err := applySections(&cfg, sections); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func parseUCI(data string) ([]section, error) {
	var sections []section
	var current *section

	for i, raw := range strings.Split(data, "\n") {
		lineNo := i + 1
		tokens, err := tokenizeUCI(raw)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		if len(tokens) == 0 {
			continue
		}

		switch tokens[0] {
		case "config":
			if len(tokens) != 2 && len(tokens) != 3 {
				return nil, fmt.Errorf("line %d: config expects type and optional name", lineNo)
			}
			name := ""
			if len(tokens) == 3 {
				name = tokens[2]
			}
			sections = append(sections, section{
				typ:     tokens[1],
				name:    name,
				line:    lineNo,
				options: map[string]string{},
				lists:   map[string][]string{},
			})
			current = &sections[len(sections)-1]
		case "option":
			if current == nil {
				return nil, fmt.Errorf("line %d: option without config section", lineNo)
			}
			if len(tokens) != 3 {
				return nil, fmt.Errorf("line %d: option expects key and value", lineNo)
			}
			if _, exists := current.options[tokens[1]]; exists {
				return nil, fmt.Errorf("line %d: duplicate option %q in %s %q", lineNo, tokens[1], current.typ, current.name)
			}
			current.options[tokens[1]] = tokens[2]
		case "list":
			if current == nil {
				return nil, fmt.Errorf("line %d: list without config section", lineNo)
			}
			if len(tokens) != 3 {
				return nil, fmt.Errorf("line %d: list expects key and value", lineNo)
			}
			current.lists[tokens[1]] = append(current.lists[tokens[1]], tokens[2])
		default:
			return nil, fmt.Errorf("line %d: unsupported UCI directive %q", lineNo, tokens[0])
		}
	}

	return sections, nil
}

func tokenizeUCI(raw string) ([]string, error) {
	var tokens []string
	var current strings.Builder
	var quote rune
	escaped := false
	inToken := false

	for _, ch := range raw {
		if escaped {
			current.WriteRune(ch)
			inToken = true
			escaped = false
			continue
		}

		if quote != 0 {
			if quote == '"' && ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				quote = 0
				continue
			}
			current.WriteRune(ch)
			inToken = true
			continue
		}

		switch {
		case ch == '#':
			if inToken {
				current.WriteRune(ch)
			}
			goto finish
		case ch == '\'' || ch == '"':
			quote = ch
			inToken = true
		case ch == ' ' || ch == '\t' || ch == '\r':
			if inToken {
				tokens = append(tokens, current.String())
				current.Reset()
				inToken = false
			}
		default:
			current.WriteRune(ch)
			inToken = true
		}
	}

finish:
	if escaped {
		return nil, fmt.Errorf("dangling escape")
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quoted string")
	}
	if inToken {
		tokens = append(tokens, current.String())
	}
	return tokens, nil
}

func applySections(cfg *Config, sections []section) error {
	seenManager := false
	// UCI section names are unique per package; outbound targets rely on it
	// (an ID resolves to exactly one outbound, subscription, or group).
	seenNames := map[string]string{}

	for _, sec := range sections {
		if sec.name != "" {
			if typ, dup := seenNames[sec.name]; dup {
				return fmt.Errorf("line %d: section name %q already used by a %s section", sec.line, sec.name, typ)
			}
			seenNames[sec.name] = sec.typ
		}
		var err error
		switch sec.typ {
		case "manager":
			if sec.name != "main" {
				return fmt.Errorf("line %d: unsupported manager section %q", sec.line, sec.name)
			}
			if seenManager {
				return fmt.Errorf("line %d: duplicate manager main section", sec.line)
			}
			seenManager = true
			err = applyManager(cfg, sec)
		case "outbound":
			var outbound Outbound
			if outbound, err = readOutbound(sec); err == nil {
				cfg.Outbounds[outbound.ID] = outbound
			}
		case "subscription":
			var subscription Subscription
			if subscription, err = readSubscription(sec); err == nil {
				cfg.Subscriptions[subscription.ID] = subscription
			}
		case "outbound_group":
			var group OutboundGroup
			if group, err = readOutboundGroup(sec); err == nil {
				cfg.OutboundGroups[group.ID] = group
			}
		case "rule_group":
			var group RuleGroup
			if group, err = readRuleGroup(sec); err == nil {
				cfg.RuleGroups[group.ID] = group
			}
		case "route":
			var route Route
			if route, err = readRoute(sec); err == nil {
				cfg.Routes = append(cfg.Routes, route)
			}
		case "dns_server":
			var server DNSServer
			if server, err = readDNSServer(sec); err == nil {
				cfg.DNSServers[server.ID] = server
			}
		case "dns_rule":
			var rule DNSRule
			if rule, err = readDNSRule(sec); err == nil {
				cfg.DNSRules[rule.ID] = rule
			}
		case "transparent":
			if sec.name != "main" && sec.name != "transparent" {
				return fmt.Errorf("line %d: unsupported transparent section %q", sec.line, sec.name)
			}
			devices := cfg.Transparent.Devices
			var transparent Transparent
			if transparent, err = readTransparent(sec); err == nil {
				transparent.Devices = devices
				cfg.Transparent = transparent
			}
		case "proxy_device":
			if sec.name == "" {
				return fmt.Errorf("line %d: proxy_device section requires a name", sec.line)
			}
			var device Device
			if device, err = readProxyDevice(sec); err == nil {
				cfg.Transparent.Devices = append(cfg.Transparent.Devices, device)
			}
		case "tun":
			if sec.name != "main" && sec.name != "tun" {
				return fmt.Errorf("line %d: unsupported tun section %q", sec.line, sec.name)
			}
			var tun TUN
			if tun, err = readTUN(sec); err == nil {
				cfg.TUN = tun
			}
		default:
			return fmt.Errorf("line %d: unsupported section type %q", sec.line, sec.typ)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func applyManager(cfg *Config, sec section) error {
	allowed := map[string]bool{
		"enabled": true, "paused": true, "log_level": true, "runtime_mode": true,
		"sing_box_bin": true, "socket_path": true, "api_listen": true,
		"mixed_listen": true, "mixed_port": true, "tproxy_port": true, "dns_port": true,
		"update_interval": true, "final": true, "dns_final": true, "probe_interval": true, "test_url": true,
	}
	if err := rejectUnknownOptions(sec, allowed); err != nil {
		return err
	}
	if err := rejectUnknownLists(sec, nil); err != nil {
		return err
	}

	var err error
	for key, value := range sec.options {
		switch key {
		case "enabled":
			cfg.Manager.Enabled, err = parseBool(sec, key, value)
		case "paused":
			cfg.Manager.Paused, err = parseBool(sec, key, value)
		case "log_level":
			cfg.Manager.LogLevel = value
		case "runtime_mode":
			cfg.Manager.RuntimeMode = value
		case "sing_box_bin":
			cfg.Manager.SingBoxBinary = value
		case "socket_path":
			cfg.Manager.SocketPath = value
		case "api_listen":
			cfg.Manager.APIListen = value
		case "mixed_listen":
			cfg.Manager.MixedListen = value
		case "mixed_port":
			cfg.Manager.MixedPort, err = parsePort(sec, key, value)
		case "tproxy_port":
			cfg.Manager.TProxyPort, err = parsePort(sec, key, value)
		case "dns_port":
			cfg.Manager.DNSPort, err = parsePort(sec, key, value)
		case "update_interval":
			cfg.Manager.UpdateInterval = value
		case "final":
			cfg.Manager.Final = value
		case "dns_final":
			cfg.Manager.DNSFinal = value
		case "probe_interval":
			cfg.Manager.ProbeInterval = value
		case "test_url":
			cfg.Manager.TestURL = value
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// readNamed checks a named section's options/lists against the allowed sets
// and returns its common enabled flag (default on) and display name.
func readNamed(sec section, options, lists map[string]bool) (bool, string, error) {
	if sec.name == "" {
		return false, "", fmt.Errorf("line %d: %s section requires a name", sec.line, sec.typ)
	}
	options["enabled"] = true
	options["name"] = true
	if err := rejectUnknownOptions(sec, options); err != nil {
		return false, "", err
	}
	if err := rejectUnknownLists(sec, lists); err != nil {
		return false, "", err
	}
	enabled := true
	if value, ok := sec.options["enabled"]; ok {
		var err error
		if enabled, err = parseBool(sec, "enabled", value); err != nil {
			return false, "", err
		}
	}
	return enabled, valueOrDefault(sec.options["name"], sec.name), nil
}

func readOutbound(sec section) (Outbound, error) {
	enabled, name, err := readNamed(sec, map[string]bool{"source": true}, nil)
	if err != nil {
		return Outbound{}, err
	}
	return Outbound{ID: sec.name, Enabled: enabled, Name: name, Source: sec.options["source"]}, nil
}

func readSubscription(sec section) (Subscription, error) {
	enabled, name, err := readNamed(sec, map[string]bool{
		"url": true, "format": true, "filter": true, "count": true,
		"update_interval": true, "last_update": true, "last_error": true,
		"user_agent": true, "hwid": true,
	}, nil)
	if err != nil {
		return Subscription{}, err
	}
	subscription := Subscription{
		ID:             sec.name,
		Enabled:        enabled,
		Name:           name,
		URL:            sec.options["url"],
		Format:         valueOrDefault(sec.options["format"], "auto"),
		Filter:         sec.options["filter"],
		UpdateInterval: valueOrDefault(sec.options["update_interval"], "24h"),
		LastUpdate:     sec.options["last_update"],
		LastError:      sec.options["last_error"],
	}
	subscription.UserAgent = sec.options["user_agent"]
	if value, ok := sec.options["hwid"]; ok {
		if subscription.HWID, err = parseBool(sec, "hwid", value); err != nil {
			return Subscription{}, err
		}
	}
	if value, ok := sec.options["count"]; ok {
		if subscription.Count, err = parseNonNegativeInt(sec, "count", value); err != nil {
			return Subscription{}, err
		}
	}
	return subscription, nil
}

func readOutboundGroup(sec section) (OutboundGroup, error) {
	enabled, name, err := readNamed(sec, map[string]bool{"mode": true}, map[string]bool{"member": true})
	if err != nil {
		return OutboundGroup{}, err
	}
	return OutboundGroup{
		ID:      sec.name,
		Enabled: enabled,
		Name:    name,
		Mode:    valueOrDefault(sec.options["mode"], "urltest"),
		Members: cleanList(sec.lists["member"]),
	}, nil
}

func readRuleGroup(sec section) (RuleGroup, error) {
	enabled, name, err := readNamed(sec, map[string]bool{}, map[string]bool{"rule": true})
	if err != nil {
		return RuleGroup{}, err
	}
	return RuleGroup{ID: sec.name, Enabled: enabled, Name: name, Rules: cleanList(sec.lists["rule"])}, nil
}

func readRoute(sec section) (Route, error) {
	enabled, _, err := readNamed(sec, map[string]bool{"rule_group": true, "outbound": true}, nil)
	if err != nil {
		return Route{}, err
	}
	return Route{ID: sec.name, Enabled: enabled, RuleGroup: sec.options["rule_group"], Outbound: sec.options["outbound"]}, nil
}

func readDNSRule(sec section) (DNSRule, error) {
	enabled, name, err := readNamed(sec, map[string]bool{"server": true},
		map[string]bool{"source_ip": true, "rule_group": true})
	if err != nil {
		return DNSRule{}, err
	}
	return DNSRule{
		ID:         sec.name,
		Enabled:    enabled,
		Name:       name,
		Sources:    cleanList(sec.lists["source_ip"]),
		RuleGroups: cleanList(sec.lists["rule_group"]),
		Server:     sec.options["server"],
	}, nil
}

func readDNSServer(sec section) (DNSServer, error) {
	if sec.name == "" {
		return DNSServer{}, fmt.Errorf("line %d: dns_server section requires a name", sec.line)
	}
	if err := rejectUnknownOptions(sec, map[string]bool{
		"enabled": true, "name": true, "type": true, "address": true, "detour": true,
	}); err != nil {
		return DNSServer{}, err
	}
	if err := rejectUnknownLists(sec, nil); err != nil {
		return DNSServer{}, err
	}

	server := DNSServer{
		ID:      sec.name,
		Enabled: true,
		Name:    valueOrDefault(sec.options["name"], sec.name),
		Type:    valueOrDefault(sec.options["type"], "udp"),
		Address: sec.options["address"],
		Detour:  sec.options["detour"],
	}
	if value, ok := sec.options["enabled"]; ok {
		enabled, err := parseBool(sec, "enabled", value)
		if err != nil {
			return DNSServer{}, err
		}
		server.Enabled = enabled
	}
	return server, nil
}

func readTransparent(sec section) (Transparent, error) {
	if err := rejectUnknownOptions(sec, map[string]bool{
		"default_mode": true, "dns_hijack": true, "kill_switch": true,
	}); err != nil {
		return Transparent{}, err
	}
	if err := rejectUnknownLists(sec, map[string]bool{
		"lan_ifname": true, "bypass_subnet": true,
	}); err != nil {
		return Transparent{}, err
	}

	transparent := Transparent{
		DefaultMode:  valueOrDefault(sec.options["default_mode"], "off"),
		LANIfnames:   cleanList(sec.lists["lan_ifname"]),
		BypassSubnet: cleanList(sec.lists["bypass_subnet"]),
	}
	var err error
	if value, ok := sec.options["dns_hijack"]; ok {
		transparent.DNSHijack, err = parseBool(sec, "dns_hijack", value)
		if err != nil {
			return Transparent{}, err
		}
	}
	if value, ok := sec.options["kill_switch"]; ok {
		transparent.KillSwitch, err = parseBool(sec, "kill_switch", value)
		if err != nil {
			return Transparent{}, err
		}
	}
	return transparent, nil
}

func readProxyDevice(sec section) (Device, error) {
	if err := rejectUnknownOptions(sec, map[string]bool{
		"enabled": true, "name": true, "mac": true, "ipv4": true, "ipv6": true,
		"mode": true, "bypass_udp": true, "group": true,
	}); err != nil {
		return Device{}, err
	}
	if err := rejectUnknownLists(sec, nil); err != nil {
		return Device{}, err
	}

	device := Device{
		ID:      sec.name,
		Enabled: true,
		Name:    valueOrDefault(sec.options["name"], sec.name),
		MAC:     sec.options["mac"],
		IPv4:    sec.options["ipv4"],
		IPv6:    sec.options["ipv6"],
		Mode:    valueOrDefault(sec.options["mode"], "default"),
		Group:   sec.options["group"],
	}
	if value, ok := sec.options["enabled"]; ok {
		enabled, err := parseBool(sec, "enabled", value)
		if err != nil {
			return Device{}, err
		}
		device.Enabled = enabled
	}
	if value, ok := sec.options["bypass_udp"]; ok {
		bypassUDP, err := parseBool(sec, "bypass_udp", value)
		if err != nil {
			return Device{}, err
		}
		device.BypassUDP = bypassUDP
	}
	return device, nil
}

func readTUN(sec section) (TUN, error) {
	if err := rejectUnknownOptions(sec, map[string]bool{
		"enabled": true, "auto_route": true, "auto_redirect": true,
		"inet4_address": true, "inet6_address": true,
	}); err != nil {
		return TUN{}, err
	}
	if err := rejectUnknownLists(sec, nil); err != nil {
		return TUN{}, err
	}

	tun := TUN{
		AutoRoute:    true,
		AutoRedirect: true,
		Inet4Address: valueOrDefault(sec.options["inet4_address"], "172.19.0.1/30"),
		Inet6Address: valueOrDefault(sec.options["inet6_address"], "fdfe:dcba:9876::1/126"),
	}
	var err error
	if value, ok := sec.options["enabled"]; ok {
		tun.Enabled, err = parseBool(sec, "enabled", value)
		if err != nil {
			return TUN{}, err
		}
	}
	if value, ok := sec.options["auto_route"]; ok {
		tun.AutoRoute, err = parseBool(sec, "auto_route", value)
		if err != nil {
			return TUN{}, err
		}
	}
	if value, ok := sec.options["auto_redirect"]; ok {
		tun.AutoRedirect, err = parseBool(sec, "auto_redirect", value)
		if err != nil {
			return TUN{}, err
		}
	}
	return tun, nil
}

func rejectUnknownOptions(sec section, allowed map[string]bool) error {
	for key := range sec.options {
		if allowed == nil || !allowed[key] {
			return fmt.Errorf("line %d: unsupported option %q in %s %q", sec.line, key, sec.typ, sec.name)
		}
	}
	return nil
}

func rejectUnknownLists(sec section, allowed map[string]bool) error {
	for key := range sec.lists {
		if allowed == nil || !allowed[key] {
			return fmt.Errorf("line %d: unsupported list %q in %s %q", sec.line, key, sec.typ, sec.name)
		}
	}
	return nil
}

func parseBool(sec section, key string, value string) (bool, error) {
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on", "enabled":
		return true, nil
	case "0", "false", "no", "off", "disabled":
		return false, nil
	default:
		return false, fmt.Errorf("line %d: option %s must be boolean, got %q", sec.line, key, value)
	}
}

func parsePort(sec section, key string, value string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("line %d: option %s must be a TCP/UDP port, got %q", sec.line, key, value)
	}
	return port, nil
}

func parseNonNegativeInt(sec section, key string, value string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("line %d: option %s must be a non-negative integer, got %q", sec.line, key, value)
	}
	return parsed, nil
}

func cleanList(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			cleaned = append(cleaned, value)
		}
	}
	return cleaned
}

func valueOrDefault(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
