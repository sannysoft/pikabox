package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	managerconfig "github.com/sannysoft/pikabox/internal/config"
	"github.com/sannysoft/pikabox/internal/health"
	"github.com/sannysoft/pikabox/internal/render"
	"github.com/sannysoft/pikabox/internal/runtime"
	"github.com/sannysoft/pikabox/internal/subscription"
)

// rpcMethod is one ubus method: its rpcd signature, how long the rpcd wrapper
// waits for it, and whether a successful call reloads the running sing-box.
type rpcMethod struct {
	params  map[string]any
	timeout time.Duration
	reload  bool
	handler func(configPath string, raw json.RawMessage) map[string]any
}

const (
	quick        = 5 * time.Second
	probe        = 180 * time.Second
	pendingAfter = 15 * time.Second
)

var methods = map[string]rpcMethod{
	"status":              {nil, quick, false, func(p string, _ json.RawMessage) map[string]any { return toMap(collectStatus(p)) }},
	"start":               {nil, quick, false, func(p string, _ json.RawMessage) map[string]any { return startRuntime(p) }},
	"stop":                {nil, 30 * time.Second, false, func(p string, _ json.RawMessage) map[string]any { return stopRuntime(p) }},
	"cleanup":             {nil, 30 * time.Second, false, func(p string, _ json.RawMessage) map[string]any { return cleanupResult(p) }},
	"restart":             {nil, 30 * time.Second, false, func(p string, _ json.RawMessage) map[string]any { return restartRuntime(p) }},
	"reload":              {nil, 30 * time.Second, false, func(p string, _ json.RawMessage) map[string]any { return controlRuntime(p, runtime.ActionReload) }},
	"validate":            {nil, 30 * time.Second, false, func(p string, _ json.RawMessage) map[string]any { return validateRuntimeConfig(p) }},
	"manager_set_enabled": {map[string]any{"enabled": "boolean"}, quick, false, setManagerEnabled},
	"manager_set_paused":  {map[string]any{"paused": "boolean"}, 30 * time.Second, false, setManagerPaused},
	"manager_set_mode":    {map[string]any{"mode": "string"}, 30 * time.Second, true, setManagerMode},
	"settings":            {nil, quick, false, settingsRPC},
	"settings_set":        {map[string]any{"settings": "object"}, 30 * time.Second, true, setSettings},
	"config_export":       {nil, quick, false, exportConfigRPC},
	"config_import":       {map[string]any{"text": "string", "dry_run": "boolean"}, 30 * time.Second, false, importConfigRPC},
	"backup_read":         {map[string]any{"name": "string"}, quick, false, backupReadRPC},
	"runtime_stats":       {nil, quick, false, func(string, json.RawMessage) map[string]any { return toMap(runtimeStats()) }},
	"logs":                {map[string]any{"lines": "number"}, quick, false, func(_ string, raw json.RawMessage) map[string]any { return logs(raw) }},
	"devices":             {nil, quick, false, func(string, json.RawMessage) map[string]any { return devices() }},
	"latency_test":        {map[string]any{"url": "string"}, 30 * time.Second, false, func(_ string, raw json.RawMessage) map[string]any { return latencyTest(raw) }},

	"probe": {nil, probe, false, func(p string, _ json.RawMessage) map[string]any {
		return ok("results", probeAll(p, nil, true))
	}},

	"health":           {nil, quick, false, healthRPC},
	"connections":      {nil, quick, false, connectionsRPC},
	"connection_close": {map[string]any{"id": "string"}, quick, false, closeConnectionRPC},

	"outbounds":       {nil, quick, false, listOutbounds},
	"outbound_set":    {map[string]any{"outbound": "object"}, 30 * time.Second, true, setOutbound},
	"outbound_delete": {map[string]any{"id": "string"}, 30 * time.Second, true, deleteOf("outbound")},
	"outbound_test":   {map[string]any{"id": "string"}, probe, false, testOutbounds},

	"subscriptions":          {nil, quick, false, listSubscriptions},
	"subscription_set":       {map[string]any{"subscription": "object"}, probe, true, setSubscription},
	"subscription_delete":    {map[string]any{"id": "string"}, 30 * time.Second, true, deleteOf("subscription")},
	"subscription_refresh":   {map[string]any{"id": "string"}, probe, false, refreshSubscriptionRPC},
	"subscription_test":      {map[string]any{"id": "string"}, probe, false, testSubscription},
	"subscription_outbounds": {map[string]any{"id": "string"}, quick, false, subscriptionOutbounds},
	"filter_preview":         {map[string]any{"id": "string", "filter": "string"}, quick, false, filterPreview},

	"outbound_groups":       {nil, 30 * time.Second, false, listOutboundGroups},
	"outbound_group_set":    {map[string]any{"group": "object"}, 30 * time.Second, true, setOutboundGroup},
	"outbound_group_delete": {map[string]any{"id": "string"}, 30 * time.Second, true, deleteOf("outbound_group")},

	"rule_groups":       {nil, quick, false, listRuleGroups},
	"rule_group_set":    {map[string]any{"group": "object"}, 30 * time.Second, true, setRuleGroup},
	"rule_group_delete": {map[string]any{"id": "string"}, 30 * time.Second, true, deleteRuleGroup},
	"rulesets_refresh":  {nil, probe, false, refreshRuleSets},

	"routing":     {nil, quick, false, listRouting},
	"route_test":  {map[string]any{"target": "string"}, 30 * time.Second, false, routeTestRPC},
	"routing_set": {map[string]any{"final": "string", "routes": "array"}, 30 * time.Second, true, setRouting},

	"dns":               {nil, quick, false, listDNS},
	"dns_server_set":    {map[string]any{"server": "object"}, 30 * time.Second, true, setDNSServer},
	"dns_server_delete": {map[string]any{"id": "string"}, 30 * time.Second, true, deleteDNSServer},
	"dns_rule_set":      {map[string]any{"rule": "object"}, 30 * time.Second, true, setDNSRule},
	"dns_rule_delete":   {map[string]any{"id": "string"}, 30 * time.Second, true, deleteDNSRule},
	"dns_test":          {map[string]any{"server": "string", "domain": "string"}, 30 * time.Second, false, dnsTest},

	"transparent":     {nil, quick, false, func(p string, _ json.RawMessage) map[string]any { return transparentStatus(p) }},
	"transparent_set": {map[string]any{"transparent": "object"}, 30 * time.Second, true, setTransparent},
	"tun":             {nil, quick, false, func(p string, _ json.RawMessage) map[string]any { return tunStatus(p) }},
	"tun_set":         {map[string]any{"tun": "object"}, 30 * time.Second, true, setTUN},
}

