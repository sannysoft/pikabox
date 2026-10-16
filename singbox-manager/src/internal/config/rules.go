package config

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/netip"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// RuleSetDir caches remote rule-sets referenced by rule groups.
var RuleSetDir = "/etc/singbox-manager/rulesets"

// RuleTypes are the accepted rule-group rule types: sing-box route rule match
// fields, plus "remote" for a rule-set URL (.srs = binary, else JSON source).
var RuleTypes = []string{
	"domain", "domain_suffix", "domain_keyword", "domain_regex",
	"ip_cidr", "source_ip_cidr", "ip_is_private",
	"port", "port_range", "source_port", "source_port_range",
	"network", "protocol", "process_name", "process_path", "package_name",
	"remote",
}

// ParseRule splits a "<type>:<value>" rule-group entry and validates it.
func ParseRule(rule string) (string, string, error) {
	typ, value, ok := strings.Cut(strings.TrimSpace(rule), ":")
	typ, value = strings.TrimSpace(typ), strings.TrimSpace(value)
	if !ok || value == "" {
		return "", "", fmt.Errorf("rule %q must be <type>:<value>", rule)
	}
	if !inSet(typ, RuleTypes...) {
		return "", "", fmt.Errorf("rule %q: unsupported type %q", rule, typ)
	}
	switch typ {
	case "ip_cidr", "source_ip_cidr":
		if _, err := netip.ParsePrefix(value); err != nil {
			if _, err := netip.ParseAddr(value); err != nil {
				return "", "", fmt.Errorf("rule %q: invalid IP/CIDR", rule)
			}
		}
	case "port", "source_port":
		if port, err := strconv.Atoi(value); err != nil || port < 1 || port > 65535 {
			return "", "", fmt.Errorf("rule %q: invalid port", rule)
		}
	case "ip_is_private":
		if value != "true" && value != "false" {
			return "", "", fmt.Errorf("rule %q: value must be true or false", rule)
		}
	case "remote":
		if parsed, err := url.Parse(value); err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return "", "", fmt.Errorf("rule %q: remote needs an http(s) URL", rule)
		}
	}
	return typ, value, nil
}

// RemoteRuleSet derives the cached rule-set for a remote URL. The tag and file
// name are a hash of the URL, so the same URL in several groups is one set.
func RemoteRuleSet(rawURL string) RuleSet {
	sum := sha1.Sum([]byte(rawURL))
	id := "rs-" + hex.EncodeToString(sum[:])[:10]
	format, ext := "source", ".json"
	if strings.HasSuffix(strings.ToLower(strings.SplitN(rawURL, "?", 2)[0]), ".srs") {
		format, ext = "binary", ".srs"
	}
	return RuleSet{ID: id, URL: rawURL, Format: format, Path: filepath.Join(RuleSetDir, id+ext)}
}

// RemoteRuleSets returns every remote rule-set referenced by an enabled rule
// group, deduplicated and ordered by tag.
func (cfg Config) RemoteRuleSets() []RuleSet {
	byID := map[string]RuleSet{}
	for _, group := range cfg.RuleGroups {
		if !group.Enabled {
			continue
		}
		for _, rule := range group.Rules {
			if typ, value, err := ParseRule(rule); err == nil && typ == "remote" {
				set := RemoteRuleSet(value)
				byID[set.ID] = set
			}
		}
	}
	sets := make([]RuleSet, 0, len(byID))
	for _, set := range byID {
		sets = append(sets, set)
	}
	sort.Slice(sets, func(i, j int) bool { return sets[i].ID < sets[j].ID })
	return sets
}
