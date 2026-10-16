package config

import (
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strings"
)

type ValidationError struct {
	Errors []string
}

func (err ValidationError) Error() string {
	return strings.Join(err.Errors, "; ")
}

func ErrorStrings(err error) []string {
	if err == nil {
		return nil
	}
	if validation, ok := err.(ValidationError); ok {
		return validation.Errors
	}
	return []string{err.Error()}
}

func Validate(cfg Config) error {
	var errors []string

	if cfg.Manager.SingBoxBinary == "" {
		errors = append(errors, "manager.main.sing_box_bin is required")
	}
	if cfg.Manager.SocketPath == "" {
		errors = append(errors, "manager.main.socket_path is required")
	}
	if !inSet(cfg.Manager.RuntimeMode, "direct", "rule", "global") {
		errors = append(errors, fmt.Sprintf("manager.main.runtime_mode must be direct, rule, or global, got %q", cfg.Manager.RuntimeMode))
	}
	if !inSet(cfg.Manager.LogLevel, "trace", "debug", "info", "warn", "error", "fatal", "panic") {
		errors = append(errors, fmt.Sprintf("manager.main.log_level is unsupported: %q", cfg.Manager.LogLevel))
	}
	if cfg.Manager.Final != "" && cfg.Kind(cfg.Manager.Final) == "" {
		errors = append(errors, fmt.Sprintf("manager.main.final references missing target %q", cfg.Manager.Final))
	}
	if cfg.Manager.DNSFinal != "" {
		if _, ok := cfg.DNSServers[cfg.Manager.DNSFinal]; !ok {
			errors = append(errors, fmt.Sprintf("manager.main.dns_final references missing dns_server %q", cfg.Manager.DNSFinal))
		}
	}

	for id, outbound := range cfg.Outbounds {
		if outbound.Enabled && strings.TrimSpace(outbound.Source) == "" {
			errors = append(errors, fmt.Sprintf("outbound.%s.source is required when enabled", id))
		}
	}

	for id, subscription := range cfg.Subscriptions {
		if subscription.Enabled && subscription.URL == "" {
			errors = append(errors, fmt.Sprintf("subscription.%s.url is required when enabled", id))
		}
		if !inSet(subscription.Format, "auto", "base64", "plain") {
			errors = append(errors, fmt.Sprintf("subscription.%s.format is unsupported: %q", id, subscription.Format))
		}
		if err := CompileFilter(subscription.Filter); err != nil {
			errors = append(errors, fmt.Sprintf("subscription.%s.filter: %v", id, err))
		}
	}

	for id, group := range cfg.OutboundGroups {
		if !inSet(group.Mode, "urltest", "fallback") {
			errors = append(errors, fmt.Sprintf("outbound_group.%s.mode must be urltest or fallback, got %q", id, group.Mode))
		}
		for _, member := range group.Members {
			if cfg.Kind(member) == "" {
				errors = append(errors, fmt.Sprintf("outbound_group.%s.member references missing %q", id, member))
			}
		}
	}
	if cycle := groupCycle(cfg); cycle != "" {
		errors = append(errors, fmt.Sprintf("outbound groups form a cycle: %s", cycle))
	}

	for id, group := range cfg.RuleGroups {
		for _, rule := range group.Rules {
			if _, _, err := ParseRule(rule); err != nil {
				errors = append(errors, fmt.Sprintf("rule_group.%s: %v", id, err))
			}
		}
	}

	for _, route := range cfg.Routes {
		if _, ok := cfg.RuleGroups[route.RuleGroup]; !ok {
			errors = append(errors, fmt.Sprintf("route.%s.rule_group references missing %q", route.ID, route.RuleGroup))
		}
		if cfg.Kind(route.Outbound) == "" {
			errors = append(errors, fmt.Sprintf("route.%s.outbound references missing %q", route.ID, route.Outbound))
		}
	}

	for id, server := range cfg.DNSServers {
		if server.Enabled && server.Address == "" {
			errors = append(errors, fmt.Sprintf("dns_server.%s.address is required when enabled", id))
		}
		if !inSet(server.Type, "udp", "tcp", "tls", "dot", "doh", "doq", "https", "quic") {
			errors = append(errors, fmt.Sprintf("dns_server.%s.type is unsupported: %q", id, server.Type))
		}
		// "proxy" is shorthand for the final target.
		if server.Detour != "" && server.Detour != "proxy" && (server.Detour == "block" || cfg.Kind(server.Detour) == "") {
			errors = append(errors, fmt.Sprintf("dns_server.%s.detour references missing %q", id, server.Detour))
		}
	}

	for id, rule := range cfg.DNSRules {
		if rule.Server == "" {
			errors = append(errors, fmt.Sprintf("dns_rule.%s.server is required", id))
		} else if _, ok := cfg.DNSServers[rule.Server]; !ok {
			errors = append(errors, fmt.Sprintf("dns_rule.%s.server references missing dns_server %q", id, rule.Server))
		}
		if len(rule.Sources) == 0 && len(rule.RuleGroups) == 0 {
			errors = append(errors, fmt.Sprintf("dns_rule.%s needs at least one source_ip or rule_group", id))
		}
		validateSourceCIDRs(&errors, "dns_rule", id, rule.Sources)
		for _, group := range rule.RuleGroups {
			if _, ok := cfg.RuleGroups[group]; !ok {
				errors = append(errors, fmt.Sprintf("dns_rule.%s.rule_group references missing %q", id, group))
			}
		}
	}

	validateTransparent(&errors, cfg)
	if cfg.TUN.Inet4Address != "" {
		prefix, err := netip.ParsePrefix(cfg.TUN.Inet4Address)
		if err != nil || !prefix.Addr().Is4() {
			errors = append(errors, fmt.Sprintf("tun.main.inet4_address is invalid: %q", cfg.TUN.Inet4Address))
		}
	}
	if cfg.TUN.Inet6Address != "" {
		prefix, err := netip.ParsePrefix(cfg.TUN.Inet6Address)
		if err != nil || !prefix.Addr().Is6() {
			errors = append(errors, fmt.Sprintf("tun.main.inet6_address is invalid: %q", cfg.TUN.Inet6Address))
		}
	}

	if len(errors) > 0 {
		return ValidationError{Errors: errors}
	}
	return nil
}