func toMap(value any) map[string]any {
	data, _ := json.Marshal(value)
	out := map[string]any{}
	_ = json.Unmarshal(data, &out)
	return out
}

func fail(err error) map[string]any { return validationResult(false, runtime.Result{}, err) }

func ok(fields ...any) map[string]any {
	out := map[string]any{"ok": true, "errors": []string{}}
	for i := 0; i+1 < len(fields); i += 2 {
		out[fields[i].(string)] = fields[i+1]
	}
	return out
}

// decode reads params either wrapped ({"<key>": {...}}, as LuCI's rpc.declare
// sends them) or bare.
func decode[T any](raw json.RawMessage, key string) (T, error) {
	var value T
	if len(raw) == 0 {
		return value, fmt.Errorf("missing %s", key)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err == nil {
		if inner, found := envelope[key]; found {
			return value, json.Unmarshal(inner, &value)
		}
	}
	return value, json.Unmarshal(raw, &value)
}

func load(configPath string) (*managerconfig.Config, error) { return managerconfig.Load(configPath) }

// newID picks the section ID for a new entry from its name, refusing names
// whose ID is already taken by any outbound, subscription, or group.
func newID(cfg *managerconfig.Config, id string, name string, typ string) (string, error) {
	if id != "" {
		if kind := cfg.Kind(id); kind != "" && kind != typ {
			return "", fmt.Errorf("id %q is already used by a %s", id, kind)
		}
		return id, nil
	}
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("name is required")
	}
	id = managerconfig.SectionID(name)
	if cfg.Kind(id) != "" || id == "proxy" {
		return "", fmt.Errorf("name %q is already in use", name)
	}
	return id, nil
}

