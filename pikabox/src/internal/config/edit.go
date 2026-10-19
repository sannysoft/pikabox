package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// SectionID turns a display name into a UCI section name.
func SectionID(name string) string { return sectionNameFromID(name) }

// upsert replaces the section with the same type and name, or appends it.
func upsert(path string, replacement section) error {
	if replacement.name == "" {
		return fmt.Errorf("%s id is required", replacement.typ)
	}
	return editConfig(path, func(sections []section) ([]section, error) {
		for i, sec := range sections {
			if sec.typ == replacement.typ && sec.name == replacement.name {
				sections[i] = replacement
				return sections, nil
			}
		}
		return append(sections, replacement), nil
	})
}

func UpsertOutbound(path string, outbound Outbound) error {
	source := strings.TrimSpace(outbound.Source)
	if strings.HasPrefix(source, "{") {
		// UCI values are single-line: store JSON compacted.
		var object map[string]any
		if err := json.Unmarshal([]byte(source), &object); err != nil {
			return fmt.Errorf("outbound %q: invalid JSON: %w", outbound.ID, err)
		}
		compact, _ := json.Marshal(object)
		source = string(compact)
	}
	return upsert(path, section{typ: "outbound", name: outbound.ID, options: map[string]string{
		"enabled": boolString(outbound.Enabled),
		"name":    valueOrDefault(outbound.Name, outbound.ID),
		"source":  source,
	}, lists: map[string][]string{}})
}

func UpsertSubscription(path string, subscription Subscription) error {
	options := map[string]string{
		"enabled":         boolString(subscription.Enabled),
		"name":            valueOrDefault(subscription.Name, subscription.ID),
		"url":             subscription.URL,
		"format":          valueOrDefault(subscription.Format, "auto"),
		"count":           strconv.Itoa(subscription.Count),
		"update_interval": valueOrDefault(subscription.UpdateInterval, "24h"),
	}
	setOrDelete(options, "filter", strings.TrimSpace(subscription.Filter))
	setOrDelete(options, "user_agent", strings.TrimSpace(subscription.UserAgent))
	if subscription.SelectBy == "speed" {
		options["select_by"] = "speed"
	}
	if subscription.HWID {
		options["hwid"] = "1"
	}
	setOrDelete(options, "last_update", subscription.LastUpdate)
	setOrDelete(options, "last_error", subscription.LastError)
	return upsert(path, section{typ: "subscription", name: subscription.ID, options: options, lists: map[string][]string{}})
}

// MarkSubscription records the outcome of a fetch: lastUpdate on success,
// message on failure (an empty message clears a previous error).
func MarkSubscription(path string, id string, lastUpdate string, message string) error {
	return editConfig(path, func(sections []section) ([]section, error) {
		for i := range sections {
			if sections[i].typ != "subscription" || sections[i].name != id {
				continue
			}
			if lastUpdate != "" {
				sections[i].options["last_update"] = lastUpdate
			}
			if message == "" {
				delete(sections[i].options, "last_error")
			} else {
				sections[i].options["last_error"] = firstLine(message)
			}
			return sections, nil
		}
		return nil, fmt.Errorf("subscription %q not found", id)
	})
}

func UpsertOutboundGroup(path string, group OutboundGroup) error {
	lists := map[string][]string{}
	if members := cleanList(group.Members); len(members) > 0 {
		lists["member"] = members
	}
	return upsert(path, section{typ: "outbound_group", name: group.ID, options: map[string]string{
		"enabled": boolString(group.Enabled),
		"name":    valueOrDefault(group.Name, group.ID),
		"mode":    valueOrDefault(group.Mode, "urltest"),
	}, lists: lists})
}

func UpsertRuleGroup(path string, group RuleGroup) error {
	lists := map[string][]string{}
	if rules := cleanList(group.Rules); len(rules) > 0 {
		lists["rule"] = rules
	}
	return upsert(path, section{typ: "rule_group", name: group.ID, options: map[string]string{
		"enabled": boolString(group.Enabled),
		"name":    valueOrDefault(group.Name, group.ID),
	}, lists: lists})
}

