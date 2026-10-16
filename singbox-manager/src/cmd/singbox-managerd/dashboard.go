package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	managerconfig "github.com/openwrt-singbox/singbox-manager/internal/config"
	"github.com/openwrt-singbox/singbox-manager/internal/render"
	"github.com/openwrt-singbox/singbox-manager/internal/runtime"
)

// ---- event log ----

// Event is something the dashboard timeline shows: a fallback switch, a new
// top-N selection, a failed reload or download. Kept in memory only.
type Event struct {
	Time    string `json:"time"`
	Kind    string `json:"kind"` // switch | select | reload | error
	Message string `json:"message"`
}

const maxEvents = 50

var (
	eventsMu sync.Mutex
	events   []Event
)

func recordEvent(kind, format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	log.Printf("%s: %s", kind, message)
	eventsMu.Lock()
	defer eventsMu.Unlock()
	events = append(events, Event{Time: time.Now().UTC().Format(time.RFC3339), Kind: kind, Message: message})
	if len(events) > maxEvents {
		events = events[len(events)-maxEvents:]
	}
}

func recentEvents(n int) []Event {
	eventsMu.Lock()
	defer eventsMu.Unlock()
	out := []Event{}
	for i := len(events) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, events[i])
	}
	return out
}

// ---- probe history ----

type historyPoint struct {
	Time int64 `json:"t"`  // unix seconds
	MS   int   `json:"ms"` // 0 when down
}

const maxHistory = 30

var (
	historyMu sync.Mutex
	history   = map[string][]historyPoint{}
	lastOK    = map[string]string{}
)

func recordProbe(tag string, result managerconfig.ProbeResult) {
	historyMu.Lock()
	defer historyMu.Unlock()
	point := historyPoint{Time: time.Now().Unix()}
	if result.Health == "ok" {
		point.MS = result.LatencyMS
		lastOK[tag] = result.CheckedAt
	}
	points := append(history[tag], point)
	if len(points) > maxHistory {
		points = points[len(points)-maxHistory:]
	}
	history[tag] = points
}

func probeHistory(tag string) ([]historyPoint, string) {
	historyMu.Lock()
	defer historyMu.Unlock()
	return append([]historyPoint{}, history[tag]...), lastOK[tag]
}

// ---- rule-set download errors ----

var (
	ruleSetErrMu sync.Mutex
	ruleSetErrs  = map[string]string{}
)

func setRuleSetError(rawURL string, err error) {
	ruleSetErrMu.Lock()
	defer ruleSetErrMu.Unlock()
	if err == nil {
		delete(ruleSetErrs, rawURL)
		return
	}
	ruleSetErrs[rawURL] = err.Error()
}

// ---- health ----

type healthItem struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Kind      string         `json:"kind"`
	Health    string         `json:"health"` // ok | down | error | idle | unknown
	LatencyMS int            `json:"latency_ms,omitempty"`
	Error     string         `json:"error,omitempty"`
	LastOK    string         `json:"last_ok,omitempty"`
	History   []historyPoint `json:"history"`
	Detail    string         `json:"detail,omitempty"`
	Now       string         `json:"now,omitempty"`
}

type warning struct {
	Level   string `json:"level"` // error | warn
	Title   string `json:"title"`
	Message string `json:"message"`
	Page    string `json:"page,omitempty"` // LuCI sub-page to fix it on
}