func deleteOf(typ string) func(string, json.RawMessage) map[string]any {
	return func(configPath string, raw json.RawMessage) map[string]any {
		params, err := decodeIDParams(raw)
		if err != nil {
			return fail(err)
		}
		if err := managerconfig.DeleteTarget(configPath, typ, params.ID); err != nil {
			return fail(err)
		}
		if typ == "subscription" {
			managerconfig.DeleteSubscriptionState(params.ID)
		}
		return ok("id", params.ID)
	}
}

// ---- status ----

// collectStatus reports runtime state plus the live exit: the final target
// resolved through the running groups (Clash API "now") down to a leaf.
func collectStatus(configPath string) Status {
	cfg, err := load(configPath)
	if err != nil {
		defaultCfg := managerconfig.DefaultConfig()
		cfg = &defaultCfg
	}
	pid := managedRuntimePID(runtime.DefaultPaths)
	stats := collectRuntimeStats()
	plan := render.BuildPlan(*cfg)
	final := plan.Final(*cfg)
	status := Status{
		Version:         version,
		Daemon:          err == nil,
		ManagerEnabled:  cfg.Manager.Enabled,
		ManagerPaused:   cfg.Manager.Paused,
		Running:         pid > 0,
		SingBoxPID:      pid,
		Final:           final,
		ActiveGroup:     final,
		RuntimeMode:     cfg.Manager.RuntimeMode,
		Health:          "unknown",
		Connections:     stats.Connections,
		RxBytes:         stats.RxBytes,
		TxBytes:         stats.TxBytes,
		TProxyEnabled:   cfg.Transparent.Active(),
		DNSHijack:       cfg.Transparent.DNSHijack,
		KillSwitch:      cfg.Transparent.KillSwitch,
		NftablesInclude: runtime.DefaultPaths.NftablesInclude,
		TUNEnabled:      cfg.TUN.Enabled,
		StaleDataPath:   pid == 0 && runtime.DataPathActive(),
		Warnings:        plan.Warnings,
	}
	if group, isGroup := cfg.OutboundGroups[final]; isGroup {
		status.Strategy = group.Mode
	}
	status.Events = recentEvents(6)
	if pid > 0 {
		status.MemoryKB = readRSS(pid)
		if api := newClashAPI(*cfg); api != nil {
			if proxies, err := api.proxies(); err == nil {
				leaf := final
				for i := 0; i < 8 && proxies[leaf].Now != ""; i++ {
					status.Chain = append(status.Chain, leaf)
					leaf = proxies[leaf].Now
				}
				status.SelectedOutbound = leaf
				for _, tag := range status.Chain {
					status.ChainNames = append(status.ChainNames, displayName(cfg, tag))
				}
				status.SelectedName = displayName(cfg, leaf)
				status.Hops = chainHops(cfg, append(append([]string{}, status.Chain...), leaf), proxies)
				if delay := proxies[leaf].lastDelay(); delay > 0 {
					status.LatencyMS = delay
					status.Health = "ok"
				} else if len(proxies[leaf].History) > 0 {
					status.Health = "down"
				}
			}
		}
	}
	return status
}

// ---- outbounds ----

type outboundView struct {
	managerconfig.Outbound
	Type   string                     `json:"type"`
	Server string                     `json:"server"`
	Port   any                        `json:"port"`
	Error  string                     `json:"error,omitempty"`
	Probe  *managerconfig.ProbeResult `json:"probe,omitempty"`
	UsedBy []managerconfig.Usage      `json:"used_by"`
}