// SetRouting replaces the ordered route list and the final target in one
// write. Routes are re-emitted at the end of the file in the given order,
// which is the order they render in.
func SetRouting(path string, final string, routes []Route) error {
	return editConfig(path, func(sections []section) ([]section, error) {
		kept := make([]section, 0, len(sections)+len(routes))
		foundManager := false
		for _, sec := range sections {
			if sec.typ == "route" {
				continue
			}
			if sec.typ == "manager" && sec.name == "main" {
				setOrDelete(sec.options, "final", final)
				foundManager = true
			}
			kept = append(kept, sec)
		}
		if !foundManager {
			return nil, fmt.Errorf("manager main section not found")
		}
		for i, route := range routes {
			kept = append(kept, section{typ: "route", name: fmt.Sprintf("route_%d", i+1), options: map[string]string{
				"enabled":    boolString(route.Enabled),
				"rule_group": route.RuleGroup,
				"outbound":   route.Outbound,
			}, lists: map[string][]string{}})
		}
		return kept, nil
	})
}

// SetManagerOption sets (or, for an empty value, removes) one option of the
// manager main section.
func SetManagerOption(path string, key string, value string) error {
	return editConfig(path, func(sections []section) ([]section, error) {
		for i := range sections {
			if sections[i].typ == "manager" && sections[i].name == "main" {
				setOrDelete(sections[i].options, key, value)
				return sections, nil
			}
		}
		return nil, fmt.Errorf("manager main section not found")
	})
}

// DeleteTarget removes an outbound, subscription, or outbound group and every
// reference to it: group memberships, routes that send to it, the final
// target, DNS detours, and device pins.
func DeleteTarget(path string, typ string, id string) error {
	return editConfig(path, func(sections []section) ([]section, error) {
		next := make([]section, 0, len(sections))
		found := false
		for _, sec := range sections {
			switch {
			case sec.typ == typ && sec.name == id:
				found = true
				continue
			case sec.typ == "route" && sec.options["outbound"] == id:
				continue
			case sec.typ == "outbound_group":
				removeListValue(&sec, "member", id)
			case sec.typ == "manager" && sec.options["final"] == id:
				delete(sec.options, "final")
			case sec.typ == "dns_server" && sec.options["detour"] == id:
				delete(sec.options, "detour")
			case sec.typ == "proxy_device" && sec.options["group"] == id:
				delete(sec.options, "group")
			}
			next = append(next, sec)
		}
		if !found {
			return nil, fmt.Errorf("%s %q not found", typ, id)
		}
		return next, nil
	})
}

// DeleteRuleGroup removes a rule group, the routes that use it, and its
// references from DNS rules (dropping DNS rules left with nothing to match).
func DeleteRuleGroup(path string, id string) error {
	return editConfig(path, func(sections []section) ([]section, error) {
		next := make([]section, 0, len(sections))
		found := false
		for _, sec := range sections {
			switch {
			case sec.typ == "rule_group" && sec.name == id:
				found = true
				continue
			case sec.typ == "route" && sec.options["rule_group"] == id:
				continue
			case sec.typ == "dns_rule":
				removeListValue(&sec, "rule_group", id)
				if len(sec.lists["rule_group"]) == 0 && len(sec.lists["source_ip"]) == 0 {
					continue
				}
			}
			next = append(next, sec)
		}
		if !found {
			return nil, fmt.Errorf("rule group %q not found", id)
		}
		return next, nil
	})
}

func DeleteDNSServer(path string, serverID string) error {
	if serverID == "" {
		return fmt.Errorf("dns server id is required")
	}
	return editConfig(path, func(sections []section) ([]section, error) {
		next := make([]section, 0, len(sections))
		found := false
		for _, sec := range sections {
			switch {
			case sec.typ == "dns_server" && sec.name == serverID:
				found = true
				continue
			case sec.typ == "dns_rule" && sec.options["server"] == serverID:
				continue
			case sec.typ == "manager" && sec.options["dns_final"] == serverID:
				delete(sec.options, "dns_final")
			}
			next = append(next, sec)
		}
		if !found {
			return nil, fmt.Errorf("dns server %q not found", serverID)
		}
		return next, nil
	})
}

func dnsRuleSection(rule DNSRule) section {
	options := map[string]string{
		"enabled": boolString(rule.Enabled),
		"name":    valueOrDefault(rule.Name, rule.ID),
	}
	setOrDelete(options, "server", rule.Server)
	lists := map[string][]string{}
	if sources := cleanList(rule.Sources); len(sources) > 0 {
		lists["source_ip"] = sources
	}
	if groups := cleanList(rule.RuleGroups); len(groups) > 0 {
		lists["rule_group"] = groups
	}
	return section{
		typ:     "dns_rule",
		name:    rule.ID,
		options: options,
		lists:   lists,
	}
}