func healthRPC(configPath string, _ json.RawMessage) map[string]any {
	cfg, err := load(configPath)
	if err != nil {
		return fail(err)
	}
	plan := render.BuildPlan(*cfg)
	results := managerconfig.LoadProbeResults()
	var proxies map[string]clashProxy
	if api := newClashAPI(*cfg); api != nil && managedRuntimePID(runtime.DefaultPaths) > 0 {
		proxies, _ = api.proxies()
	}

	items := []healthItem{}
	for _, id := range sortedIDs(cfg.OutboundGroups) {
		group := cfg.OutboundGroups[id]
		item := healthItem{ID: id, Name: group.Name, Kind: "group", Health: "unknown", Detail: group.Mode}
		if !plan.Rendered[id] {
			item.Health, item.Error = "error", "no available members"
		} else if now := proxies[id].Now; now != "" {
			item.Now = displayName(cfg, now)
			if delay := proxies[now].lastDelay(); delay > 0 {
				item.Health, item.LatencyMS = "ok", delay
			} else if result, probed := results[now]; probed {
				item.Health, item.LatencyMS = result.Health, result.LatencyMS
			}
		} else if managedRuntimePID(runtime.DefaultPaths) == 0 {
			item.Health = "idle"
		}
		item.History, item.LastOK = probeHistory(id)
		items = append(items, item)
	}
	for _, id := range sortedIDs(cfg.Subscriptions) {
		sub := cfg.Subscriptions[id]
		state := managerconfig.LoadSubscriptionState(id)
		item := healthItem{ID: id, Name: sub.Name, Kind: "subscription", Health: "unknown"}
		alive, best := 0, 0
		for _, candidate := range state.Matched {
			if result := results[candidate]; result.Health == "ok" {
				alive++
				if best == 0 || result.LatencyMS < best {
					best = result.LatencyMS
				}
			}
		}
		item.Detail = fmt.Sprintf("%d of %d alive · %d in use", alive, len(state.Matched), len(state.Selected))
		switch {
		case !sub.Enabled:
			item.Health = "idle"
		case sub.LastError != "":
			item.Health, item.Error = "error", sub.LastError
		case alive > 0:
			item.Health, item.LatencyMS = "ok", best
		case len(state.Matched) > 0:
			item.Health = "down"
		}
		item.History, item.LastOK = probeHistory(id)
		items = append(items, item)
	}
	for _, id := range sortedIDs(cfg.Outbounds) {
		outbound := cfg.Outbounds[id]
		item := healthItem{ID: id, Name: outbound.Name, Kind: "outbound", Health: "unknown"}
		if !outbound.Enabled {
			item.Health = "idle"
		} else if !plan.Rendered[id] {
			item.Health, item.Error = "error", "invalid source"
		} else if result, probed := results[id]; probed {
			item.Health, item.LatencyMS, item.Error = result.Health, result.LatencyMS, result.Error
		}
		item.History, item.LastOK = probeHistory(id)
		items = append(items, item)
	}

	warnings := []warning{}
	if managedRuntimePID(runtime.DefaultPaths) == 0 && runtime.DataPathActive() {
		warnings = append(warnings, warning{"error", "Proxy rules active while sing-box is stopped", "In-scope devices have no internet. Start the proxy or remove the rules.", "dashboard"})
	}
	for _, w := range plan.Warnings {
		warnings = append(warnings, warning{"warn", "Not rendered", w, "groups"})
	}
	for _, id := range sortedIDs(cfg.Subscriptions) {
		sub := cfg.Subscriptions[id]
		if !sub.Enabled {
			continue
		}
		if sub.LastError != "" {
			warnings = append(warnings, warning{"error", "Subscription " + sub.Name + " failed to refresh", sub.LastError, "subscriptions"})
		}
		if skipped := managerconfig.LoadSubscriptionState(id).Skipped; skipped != "" {
			warnings = append(warnings, warning{"warn", "Subscription " + sub.Name + ": entries skipped", skipped, "subscriptions"})
		}
	}
	ruleSetErrMu.Lock()
	for rawURL, message := range ruleSetErrs {
		warnings = append(warnings, warning{"warn", "Rule-set not downloaded", rawURL + ": " + message, "rules"})
	}
	ruleSetErrMu.Unlock()
	nodes, edges := healthGraph(cfg, plan, items, proxies, results)
	return ok("items", items, "warnings", warnings, "events", recentEvents(12), "nodes", nodes, "edges", edges)
}

// ---- health graph ----

type graphNode struct {
	healthItem
	Column int  `json:"column"`
	Unused bool `json:"unused,omitempty"` // no route reaches it
	More   int  `json:"more,omitempty"`   // subscription: alive candidates not in use
}