func listOutbounds(configPath string, _ json.RawMessage) map[string]any {
	cfg, err := load(configPath)
	if err != nil {
		return fail(err)
	}
	results := managerconfig.LoadProbeResults()
	views := []outboundView{}
	for _, id := range sortedIDs(cfg.Outbounds) {
		outbound := cfg.Outbounds[id]
		view := outboundView{Outbound: outbound, UsedBy: cfg.UsedBy(id)}
		if item, err := subscription.ParseSource(outbound.Source); err != nil {
			view.Error = err.Error()
		} else {
			view.Type, _ = item.Outbound["type"].(string)
			view.Server, _ = item.Outbound["server"].(string)
			view.Port = item.Outbound["server_port"]
		}
		if result, found := results[id]; found {
			view.Probe = &result
		}
		views = append(views, view)
	}
	return ok("outbounds", views)
}

func setOutbound(configPath string, raw json.RawMessage) map[string]any {
	outbound, err := decode[managerconfig.Outbound](raw, "outbound")
	if err != nil {
		return fail(err)
	}
	cfg, err := load(configPath)
	if err != nil {
		return fail(err)
	}
	if outbound.ID, err = newID(cfg, outbound.ID, outbound.Name, "outbound"); err != nil {
		return fail(err)
	}
	if _, err := subscription.ParseSource(outbound.Source); err != nil {
		return fail(err)
	}
	if err := managerconfig.UpsertOutbound(configPath, outbound); err != nil {
		return fail(err)
	}
	return ok("id", outbound.ID)
}

// testOutbounds probes one outbound (id) or, with no id, every outbound.
func testOutbounds(configPath string, raw json.RawMessage) map[string]any {
	params, _ := decodeIDParams(raw)
	if params.ID == "" {
		results := probeAll(configPath, noSubscriptions, true)
		return ok("results", results)
	}
	cfg, err := load(configPath)
	if err != nil {
		return fail(err)
	}
	outbound, found := cfg.Outbounds[params.ID]
	if !found {
		return fail(fmt.Errorf("outbound %q not found", params.ID))
	}
	item, err := subscription.ParseSource(outbound.Source)
	if err != nil {
		return fail(err)
	}
	// A manual test always includes the bulk check (when enabled).
	opts := health.ProbeOptions{Binary: cfg.Manager.SingBoxBinary, TestURL: cfg.Manager.TestURL, BulkURL: bulkURL(cfg)}
	opts.BulkSelect = func(delay map[string]health.EndpointResult) []string {
		if delay[params.ID].Health == "ok" {
			return []string{params.ID}
		}
		return nil
	}
	probeMu.Lock()
	results := health.ProbeOutbounds(context.Background(), opts, map[string]map[string]any{params.ID: item.Outbound})
	probeMu.Unlock()
	stored := managerconfig.LoadProbeResults()
	stored[params.ID] = mergeProbe(stored[params.ID], results[params.ID], time.Now().UTC().Format(time.RFC3339))
	_ = managerconfig.SaveProbeResults(stored)
	recordProbe(params.ID, stored[params.ID])
	return ok("result", stored[params.ID])
}

// ---- subscriptions ----

type subscriptionView struct {
	managerconfig.Subscription
	FetchedAt string          `json:"fetched_at"`
	Total     int             `json:"total"`
	Skipped   string          `json:"skipped,omitempty"`
	Matched   int             `json:"matched"`
	Alive     int             `json:"alive"`
	BestMS    int             `json:"best_ms"`
	Selected  []candidateView `json:"selected"`
}