func SetManagerEnabled(path string, enabled bool) error {
	return editConfig(path, func(sections []section) ([]section, error) {
		for i := range sections {
			if sections[i].typ != "manager" || sections[i].name != "main" {
				continue
			}
			if sections[i].options == nil {
				sections[i].options = map[string]string{}
			}
			sections[i].options["enabled"] = boolString(enabled)
			return sections, nil
		}
		return nil, fmt.Errorf("manager main section not found")
	})
}

func SetManagerPaused(path string, paused bool) error {
	return editConfig(path, func(sections []section) ([]section, error) {
		for i := range sections {
			if sections[i].typ != "manager" || sections[i].name != "main" {
				continue
			}
			if sections[i].options == nil {
				sections[i].options = map[string]string{}
			}
			sections[i].options["paused"] = boolString(paused)
			return sections, nil
		}
		return nil, fmt.Errorf("manager main section not found")
	})
}

func SetManagerRuntimeMode(path string, mode string) error {
	if !inSet(mode, "direct", "rule", "global") {
		return fmt.Errorf("runtime mode must be direct, rule, or global, got %q", mode)
	}
	return editConfig(path, func(sections []section) ([]section, error) {
		for i := range sections {
			if sections[i].typ != "manager" || sections[i].name != "main" {
				continue
			}
			if sections[i].options == nil {
				sections[i].options = map[string]string{}
			}
			sections[i].options["runtime_mode"] = mode
			return sections, nil
		}
		return nil, fmt.Errorf("manager main section not found")
	})
}

func UpsertDNSRule(path string, rule DNSRule) error {
	if rule.ID == "" {
		return fmt.Errorf("dns rule id is required")
	}
	if rule.Name == "" {
		rule.Name = rule.ID
	}
	return editConfig(path, func(sections []section) ([]section, error) {
		replacement := dnsRuleSection(rule)
		for i, sec := range sections {
			if sec.typ == "dns_rule" && sec.name == rule.ID {
				sections[i] = replacement
				return sections, nil
			}
		}
		return append(sections, replacement), nil
	})
}

func DeleteDNSRule(path string, ruleID string) error {
	if ruleID == "" {
		return fmt.Errorf("dns rule id is required")
	}
	return editConfig(path, func(sections []section) ([]section, error) {
		next := make([]section, 0, len(sections))
		found := false
		for _, sec := range sections {
			if sec.typ == "dns_rule" && sec.name == ruleID {
				found = true
				continue
			}
			next = append(next, sec)
		}
		if !found {
			return nil, fmt.Errorf("dns rule %q not found", ruleID)
		}
		return next, nil
	})
}

func UpsertDNSServer(path string, server DNSServer) error {
	if server.ID == "" {
		return fmt.Errorf("dns server id is required")
	}
	if server.Name == "" {
		server.Name = server.ID
	}
	if server.Type == "" {
		server.Type = "udp"
	}
	return editConfig(path, func(sections []section) ([]section, error) {
		replacement := dnsServerSection(server)
		for i, sec := range sections {
			if sec.typ == "dns_server" && sec.name == server.ID {
				sections[i] = replacement
				return sections, nil
			}
		}
		return append(sections, replacement), nil
	})
}

func UpsertTransparent(path string, transparent Transparent) error {
	return editConfig(path, func(sections []section) ([]section, error) {
		kept := make([]section, 0, len(sections))
		for _, sec := range sections {
			if sec.typ == "transparent" || sec.typ == "proxy_device" {
				continue
			}
			kept = append(kept, sec)
		}
		kept = append(kept, transparentSection(transparent))
		for _, device := range transparent.Devices {
			kept = append(kept, proxyDeviceSection(device))
		}
		return kept, nil
	})
}

// UpsertTUN rewrites the TUN section.
func UpsertTUN(path string, tun TUN) error {
	return editConfig(path, func(sections []section) ([]section, error) {
		replacement := tunSection(tun)
		for i := range sections {
			if sections[i].typ == "tun" {
				replacement.name = sections[i].name
				sections[i] = replacement
				return sections, nil
			}
		}
		return append(sections, replacement), nil
	})
}

// editMu serialises read-modify-write cycles: the RPC handlers and the
// schedulers all edit the same file.
var editMu sync.Mutex

func editConfig(path string, mutate func([]section) ([]section, error)) error {
	editMu.Lock()
	defer editMu.Unlock()
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sections, err := parseUCI(string(data))
	if err != nil {
		return err
	}
	sections, err = mutate(sections)
	if err != nil {
		return err
	}
	cfg := DefaultConfig()
	if err := applySections(&cfg, sections); err != nil {
		return err
	}
	if err := Validate(cfg); err != nil {
		return err
	}
	rendered := renderUCI(sections)
	if err := writeAtomic(path, []byte(rendered), 0644); err != nil {
		return err
	}
	return nil
}