func validateTransparent(errors *[]string, cfg Config) {
	tp := cfg.Transparent
	if !inSet(tp.DefaultMode, "off", "tproxy") {
		*errors = append(*errors, fmt.Sprintf("transparent.main.default_mode must be off or tproxy, got %q", tp.DefaultMode))
	}

	seen := map[string]bool{}
	for _, device := range tp.Devices {
		if device.ID != "" {
			if seen[device.ID] {
				*errors = append(*errors, fmt.Sprintf("proxy_device.%s is duplicated", device.ID))
			}
			seen[device.ID] = true
		}
		if !inSet(device.Mode, "default", "tproxy", "bypass") {
			*errors = append(*errors, fmt.Sprintf("proxy_device.%s.mode must be default, tproxy, or bypass, got %q", device.ID, device.Mode))
		}
		if !device.Enabled {
			continue
		}
		if device.MAC == "" && device.IPv4 == "" && device.IPv6 == "" {
			*errors = append(*errors, fmt.Sprintf("proxy_device.%s needs at least one of mac, ipv4, or ipv6", device.ID))
		}
		if device.MAC != "" {
			if _, err := net.ParseMAC(device.MAC); err != nil {
				*errors = append(*errors, fmt.Sprintf("proxy_device.%s.mac is invalid: %q", device.ID, device.MAC))
			}
		}
		if device.IPv4 != "" {
			if addr, err := netip.ParseAddr(device.IPv4); err != nil || !addr.Is4() {
				*errors = append(*errors, fmt.Sprintf("proxy_device.%s.ipv4 is invalid: %q", device.ID, device.IPv4))
			}
		}
		if device.IPv6 != "" {
			if addr, err := netip.ParseAddr(device.IPv6); err != nil || !addr.Is6() {
				*errors = append(*errors, fmt.Sprintf("proxy_device.%s.ipv6 is invalid: %q", device.ID, device.IPv6))
			}
		}
		if device.Group != "" && cfg.Kind(device.Group) == "" {
			*errors = append(*errors, fmt.Sprintf("proxy_device.%s.group references missing target %q", device.ID, device.Group))
		}
	}

	if !tp.Active() {
		return
	}
	if cfg.TUN.Enabled {
		*errors = append(*errors, "transparent proxying and tun.main cannot both be enabled")
	}
	if len(tp.LANIfnames) == 0 {
		*errors = append(*errors, "transparent.main.lan_ifname is required when proxying is enabled")
	}
	if cfg.UsesTProxy() && cfg.Manager.TProxyPort == 0 {
		*errors = append(*errors, "manager.main.tproxy_port is required when tproxy is in use")
	}
	if tp.DNSHijack && cfg.Manager.DNSPort == 0 {
		*errors = append(*errors, "manager.main.dns_port is required when DNS hijacking is enabled")
	}
	for _, subnet := range tp.BypassSubnet {
		if _, err := netip.ParsePrefix(subnet); err != nil {
			*errors = append(*errors, fmt.Sprintf("transparent.main.bypass_subnet is invalid: %q", subnet))
		}
	}
}

// groupCycle returns a description of the first outbound-group cycle, or "".
func groupCycle(cfg Config) string {
	state := map[string]int{} // 0 unvisited, 1 on stack, 2 done
	var path []string
	var visit func(id string) string
	visit = func(id string) string {
		switch state[id] {
		case 1:
			return strings.Join(append(path, id), " -> ")
		case 2:
			return ""
		}
		state[id] = 1
		path = append(path, id)
		for _, member := range cfg.OutboundGroups[id].Members {
			if _, isGroup := cfg.OutboundGroups[member]; isGroup {
				if cycle := visit(member); cycle != "" {
					return cycle
				}
			}
		}
		path = path[:len(path)-1]
		state[id] = 2
		return ""
	}
	ids := make([]string, 0, len(cfg.OutboundGroups))
	for id := range cfg.OutboundGroups {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if cycle := visit(id); cycle != "" {
			return cycle
		}
	}
	return ""
}

func validateSourceCIDRs(errors *[]string, kind string, id string, sources []string) {
	for _, source := range sources {
		if _, err := netip.ParseAddr(source); err == nil {
			continue
		}
		if _, err := netip.ParsePrefix(source); err != nil {
			*errors = append(*errors, fmt.Sprintf("%s.%s.source_ip is invalid: %q", kind, id, source))
		}
	}
}

func inSet(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