type candidateView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Server    string `json:"server"`
	Port      any    `json:"port"`
	Matched   bool   `json:"matched"`
	Selected  bool   `json:"selected"`
	Health    string `json:"health,omitempty"`
	LatencyMS int    `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
	Bulk      string `json:"bulk,omitempty"`
	BulkKBps  int    `json:"bulk_kbps,omitempty"`
	BulkError string `json:"bulk_error,omitempty"`
}

func candidateViews(state managerconfig.SubscriptionState, results map[string]managerconfig.ProbeResult) []candidateView {
	matched := map[string]bool{}
	for _, id := range state.Matched {
		matched[id] = true
	}
	selected := map[string]bool{}
	for _, id := range state.Selected {
		selected[id] = true
	}
	views := make([]candidateView, 0, len(state.Candidates))
	for _, c := range state.Candidates {
		view := candidateView{ID: c.ID, Name: c.Name, Matched: matched[c.ID], Selected: selected[c.ID], Port: c.Outbound["server_port"]}
		view.Type, _ = c.Outbound["type"].(string)
		view.Server, _ = c.Outbound["server"].(string)
		if result, found := results[c.ID]; found && view.Matched {
			view.Health, view.LatencyMS, view.Error = result.Health, result.LatencyMS, result.Error
			if result.BulkFresh() {
				view.Bulk, view.BulkKBps, view.BulkError = result.Bulk, result.BulkKBps, result.BulkError
			}
		}
		views = append(views, view)
	}
	return views
}

func listSubscriptions(configPath string, _ json.RawMessage) map[string]any {
	cfg, err := load(configPath)
	if err != nil {
		return fail(err)
	}
	results := managerconfig.LoadProbeResults()
	views := []subscriptionView{}
	for _, id := range sortedIDs(cfg.Subscriptions) {
		state := managerconfig.LoadSubscriptionState(id)
		view := subscriptionView{Subscription: cfg.Subscriptions[id], FetchedAt: state.FetchedAt, Total: len(state.Candidates), Skipped: state.Skipped, Matched: len(state.Matched), Selected: []candidateView{}}
		for _, candidate := range candidateViews(state, results) {
			if candidate.Health == "ok" {
				view.Alive++
				if view.BestMS == 0 || candidate.LatencyMS < view.BestMS {
					view.BestMS = candidate.LatencyMS
				}
			}
			if candidate.Selected {
				view.Selected = append(view.Selected, candidate)
			}
		}
		sort.SliceStable(view.Selected, func(i, j int) bool { return view.Selected[i].LatencyMS < view.Selected[j].LatencyMS })
		views = append(views, view)
	}
	return ok("subscriptions", views)
}

func setSubscription(configPath string, raw json.RawMessage) map[string]any {
	sub, err := decode[managerconfig.Subscription](raw, "subscription")
	if err != nil {
		return fail(err)
	}
	cfg, err := load(configPath)
	if err != nil {
		return fail(err)
	}
	existing, isEdit := cfg.Subscriptions[sub.ID]
	if sub.ID, err = newID(cfg, sub.ID, sub.Name, "subscription"); err != nil {
		return fail(err)
	}
	if err := managerconfig.CompileFilter(sub.Filter); err != nil {
		return fail(err)
	}
	sub.LastUpdate, sub.LastError = existing.LastUpdate, existing.LastError
	if isEdit && (existing.URL != sub.URL || existing.UserAgent != sub.UserAgent || existing.HWID != sub.HWID) {
		sub.LastUpdate = "" // the server may answer differently: fetch now
	}
	if err := managerconfig.UpsertSubscription(configPath, sub); err != nil {
		return fail(err)
	}
	response := ok("id", sub.ID)
	if !sub.Enabled {
		return response
	}
	// A new or re-pointed subscription is fetched now; a filter/count edit is
	// re-applied to the stored candidates and re-probed without a refetch.
	if sub.LastUpdate == "" {
		if _, err := refreshSubscription(configPath, sub.ID); err != nil {
			response["warning"] = err.Error()
		}
		return response
	}
	state := managerconfig.LoadSubscriptionState(sub.ID)
	if state.Matched, err = subscription.Match(sub.Filter, state.Candidates); err == nil {
		_ = managerconfig.SaveSubscriptionState(sub.ID, state)
		probeAll(configPath, []string{sub.ID}, false)
	}
	return response
}

func refreshSubscriptionRPC(configPath string, raw json.RawMessage) map[string]any {
	params, err := decodeIDParams(raw)
	if err != nil {
		return fail(err)
	}
	state, err := refreshSubscription(configPath, params.ID)
	if err != nil {
		return fail(err)
	}
	return ok("total", len(state.Candidates), "matched", len(state.Matched), "selected", state.Selected, "skipped", state.Skipped)
}

func testSubscription(configPath string, raw json.RawMessage) map[string]any {
	params, err := decodeIDParams(raw)
	if err != nil {
		return fail(err)
	}
	probeAll(configPath, []string{params.ID}, false)
	return subscriptionOutbounds(configPath, raw)
}

func subscriptionOutbounds(configPath string, raw json.RawMessage) map[string]any {
	params, err := decodeIDParams(raw)
	if err != nil {
		return fail(err)
	}
	state := managerconfig.LoadSubscriptionState(params.ID)
	return ok("outbounds", candidateViews(state, managerconfig.LoadProbeResults()))
}

// filterPreview evaluates a filter against a subscription's stored candidates
// without saving it, so the UI can show what an expression would keep.
func filterPreview(configPath string, raw json.RawMessage) map[string]any {
	var params struct {
		ID     string `json:"id"`
		Filter string `json:"filter"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return fail(err)
	}
	state := managerconfig.LoadSubscriptionState(params.ID)
	matched, err := subscription.Match(params.Filter, state.Candidates)
	if err != nil {
		return fail(err)
	}
	names := map[string]string{}
	for _, c := range state.Candidates {
		names[c.ID] = c.Name
	}
	kept := make([]string, 0, len(matched))
	for _, id := range matched {
		kept = append(kept, names[id])
	}
	return ok("total", len(state.Candidates), "matched", len(matched), "names", kept)
}