type graphEdge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Active bool   `json:"active,omitempty"` // the parent currently uses this child
}

// healthGraph lays the outbound graph out in columns: routes (column 0) →
// groups → subscriptions/outbounds → the subscription servers in use. A node
// sits one column right of its deepest parent, so shared members appear once
// with several incoming edges.
func healthGraph(cfg *managerconfig.Config, plan render.Plan, items []healthItem, proxies map[string]clashProxy, results map[string]managerconfig.ProbeResult) ([]graphNode, []graphEdge) {
	byID := map[string]*graphNode{}
	var order []string
	add := func(node graphNode) *graphNode {
		if existing, seen := byID[node.ID]; seen {
			return existing
		}
		byID[node.ID] = &node
		order = append(order, node.ID)
		return &node
	}
	for _, item := range items {
		add(graphNode{healthItem: item})
	}
	var edges []graphEdge
	children := map[string][]string{}
	link := func(from, to string, active bool) {
		edges = append(edges, graphEdge{From: from, To: to, Active: active})
		children[from] = append(children[from], to)
	}
	builtin := func(id string) {
		add(graphNode{healthItem: healthItem{ID: id, Name: id, Kind: "builtin", Health: "ok"}})
	}

	// Column 0: what sends traffic somewhere.
	running := managedRuntimePID(runtime.DefaultPaths) > 0
	final := plan.Final(*cfg)
	var roots []string
	for _, route := range cfg.Routes {
		name := route.RuleGroup
		if group, found := cfg.RuleGroups[route.RuleGroup]; found && group.Name != "" {
			name = group.Name
		}
		root := "route:" + route.ID
		node := add(graphNode{healthItem: healthItem{ID: root, Name: name, Kind: "rule", Health: "ok"}})
		if !route.Enabled {
			node.Health = "idle"
		}
		roots = append(roots, root)
		if route.Outbound == "direct" || route.Outbound == "block" {
			builtin(route.Outbound)
		}
		link(root, route.Outbound, false)
	}
	add(graphNode{healthItem: healthItem{ID: "route:final", Name: "Everything else", Kind: "rule", Health: "ok"}})
	roots = append(roots, "route:final")
	if final == "direct" || final == "block" {
		builtin(final)
	}
	link("route:final", final, running)

	for _, id := range sortedIDs(cfg.OutboundGroups) {
		for _, member := range cfg.OutboundGroups[id].Members {
			if member == "direct" || member == "block" {
				builtin(member)
			}
			link(id, member, running && proxies[id].Now == member)
		}
	}
	// Subscriptions fan out to the servers they currently use.
	for _, id := range sortedIDs(cfg.Subscriptions) {
		state := managerconfig.LoadSubscriptionState(id)
		names := map[string]string{}
		for _, candidate := range state.Candidates {
			names[candidate.ID] = candidate.Name
		}
		selected := map[string]bool{}
		for _, tag := range state.Selected {
			selected[tag] = true
			result := results[tag]
			add(graphNode{healthItem: healthItem{ID: tag, Name: names[tag], Kind: "server", Health: firstNonEmptyString(result.Health, "unknown"), LatencyMS: result.LatencyMS, Error: result.Error}})
			link(id, tag, running && proxies[id].Now == tag)
		}
		if node, found := byID[id]; found {
			for _, tag := range state.Matched {
				if !selected[tag] && results[tag].Health == "ok" {
					node.More++
				}
			}
		}
	}

	// Columns: longest path from the roots; reachability marks unused nodes.
	column := map[string]int{}
	reached := map[string]bool{}
	var walk func(id string, depth int, onPath map[string]bool)
	walk = func(id string, depth int, onPath map[string]bool) {
		if onPath[id] {
			return // cycles are rejected by validation; never loop here
		}
		reached[id] = true
		if depth > column[id] || column[id] == 0 {
			column[id] = depth
		}
		onPath[id] = true
		for _, child := range children[id] {
			walk(child, depth+1, onPath)
		}
		delete(onPath, id)
	}
	for _, root := range roots {
		walk(root, 0, map[string]bool{})
	}
	// Unreached nodes keep their structure: one right of their deepest parent.
	for changed := true; changed; {
		changed = false
		for _, edge := range edges {
			if reached[edge.To] {
				continue
			}
			if want := column[edge.From] + 1; column[edge.To] < want {
				column[edge.To] = want
				changed = true
			}
		}
	}
	nodes := make([]graphNode, 0, len(order))
	for _, id := range order {
		node := *byID[id]
		node.Column = column[id]
		if node.Column == 0 && node.Kind != "rule" {
			node.Column = 1
		}
		node.Unused = !reached[id]
		nodes = append(nodes, node)
	}
	// Drop edges whose target never became a node (a missing member).
	kept := edges[:0]
	for _, edge := range edges {
		if byID[edge.To] != nil {
			kept = append(kept, edge)
		}
	}
	return nodes, kept
}