func dnsServerSection(server DNSServer) section {
	options := map[string]string{
		"enabled": boolString(server.Enabled),
		"name":    server.Name,
		"type":    valueOrDefault(server.Type, "udp"),
	}
	set := func(key string, value string) {
		if value != "" {
			options[key] = value
		}
	}
	set("address", server.Address)
	set("detour", server.Detour)
	return section{
		typ:     "dns_server",
		name:    server.ID,
		options: options,
		lists:   map[string][]string{},
	}
}

func transparentSection(transparent Transparent) section {
	options := map[string]string{
		"default_mode": valueOrDefault(transparent.DefaultMode, "off"),
		"dns_hijack":   boolString(transparent.DNSHijack),
		"kill_switch":  boolString(transparent.KillSwitch),
	}
	lists := map[string][]string{}
	if v := cleanList(transparent.LANIfnames); len(v) > 0 {
		lists["lan_ifname"] = v
	}
	if v := cleanList(transparent.BypassSubnet); len(v) > 0 {
		lists["bypass_subnet"] = v
	}
	return section{typ: "transparent", name: "transparent", options: options, lists: lists}
}

func proxyDeviceSection(device Device) section {
	name := device.ID
	if name == "" {
		name = sectionNameFromID(device.Name)
	}
	options := map[string]string{
		"enabled":    boolString(device.Enabled),
		"mode":       valueOrDefault(device.Mode, "default"),
		"bypass_udp": boolString(device.BypassUDP),
	}
	setOrDelete(options, "name", device.Name)
	setOrDelete(options, "mac", device.MAC)
	setOrDelete(options, "ipv4", device.IPv4)
	setOrDelete(options, "ipv6", device.IPv6)
	setOrDelete(options, "group", device.Group)
	return section{typ: "proxy_device", name: name, options: options, lists: map[string][]string{}}
}

func tunSection(tun TUN) section {
	options := map[string]string{
		"enabled":       boolString(tun.Enabled),
		"auto_route":    boolString(tun.AutoRoute),
		"auto_redirect": boolString(tun.AutoRedirect),
	}
	setOrDelete(options, "inet4_address", tun.Inet4Address)
	setOrDelete(options, "inet6_address", tun.Inet6Address)
	return section{typ: "tun", name: "tun", options: options, lists: map[string][]string{}}
}

func setOrDelete(options map[string]string, key string, value string) {
	if value == "" {
		delete(options, key)
		return
	}
	options[key] = value
}

func removeListValue(sec *section, key string, value string) {
	values := sec.lists[key]
	next := make([]string, 0, len(values))
	for _, item := range values {
		if item != value {
			next = append(next, item)
		}
	}
	if len(next) == 0 {
		delete(sec.lists, key)
		return
	}
	sec.lists[key] = next
}

func sectionNameFromID(id string) string {
	name := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		default:
			return '_'
		}
	}, id)
	name = strings.Trim(name, "_")
	if name == "" {
		return "entry"
	}
	return name
}

func renderUCI(sections []section) string {
	var builder strings.Builder
	for i, sec := range sections {
		if i > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString("config ")
		builder.WriteString(quoteUCI(sec.typ))
		if sec.name != "" {
			builder.WriteByte(' ')
			builder.WriteString(quoteUCI(sec.name))
		}
		builder.WriteString("\n")

		for _, key := range sortedKeys(sec.options) {
			builder.WriteString("\toption ")
			builder.WriteString(quoteUCI(key))
			builder.WriteByte(' ')
			builder.WriteString(quoteUCI(sec.options[key]))
			builder.WriteString("\n")
		}
		for _, key := range sortedKeys(sec.lists) {
			for _, value := range sec.lists[key] {
				builder.WriteString("\tlist ")
				builder.WriteString(quoteUCI(key))
				builder.WriteByte(' ')
				builder.WriteString(quoteUCI(value))
				builder.WriteString("\n")
			}
		}
	}
	return builder.String()
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func quoteUCI(value string) string {
	escaped := strings.NewReplacer("\\", "\\\\", "\"", "\\\"").Replace(value)
	return "\"" + escaped + "\""
}

func boolString(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func firstLine(value string) string {
	value = strings.TrimSpace(value)
	if idx := strings.IndexByte(value, '\n'); idx >= 0 {
		value = value[:idx]
	}
	if value == "" {
		return "error"
	}
	return value
}