// ---- outbound groups ----

type memberView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Available bool   `json:"available"`
	LatencyMS int    `json:"latency_ms,omitempty"`
}

type groupView struct {
	managerconfig.OutboundGroup
	MemberInfo []memberView `json:"member_info"`
	Available  bool         `json:"available"`
	Now        string       `json:"now,omitempty"`
	LatencyMS  int          `json:"latency_ms,omitempty"`
}

// displayName resolves a rendered tag to its UI name, including subscription
// candidates ("<subscription>.<hash>").
func displayName(cfg *managerconfig.Config, tag string) string {
	if sub, _, isCandidate := strings.Cut(tag, "."); isCandidate {
		for _, candidate := range managerconfig.LoadSubscriptionState(sub).Candidates {
			if candidate.ID == tag {
				return candidate.Name
			}
		}
		return tag
	}
	return targetName(cfg, tag)
}

func targetName(cfg *managerconfig.Config, id string) string {
	switch cfg.Kind(id) {
	case "group":
		return cfg.OutboundGroups[id].Name
	case "outbound":
		return cfg.Outbounds[id].Name
	case "subscription":
		return cfg.Subscriptions[id].Name
	}
	return id
}

func listOutboundGroups(configPath string, _ json.RawMessage) map[string]any {
	cfg, err := load(configPath)
	if err != nil {
		return fail(err)
	}
	plan := render.BuildPlan(*cfg)
	var proxies map[string]clashProxy
	if api := newClashAPI(*cfg); api != nil && managedRuntimePID(runtime.DefaultPaths) > 0 {
		proxies, _ = api.proxies()
	}
	results := managerconfig.LoadProbeResults()
	latency := func(id string) int {
		if delay := proxies[id].lastDelay(); delay > 0 {
			return delay
		}
		if result, found := results[id]; found && result.Health == "ok" {
			return result.LatencyMS
		}
		return 0
	}
	views := []groupView{}
	for _, id := range sortedIDs(cfg.OutboundGroups) {
		group := cfg.OutboundGroups[id]
		view := groupView{OutboundGroup: group, MemberInfo: []memberView{}, Available: plan.Rendered[id], Now: proxies[id].Now, LatencyMS: latency(id)}
		if view.LatencyMS == 0 && view.Now != "" {
			view.LatencyMS = latency(view.Now) // selectors keep no delay history of their own
		}
		for _, member := range group.Members {
			view.MemberInfo = append(view.MemberInfo, memberView{ID: member, Name: targetName(cfg, member), Kind: cfg.Kind(member), Available: plan.Rendered[member], LatencyMS: latency(member)})
		}
		views = append(views, view)
	}
	return ok("groups", views, "targets", targets(cfg, true), "warnings", plan.Warnings)
}

