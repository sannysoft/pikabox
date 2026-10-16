package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	managerconfig "github.com/sannysoft/pikabox/internal/config"
	"github.com/sannysoft/pikabox/internal/health"
	"github.com/sannysoft/pikabox/internal/render"
	"github.com/sannysoft/pikabox/internal/ruleset"
	"github.com/sannysoft/pikabox/internal/runtime"
	"github.com/sannysoft/pikabox/internal/subscription"
)

// noSubscriptions tells probeAll to skip subscriptions (nil means all).
var noSubscriptions = []string{}

// probeMu serialises probe runs: each stands up its own sing-box instance and a
// scheduled run overlapping a UI-triggered one only doubles the router's load.
var probeMu sync.Mutex

// refreshSubscription fetches a subscription, applies its filter, stores the
// candidates, then probes them and selects the fastest Count. The fetch goes
// direct first and falls back to the running tunnel (subscription hosts are
// often blocked on the very networks this runs on).
func refreshSubscription(configPath string, id string) (managerconfig.SubscriptionState, error) {
	cfg, err := managerconfig.Load(configPath)
	if err != nil {
		return managerconfig.SubscriptionState{}, err
	}
	sub, ok := cfg.Subscriptions[id]
	if !ok {
		return managerconfig.SubscriptionState{}, fmt.Errorf("subscription %q not found", id)
	}
	fail := func(err error) (managerconfig.SubscriptionState, error) {
		recordEvent("error", "subscription %s refresh failed: %v", sub.Name, err)
		_ = managerconfig.MarkSubscription(configPath, id, "", err.Error())
		return managerconfig.SubscriptionState{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	headers, err := fetchHeaders(sub)
	if err != nil {
		return fail(err)
	}
	data, err := subscription.Fetch(ctx, sub.URL, "", headers)
	if err != nil {
		if proxy := localProxyAddr(*cfg); proxy != "" {
			data, err = subscription.Fetch(ctx, sub.URL, proxy, headers)
		}
		if err != nil {
			return fail(err)
		}
	}
	items, skipped, err := subscription.ParseOutbounds(data, sub.Format)
	if err != nil {
		return fail(err)
	}
	candidates := subscription.Candidates(id, items)
	matched, err := subscription.Match(sub.Filter, candidates)
	if err != nil {
		return fail(err)
	}

	previous := managerconfig.LoadSubscriptionState(id)
	state := managerconfig.SubscriptionState{
		FetchedAt:  time.Now().UTC().Format(time.RFC3339),
		Candidates: candidates,
		Matched:    matched,
		Skipped:    skipped,
		Selected:   previous.Selected,
	}
	if err := managerconfig.SaveSubscriptionState(id, state); err != nil {
		return fail(err)
	}
	if err := managerconfig.MarkSubscription(configPath, id, state.FetchedAt, ""); err != nil {
		return state, err
	}
	probeAll(configPath, []string{id}, false)
	return managerconfig.LoadSubscriptionState(id), nil
}

// fetchHeaders builds the per-subscription request headers: a custom
// User-Agent and, when enabled, the device headers HWID-limited panels
// (Remnawave and similar) expect.
func fetchHeaders(sub managerconfig.Subscription) (map[string]string, error) {
	headers := map[string]string{}
	if sub.UserAgent != "" {
		headers["User-Agent"] = sub.UserAgent
	}
	if sub.HWID {
		id, err := managerconfig.DeviceID()
		if err != nil {
			return nil, fmt.Errorf("hwid: %w", err)
		}
		headers["x-hwid"] = id
		headers["x-device-os"] = "Linux"
		headers["x-ver-os"] = openwrtVersion()
		headers["x-device-model"] = "OpenWrt router"
	}
	return headers, nil
}

func openwrtVersion() string {
	data, err := os.ReadFile("/etc/openwrt_version")
	if v := strings.TrimSpace(string(data)); err == nil && v != "" {
		return v
	}
	return "OpenWrt"
}

// probeAll delay-tests outbounds and subscription candidates in one probe
// instance, records the results, and re-selects each probed subscription's
// fastest Count candidates. subs limits which subscriptions are probed (nil =
// all enabled); withOutbounds adds every enabled standalone outbound. sing-box
// is reloaded only when some selection actually changed.
func probeAll(configPath string, subs []string, withOutbounds bool) map[string]managerconfig.ProbeResult {
	probeMu.Lock()
	defer probeMu.Unlock()

	cfg, err := managerconfig.Load(configPath)
	if err != nil {
		log.Printf("probe skipped: %v", err)
		return nil
	}
	targets := map[string]map[string]any{}
	if withOutbounds {
		for id, outbound := range cfg.Outbounds {
			if !outbound.Enabled {
				continue
			}
			if item, err := subscription.ParseSource(outbound.Source); err == nil {
				targets[id] = item.Outbound
			}
		}
	}
	if subs == nil {
		for id, sub := range cfg.Subscriptions {
			if sub.Enabled {
				subs = append(subs, id)
			}
		}
	}
	states := map[string]managerconfig.SubscriptionState{}
	for _, id := range subs {
		state := managerconfig.LoadSubscriptionState(id)
		states[id] = state
		for _, candidate := range state.MatchedCandidates() {
			targets[candidate.ID] = candidate.Outbound
		}
	}

	opts := health.ProbeOptions{Binary: cfg.Manager.SingBoxBinary, TestURL: cfg.Manager.TestURL}
	results := health.ProbeOutbounds(context.Background(), opts, targets)

	stored := managerconfig.LoadProbeResults()
	now := time.Now().UTC().Format(time.RFC3339)
	for tag, result := range results {
		stored[tag] = managerconfig.ProbeResult{Health: result.Health, LatencyMS: result.LatencyMS, Error: result.Error, CheckedAt: now}
		recordProbe(tag, stored[tag])
	}
	// Forget results for candidates no longer matched by any subscription.
	known := map[string]bool{}
	for id := range cfg.Outbounds {
		known[id] = true
	}
	for id := range cfg.Subscriptions {
		for _, candidate := range managerconfig.LoadSubscriptionState(id).Matched {
			known[candidate] = true
		}
	}
	for tag := range stored {
		if !known[tag] {
			delete(stored, tag)
		}
	}
	if err := managerconfig.SaveProbeResults(stored); err != nil {
		log.Printf("save probe results: %v", err)
	}

	changed := false
	for id, state := range states {
		selected := selectFastest(state.MatchedCandidates(), stored, cfg.Subscriptions[id].Count)
		if sameSet(selected, state.Selected) {
			continue
		}
		state.Selected = selected
		if err := managerconfig.SaveSubscriptionState(id, state); err != nil {
			log.Printf("save subscription %s state: %v", id, err)
			continue
		}
		changed = true
		names := make([]string, 0, len(selected))
		for _, tag := range selected {
			names = append(names, displayName(cfg, tag))
		}
		recordEvent("select", "%s now uses %s", cfg.Subscriptions[id].Name, strings.Join(names, ", "))
	}
	// One history point per subscription (its best alive candidate).
	for id, state := range states {
		best := managerconfig.ProbeResult{Health: "down", CheckedAt: now}
		for _, tag := range state.Matched {
			if r := stored[tag]; r.Health == "ok" && (best.Health != "ok" || r.LatencyMS < best.LatencyMS) {
				best = r
			}
		}
		recordProbe(id, best)
	}
	if changed {
		reloadIfRunning(configPath)
	}
	recordGroupHistory(*cfg)
	return stored
}

// selectFastest returns up to count alive candidate IDs, fastest first
// (count 0 = every alive candidate).
// rdx: no hysteresis beyond set equality; add a latency margin if selections flap.
func selectFastest(candidates []managerconfig.Candidate, results map[string]managerconfig.ProbeResult, count int) []string {
	var alive []managerconfig.Candidate
	for _, candidate := range candidates {
		if results[candidate.ID].Health == "ok" {
			alive = append(alive, candidate)
		}
	}
	sort.SliceStable(alive, func(i, j int) bool {
		return results[alive[i].ID].LatencyMS < results[alive[j].ID].LatencyMS
	})
	if count > 0 && len(alive) > count {
		alive = alive[:count]
	}
	ids := make([]string, len(alive))
	for i, candidate := range alive {
		ids[i] = candidate.ID
	}
	return ids
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]bool{}
	for _, v := range a {
		seen[v] = true
	}
	for _, v := range b {
		if !seen[v] {
			return false
		}
	}
	return true
}

func reloadIfRunning(configPath string) {
	if managedRuntimePID(runtime.DefaultPaths) == 0 {
		return
	}
	cfg, err := managerconfig.Load(configPath)
	if err != nil {
		log.Printf("reload skipped: %v", err)
		return
	}
	if _, err := runtime.Control(*cfg, runtime.ActionReload, runtime.DefaultPaths, render.Render); err != nil {
		recordEvent("error", "reload failed: %v", err)
	}
}

// downloadRuleSets caches every remote rule-set referenced by rule groups that
// is missing or stale, through the tunnel when it is up. It returns whether
// anything new landed (the config then switches that set to its local copy).
func downloadRuleSets(cfg managerconfig.Config, force bool) (bool, []string) {
	proxy := localProxyAddr(cfg)
	updated := false
	var errs []string
	for _, set := range cfg.RemoteRuleSets() {
		if !force && !ruleset.Due(set, time.Now()) {
			continue
		}
		_, err := ruleset.Download(context.Background(), set, proxy)
		if err != nil && proxy != "" {
			// The tunnel may be mid-reload (a save just restarted sing-box) or
			// the exit may block the host; a direct fetch often still works.
			_, err = ruleset.Download(context.Background(), set, "")
		}
		if err != nil {
			setRuleSetError(set.URL, err)
			errs = append(errs, fmt.Sprintf("%s: %v", set.URL, err))
			continue
		}
		setRuleSetError(set.URL, nil)
		updated = true
	}
	return updated, errs
}

// clashAPI talks to the running sing-box's Clash API controller.
type clashAPI struct {
	base   string
	client *http.Client
}

func newClashAPI(cfg managerconfig.Config) *clashAPI {
	if cfg.Manager.APIListen == "" {
		return nil
	}
	return &clashAPI{base: "http://" + cfg.Manager.APIListen, client: &http.Client{Timeout: 15 * time.Second}}
}

func (api *clashAPI) delay(tag string, testURL string) (int, error) {
	endpoint := fmt.Sprintf("%s/proxies/%s/delay?timeout=5000&url=%s", api.base, url.PathEscape(tag), url.QueryEscape(testURL))
	resp, err := api.client.Get(endpoint)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	var body struct {
		Delay   int    `json:"delay"`
		Message string `json:"message"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if resp.StatusCode != http.StatusOK || body.Delay <= 0 {
		return 0, fmt.Errorf("delay test failed: %s", firstNonEmptyString(body.Message, resp.Status))
	}
	return body.Delay, nil
}

func (api *clashAPI) proxies() (map[string]clashProxy, error) {
	resp, err := api.client.Get(api.base + "/proxies")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var body struct {
		Proxies map[string]clashProxy `json:"proxies"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	return body.Proxies, nil
}

func (api *clashAPI) choose(group string, member string) error {
	payload, _ := json.Marshal(map[string]string{"name": member})
	req, _ := http.NewRequest(http.MethodPut, api.base+"/proxies/"+url.PathEscape(group), bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := api.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("select %s in %s: %s", member, group, resp.Status)
	}
	return nil
}

type clashProxy struct {
	Type    string `json:"type"`
	Now     string `json:"now"`
	History []struct {
		Delay int `json:"delay"`
	} `json:"history"`
}

func (p clashProxy) lastDelay() int {
	if len(p.History) == 0 {
		return 0
	}
	return p.History[len(p.History)-1].Delay
}

// enforceFallback points every fallback group at its first alive member, in
// written order. sing-box has no ordered-fallback outbound, so groups render as
// selectors and this controller drives them through the Clash API.
func enforceFallback(cfg managerconfig.Config) {
	api := newClashAPI(cfg)
	if api == nil || managedRuntimePID(runtime.DefaultPaths) == 0 {
		return
	}
	plan := render.BuildPlan(cfg)
	if len(plan.Fallback) == 0 {
		return
	}
	proxies, err := api.proxies()
	if err != nil {
		return
	}
	testURL := firstNonEmptyString(cfg.Manager.TestURL, health.DefaultTestURL)
	for group, members := range plan.Fallback {
		target := members[len(members)-1] // nothing alive: park on the last
		for _, member := range members {
			if member == "direct" || member == "block" {
				target = member
				break
			}
			if _, err := api.delay(member, testURL); err == nil {
				target = member
				break
			}
		}
		if proxies[group].Now == target {
			continue
		}
		if err := api.choose(group, target); err != nil {
			recordEvent("error", "fallback %s: %v", group, err)
			continue
		}
		recordEvent("switch", "%s switched to %s (was %s)", cfg.OutboundGroups[group].Name, displayName(&cfg, target), displayName(&cfg, firstNonEmptyString(proxies[group].Now, "—")))
	}
}

// recordGroupHistory adds one point per group: the live delay of the member
// it currently uses.
func recordGroupHistory(cfg managerconfig.Config) {
	api := newClashAPI(cfg)
	if api == nil || managedRuntimePID(runtime.DefaultPaths) == 0 {
		return
	}
	proxies, err := api.proxies()
	if err != nil {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for id := range cfg.OutboundGroups {
		result := managerconfig.ProbeResult{Health: "down", CheckedAt: now}
		if delay := proxies[proxies[id].Now].lastDelay(); delay > 0 {
			result = managerconfig.ProbeResult{Health: "ok", LatencyMS: delay, CheckedAt: now}
		}
		recordProbe(id, result)
	}
}

func startFallbackController(configPath string) {
	go func() {
		for {
			time.Sleep(30 * time.Second)
			if cfg, err := managerconfig.Load(configPath); err == nil && cfg.Manager.Active() {
				enforceFallback(*cfg)
			}
		}
	}()
}

// startProbeScheduler re-tests everything every probe_interval, refetches due
// subscriptions, and keeps remote rule-sets cached.
func startProbeScheduler(configPath string) {
	go func() {
		time.Sleep(15 * time.Second)
		for {
			cfg, err := managerconfig.Load(configPath)
			interval := time.Hour
			if err == nil {
				interval = durationOr(cfg.Manager.ProbeInterval, 10*time.Minute, time.Minute)
				if cfg.Manager.Active() {
					now := time.Now().UTC()
					for id, sub := range cfg.Subscriptions {
						if subscription.Due(sub, now) {
							if _, err := refreshSubscription(configPath, id); err != nil {
								log.Printf("subscription %s refresh failed: %v", id, err)
							}
						}
					}
					probeAll(configPath, nil, true)
					if updated, errs := downloadRuleSets(*cfg, false); updated {
						reloadIfRunning(configPath)
					} else if len(errs) > 0 {
						log.Printf("rule-set download: %v", errs)
					}
				}
			}
			time.Sleep(interval)
		}
	}()
}

func durationOr(value string, fallback, min time.Duration) time.Duration {
	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 {
		d = fallback
	}
	if d < min {
		d = min
	}
	return d
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