// ---- connections ----

type connectionView struct {
	ID       string   `json:"id"`
	Host     string   `json:"host"`
	Port     string   `json:"port"`
	Network  string   `json:"network"`
	Source   string   `json:"source"`
	Device   string   `json:"device,omitempty"`
	Route    string   `json:"route"`      // what decided the outbound
	RouteKey string   `json:"route_key"`  // route ID, "final", "dns", ...
	Chain    []string `json:"chain"`      // outermost first, display names
	Kinds    []string `json:"chain_kind"` // kind per chain entry
	Rule     string   `json:"rule"`
	Upload   int64    `json:"upload"`
	Download int64    `json:"download"`
	Start    string   `json:"start"`
}

type clashConnection struct {
	ID       string   `json:"id"`
	Chains   []string `json:"chains"`
	Rule     string   `json:"rule"`
	Upload   int64    `json:"upload"`
	Download int64    `json:"download"`
	Start    string   `json:"start"`
	Metadata struct {
		Host            string `json:"host"`
		DestinationIP   string `json:"destinationIP"`
		DestinationPort string `json:"destinationPort"`
		Network         string `json:"network"`
		SourceIP        string `json:"sourceIP"`
	} `json:"metadata"`
}

func (api *clashAPI) getJSON(path string, v any) error {
	resp, err := api.client.Get(api.base + path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", path, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

const maxConnections = 400

func connectionsRPC(configPath string, _ json.RawMessage) map[string]any {
	cfg, err := load(configPath)
	if err != nil {
		return fail(err)
	}
	api := newClashAPI(*cfg)
	if api == nil || managedRuntimePID(runtime.DefaultPaths) == 0 {
		return ok("connections", []connectionView{}, "running", false)
	}
	var snapshot struct {
		Connections   []clashConnection `json:"connections"`
		UploadTotal   int64             `json:"uploadTotal"`
		DownloadTotal int64             `json:"downloadTotal"`
	}
	if err := api.getJSON("/connections", &snapshot); err != nil {
		return fail(err)
	}
	var rules struct {
		Rules []struct {
			Payload string `json:"payload"`
			Proxy   string `json:"proxy"`
		} `json:"rules"`
	}
	_ = api.getJSON("/rules", &rules)
	labels := render.RouteLabels(*cfg)
	ruleIndex := map[string]int{}
	for i, rule := range rules.Rules {
		key := rule.Payload + " => " + rule.Proxy
		if _, seen := ruleIndex[key]; !seen {
			ruleIndex[key] = i
		}
	}
	deviceNames := map[string]string{}
	for _, device := range discoverDevices() {
		if device.Name != "" {
			deviceNames[device.IP] = device.Name
		}
	}
	for _, device := range cfg.Transparent.Devices {
		for _, ip := range []string{device.IPv4, device.IPv6} {
			if ip != "" && device.Name != "" {
				deviceNames[ip] = device.Name
			}
		}
	}

	views := make([]connectionView, 0, len(snapshot.Connections))
	for _, c := range snapshot.Connections {
		view := connectionView{
			ID: c.ID, Host: c.Metadata.Host, Port: c.Metadata.DestinationPort, Network: c.Metadata.Network,
			Source: c.Metadata.SourceIP, Device: deviceNames[c.Metadata.SourceIP],
			Rule: c.Rule, Upload: c.Upload, Download: c.Download, Start: c.Start,
		}
		if view.Host == "" {
			view.Host = c.Metadata.DestinationIP
		}
		// Clash lists the chain leaf-first; show it the way traffic flows.
		for i := len(c.Chains) - 1; i >= 0; i-- {
			view.Chain = append(view.Chain, displayName(cfg, c.Chains[i]))
			kind := cfg.Kind(c.Chains[i])
			if kind == "" && strings.Contains(c.Chains[i], ".") {
				kind = "server" // a subscription's candidate
			} else if kind == "builtin" {
				kind = c.Chains[i]
			}
			view.Kinds = append(view.Kinds, kind)
		}
		view.Route, view.RouteKey = describeRule(cfg, c.Rule, ruleIndex, labels)
		views = append(views, view)
	}
	sort.Slice(views, func(i, j int) bool { return views[i].Start > views[j].Start })
	if len(views) > maxConnections {
		views = views[:maxConnections]
	}
	return ok("connections", views, "running", true, "upload_total", snapshot.UploadTotal, "download_total", snapshot.DownloadTotal, "count", len(snapshot.Connections))
}

// describeRule names what routed a connection: one of our routes (by its
// rule group), the default route, DNS, or a device pin.
func describeRule(cfg *managerconfig.Config, rule string, index map[string]int, labels []render.RuleLabel) (string, string) {
	if rule == "final" || rule == "" {
		return "Everything else", "final"
	}
	i, found := index[rule]
	if !found || i >= len(labels) {
		return rule, "unknown"
	}
	label := labels[i]
	switch label.Kind {
	case "route":
		name := label.Group
		if group, ok := cfg.RuleGroups[label.Group]; ok && group.Name != "" {
			name = group.Name
		}
		return name, label.ID
	case "device":
		return "Device pin " + label.ID, "device"
	case "dns":
		return "DNS", "dns"
	}
	return label.Kind, label.Kind
}

func closeConnectionRPC(configPath string, raw json.RawMessage) map[string]any {
	params, err := decodeIDParams(raw)
	if err != nil || params.ID == "" {
		return fail(fmt.Errorf("id is required"))
	}
	cfg, err := load(configPath)
	if err != nil {
		return fail(err)
	}
	api := newClashAPI(*cfg)
	if api == nil {
		return fail(fmt.Errorf("the controller API is disabled"))
	}
	req, _ := http.NewRequest(http.MethodDelete, api.base+"/connections/"+url.PathEscape(params.ID), nil)
	resp, err := api.client.Do(req)
	if err != nil {
		return fail(err)
	}
	resp.Body.Close()
	return ok("id", params.ID)
}

// chainInfo describes each hop of the live exit path for the status view.
type chainHop struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Detail    string `json:"detail,omitempty"`
	LatencyMS int    `json:"latency_ms,omitempty"`
}

func chainHops(cfg *managerconfig.Config, tags []string, proxies map[string]clashProxy) []chainHop {
	results := managerconfig.LoadProbeResults()
	hops := make([]chainHop, 0, len(tags))
	for _, tag := range tags {
		hop := chainHop{ID: tag, Name: displayName(cfg, tag), Kind: cfg.Kind(tag)}
		switch {
		case hop.Kind == "builtin":
			hop.Kind = tag
		case hop.Kind == "" && strings.Contains(tag, "."):
			hop.Kind = "server"
		case hop.Kind == "group":
			hop.Detail = cfg.OutboundGroups[tag].Mode
		}
		if delay := proxies[tag].lastDelay(); delay > 0 {
			hop.LatencyMS = delay
		} else if result, probed := results[tag]; probed && result.Health == "ok" {
			hop.LatencyMS = result.LatencyMS
		}
		hops = append(hops, hop)
	}
	return hops
}