// targets lists everything an outbound group member or a route can point at.
func targets(cfg *managerconfig.Config, builtins bool) []map[string]string {
	var out []map[string]string
	if builtins {
		out = append(out, map[string]string{"id": "direct", "name": "direct", "kind": "builtin"}, map[string]string{"id": "block", "name": "block", "kind": "builtin"})
	}
	for _, id := range sortedIDs(cfg.OutboundGroups) {
		out = append(out, map[string]string{"id": id, "name": cfg.OutboundGroups[id].Name, "kind": "group"})
	}
	for _, id := range sortedIDs(cfg.Subscriptions) {
		out = append(out, map[string]string{"id": id, "name": cfg.Subscriptions[id].Name, "kind": "subscription"})
	}
	for _, id := range sortedIDs(cfg.Outbounds) {
		out = append(out, map[string]string{"id": id, "name": cfg.Outbounds[id].Name, "kind": "outbound"})
	}
	return out
}

func setOutboundGroup(configPath string, raw json.RawMessage) map[string]any {
	group, err := decode[managerconfig.OutboundGroup](raw, "group")
	if err != nil {
		return fail(err)
	}
	cfg, err := load(configPath)
	if err != nil {
		return fail(err)
	}
	if group.ID, err = newID(cfg, group.ID, group.Name, "group"); err != nil {
		return fail(err)
	}
	if err := managerconfig.UpsertOutboundGroup(configPath, group); err != nil {
		return fail(err)
	}
	return ok("id", group.ID)
}

// ---- rule groups ----

func listRuleGroups(configPath string, _ json.RawMessage) map[string]any {
	cfg, err := load(configPath)
	if err != nil {
		return fail(err)
	}
	groups := []managerconfig.RuleGroup{}
	for _, id := range sortedIDs(cfg.RuleGroups) {
		groups = append(groups, cfg.RuleGroups[id])
	}
	cached := map[string]bool{}
	for _, set := range cfg.RemoteRuleSets() {
		cached[set.URL] = fileExists(set.Path)
	}
	return ok("groups", groups, "rule_types", managerconfig.RuleTypes, "remote_cached", cached)
}

func setRuleGroup(configPath string, raw json.RawMessage) map[string]any {
	group, err := decode[managerconfig.RuleGroup](raw, "group")
	if err != nil {
		return fail(err)
	}
	cfg, err := load(configPath)
	if err != nil {
		return fail(err)
	}
	if group.ID == "" {
		if strings.TrimSpace(group.Name) == "" {
			return fail(fmt.Errorf("name is required"))
		}
		group.ID = managerconfig.SectionID(group.Name)
		if _, taken := cfg.RuleGroups[group.ID]; taken || cfg.Kind(group.ID) != "" {
			return fail(fmt.Errorf("name %q is already in use", group.Name))
		}
	}
	for _, rule := range group.Rules {
		if _, _, err := managerconfig.ParseRule(rule); err != nil {
			return fail(err)
		}
	}
	if err := managerconfig.UpsertRuleGroup(configPath, group); err != nil {
		return fail(err)
	}
	// Cache any new remote rule-set now so the reload uses the local copy.
	if cfg, err := load(configPath); err == nil {
		downloadRuleSets(*cfg, false)
	}
	return ok("id", group.ID)
}

