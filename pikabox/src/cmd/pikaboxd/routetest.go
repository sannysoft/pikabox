package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	managerconfig "github.com/sannysoft/pikabox/internal/config"
	"github.com/sannysoft/pikabox/internal/render"
	"github.com/sannysoft/pikabox/internal/runtime"
)

// routeMatch explains which route claimed a destination.
type routeMatch struct {
	RouteID   string `json:"route_id"`
	Position  int    `json:"position"` // 1-based, as on the Routing page
	RuleGroup string `json:"rule_group"`
	GroupName string `json:"rule_group_name"`
	Rule      string `json:"rule"` // the "type:value" rule that matched
	Via       string `json:"via"`  // the domain or IP it matched on
}

// routeTestRPC answers "where will this destination go?": it walks the
// routes in order the way sing-box does, then follows the target's live
// selection down to the exit server.
func routeTestRPC(configPath string, raw json.RawMessage) map[string]any {
	var params struct {
		Target string `json:"target"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return fail(err)
	}
	host, port, err := parseTarget(params.Target)
	if err != nil {
		return fail(err)
	}
	cfg, err := load(configPath)
	if err != nil {
		return fail(err)
	}
	plan := render.BuildPlan(*cfg)

	// Domains are matched by name and, like transparent proxying (which sees
	// the destination IP), IP rules are matched against the resolved address.
	var ips []netip.Addr
	if addr, perr := netip.ParseAddr(host); perr == nil {
		ips = []netip.Addr{addr.Unmap()}
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		resolved, _ := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		cancel()
		for _, a := range resolved {
			ips = append(ips, a.Unmap())
		}
	}

	var notes []string
	var matched *routeMatch
	outbound := plan.Final(*cfg)
	switch cfg.Manager.RuntimeMode {
	case "direct":
		outbound = "direct"
		notes = append(notes, "Mode is direct: routes are ignored and everything goes direct.")
	case "global":
		notes = append(notes, "Mode is global: routes are ignored and everything goes to the default.")
	default:
		for i, route := range cfg.Routes {
			group, exists := cfg.RuleGroups[route.RuleGroup]
			if !route.Enabled || !exists || !group.Enabled {
				continue
			}
			rule, via := matchGroup(cfg, group, host, port, ips, &notes)
			if rule == "" {
				continue
			}
			if !plan.Rendered[route.Outbound] {
				notes = append(notes, fmt.Sprintf("Route %d (%s) matches, but its target %q is unavailable right now, so it is skipped.", i+1, firstNonEmptyString(group.Name, group.ID), targetName(cfg, route.Outbound)))
				continue
			}
			matched = &routeMatch{RouteID: route.ID, Position: i + 1, RuleGroup: group.ID, GroupName: firstNonEmptyString(group.Name, group.ID), Rule: rule, Via: via}
			outbound = route.Outbound
			break
		}
	}
	if len(ips) == 0 && net.ParseIP(host) == nil {
		notes = append(notes, "The name did not resolve here, so IP-based rules were not checked.")
	}

	// Follow the live selection (group → member → server) when running.
	tags := []string{outbound}
	if api := newClashAPI(*cfg); api != nil && managedRuntimePID(runtime.DefaultPaths) > 0 {
		if proxies, err := api.proxies(); err == nil {
			for i := 0; i < 8 && proxies[tags[len(tags)-1]].Now != ""; i++ {
				tags = append(tags, proxies[tags[len(tags)-1]].Now)
			}
		}
	} else {
		notes = append(notes, "The proxy is not running, so the server inside a group cannot be shown.")
	}
	ipText := make([]string, 0, len(ips))
	for _, ip := range ips {
		ipText = append(ipText, ip.String())
	}
	result := ok("host", host, "ips", ipText, "outbound", outbound, "outbound_name", targetName(cfg, outbound),
		"chain", chainHops(cfg, tags, nil), "mode", cfg.Manager.RuntimeMode, "notes", append([]string{}, notes...))
	if port > 0 {
		result["port"] = port
	}
	if matched != nil {
		result["matched"] = matched
	}
	return result
}

// parseTarget accepts "example.com", "example.com:443", an IP, "[v6]:port"
// or a pasted URL, and returns the host (lower-case) and optional port.
func parseTarget(raw string) (string, int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", 0, fmt.Errorf("enter a domain, IP or URL")
	}
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" {
			return "", 0, fmt.Errorf("could not read a host from %q", raw)
		}
		port, _ := strconv.Atoi(u.Port())
		if port == 0 {
			switch u.Scheme {
			case "https":
				port = 443
			case "http":
				port = 80
			}
		}
		return strings.ToLower(u.Hostname()), port, nil
	}
	if h, p, err := net.SplitHostPort(raw); err == nil {
		port, perr := strconv.Atoi(p)
		if perr != nil || port < 1 || port > 65535 {
			return "", 0, fmt.Errorf("invalid port in %q", raw)
		}
		return strings.ToLower(h), port, nil
	}
	return strings.ToLower(strings.Trim(raw, "[]/")), 0, nil
}

// matchGroup returns the first rule of group that matches (and what it
// matched on), or "" when none does. Rules this check cannot evaluate
// (process, protocol, network, source) are noted, not guessed.
func matchGroup(cfg *managerconfig.Config, group managerconfig.RuleGroup, host string, port int, ips []netip.Addr, notes *[]string) (string, string) {
	isIP := net.ParseIP(host) != nil
	for _, rule := range group.Rules {
		typ, value, err := managerconfig.ParseRule(rule)
		if err != nil {
			continue
		}
		switch typ {
		case "domain":
			if !isIP && host == strings.ToLower(value) {
				return rule, host
			}
		case "domain_suffix":
			if !isIP && domainSuffix(host, strings.ToLower(value)) {
				return rule, host
			}
		case "domain_keyword":
			if !isIP && strings.Contains(host, strings.ToLower(value)) {
				return rule, host
			}
		case "domain_regex":
			if re, err := regexp.Compile(value); err == nil && !isIP && re.MatchString(host) {
				return rule, host
			}
		case "ip_cidr":
			prefix, err := netip.ParsePrefix(value)
			if err != nil {
				if addr, aerr := netip.ParseAddr(value); aerr == nil {
					prefix = netip.PrefixFrom(addr, addr.BitLen())
				} else {
					continue
				}
			}
			for _, ip := range ips {
				if prefix.Contains(ip) {
					return rule, ip.String()
				}
			}
		case "ip_is_private":
			for _, ip := range ips {
				if (value == "true") == (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()) {
					return rule, ip.String()
				}
			}
		case "port":
			if port > 0 && strconv.Itoa(port) == value {
				return rule, ":" + value
			}
		case "remote":
			set := managerconfig.RemoteRuleSet(value)
			if !fileExists(set.Path) {
				*notes = append(*notes, fmt.Sprintf("Rule-set %s is not downloaded yet, so %q was not checked.", shortURL(value), firstNonEmptyString(group.Name, group.ID)))
				continue
			}
			subjects := []string{}
			if !isIP {
				subjects = append(subjects, host)
			}
			for _, ip := range ips {
				subjects = append(subjects, ip.String())
			}
			for _, subject := range subjects {
				if ruleSetMatches(cfg.Manager.SingBoxBinary, set, subject) {
					return rule, subject
				}
			}
		}
	}
	return "", ""
}

// domainSuffix follows sing-box: ".ru" matches subdomains only, "ru" also
// matches the name itself.
func domainSuffix(host, suffix string) bool {
	if strings.HasPrefix(suffix, ".") {
		return strings.HasSuffix(host, suffix)
	}
	return host == suffix || strings.HasSuffix(host, "."+suffix)
}

// ruleSetMatches asks sing-box itself whether a cached rule-set matches, so
// the answer is exactly what the running config does.
func ruleSetMatches(binary string, set managerconfig.RuleSet, subject string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// sing-box reports a hit as "match rules.[N]: …" on stderr and prints
	// nothing on a miss.
	out, err := exec.CommandContext(ctx, binary, "rule-set", "match", "-f", set.Format, set.Path, subject).CombinedOutput()
	return err == nil && strings.Contains(string(out), "match rules")
}

func shortURL(raw string) string {
	if i := strings.LastIndex(raw, "/"); i >= 0 && i < len(raw)-1 {
		return raw[i+1:]
	}
	return raw
}