func deleteRuleGroup(configPath string, raw json.RawMessage) map[string]any {
	params, err := decodeIDParams(raw)
	if err != nil {
		return fail(err)
	}
	if err := managerconfig.DeleteRuleGroup(configPath, params.ID); err != nil {
		return fail(err)
	}
	return ok("id", params.ID)
}

func refreshRuleSets(configPath string, _ json.RawMessage) map[string]any {
	cfg, err := load(configPath)
	if err != nil {
		return fail(err)
	}
	updated, errs := downloadRuleSets(*cfg, true)
	if updated {
		reloadIfRunning(configPath)
	}
	if len(errs) > 0 {
		return map[string]any{"ok": false, "updated": updated, "errors": errs}
	}
	return ok("updated", updated)
}

// ---- routing ----

func listRouting(configPath string, _ json.RawMessage) map[string]any {
	cfg, err := load(configPath)
	if err != nil {
		return fail(err)
	}
	groups := []map[string]string{}
	for _, id := range sortedIDs(cfg.RuleGroups) {
		groups = append(groups, map[string]string{"id": id, "name": cfg.RuleGroups[id].Name})
	}
	routes := cfg.Routes
	if routes == nil {
		routes = []managerconfig.Route{}
	}
	return ok("final", cfg.Manager.Final, "mode", cfg.Manager.RuntimeMode, "routes", routes,
		"rule_groups", groups, "targets", targets(cfg, true))
}

func setRouting(configPath string, raw json.RawMessage) map[string]any {
	var params struct {
		Final  string                `json:"final"`
		Routes []managerconfig.Route `json:"routes"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return fail(err)
	}
	if err := managerconfig.SetRouting(configPath, params.Final, params.Routes); err != nil {
		return fail(err)
	}
	return ok()
}

// ---- DNS ----

func listDNS(configPath string, _ json.RawMessage) map[string]any {
	cfg, err := load(configPath)
	if err != nil {
		return fail(err)
	}
	servers := []managerconfig.DNSServer{}
	for _, id := range sortedIDs(cfg.DNSServers) {
		servers = append(servers, cfg.DNSServers[id])
	}
	rules := []managerconfig.DNSRule{}
	for _, id := range sortedIDs(cfg.DNSRules) {
		rules = append(rules, cfg.DNSRules[id])
	}
	ruleGroups := []map[string]string{}
	for _, id := range sortedIDs(cfg.RuleGroups) {
		ruleGroups = append(ruleGroups, map[string]string{"id": id, "name": cfg.RuleGroups[id].Name})
	}
	dnsServers, dnsRules, dnsInbound := renderedDNSDebug(*cfg)
	return ok(
		"dns_final", cfg.Manager.DNSFinal,
		"servers", servers,
		"rules", rules,
		"rule_groups", ruleGroups,
		"detours", targets(cfg, false),
		"capture_enabled", cfg.Transparent.Active() && cfg.Transparent.DNSHijack,
		"warnings", dnsWarnings(*cfg),
		"rendered_servers", dnsServers,
		"rendered_rules", dnsRules,
		"active_dns_inbound", dnsInbound,
		"devices", discoverDevices(),
	)
}

func setDNSServer(configPath string, raw json.RawMessage) map[string]any {
	server, err := decode[managerconfig.DNSServer](raw, "server")
	if err != nil {
		return fail(err)
	}
	if server.ID == "" {
		server.ID = managerconfig.SectionID(server.Name)
	}
	if err := managerconfig.UpsertDNSServer(configPath, server); err != nil {
		return fail(err)
	}
	return ok("server", server.ID)
}

func setDNSRule(configPath string, raw json.RawMessage) map[string]any {
	rule, err := decode[managerconfig.DNSRule](raw, "rule")
	if err != nil {
		return fail(err)
	}
	if rule.ID == "" {
		rule.ID = managerconfig.SectionID(rule.Name)
	}
	if err := managerconfig.UpsertDNSRule(configPath, rule); err != nil {
		return fail(err)
	}
	return ok("rule", rule.ID)
}

func sortedIDs[T any](m map[string]T) []string {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
