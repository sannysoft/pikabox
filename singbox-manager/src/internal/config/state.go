package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var (
	// StateDir persists subscription candidates and their top-N selection, so
	// the rendered config is reproducible across reboots without a refetch.
	StateDir = "/etc/singbox-manager/state"
	// RunDir holds volatile delay results (tmpfs: no flash wear per probe).
	RunDir = "/var/run/singbox-manager"
)

// Candidate is one outbound fetched from a subscription that passed its filter.
type Candidate struct {
	// ID is the sing-box tag: "<subscription>.<hash>". UCI names never contain
	// '.', so it cannot collide with an outbound, subscription, or group.
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Outbound map[string]any `json:"outbound"`
}

type SubscriptionState struct {
	FetchedAt string `json:"fetched_at,omitempty"`
	// Candidates is everything fetched; Matched the IDs passing the filter
	// (kept apart so a filter edit re-applies without a refetch).
	Candidates []Candidate `json:"candidates"`
	Matched    []string    `json:"matched"`
	// Skipped summarises entries that could not become sing-box outbounds
	// (e.g. "19× unsupported transport xhttp"); they are ignored.
	Skipped string `json:"skipped,omitempty"`
	// Selected is the top-N alive candidate IDs, fastest first.
	Selected []string `json:"selected"`
}

// MatchedCandidates returns the candidates that passed the filter, in order.
func (s SubscriptionState) MatchedCandidates() []Candidate {
	matched := map[string]bool{}
	for _, id := range s.Matched {
		matched[id] = true
	}
	var out []Candidate
	for _, c := range s.Candidates {
		if matched[c.ID] {
			out = append(out, c)
		}
	}
	return out
}

// SelectedOutbounds returns the selected candidates' outbounds, in order.
func (s SubscriptionState) SelectedOutbounds() []map[string]any {
	byID := map[string]map[string]any{}
	for _, c := range s.Candidates {
		byID[c.ID] = c.Outbound
	}
	var out []map[string]any
	for _, id := range s.Selected {
		if ob, ok := byID[id]; ok {
			out = append(out, ob)
		}
	}
	return out
}

// ProbeResult is the latest delay test of one outbound or candidate.
type ProbeResult struct {
	Health    string `json:"health"` // ok | down | error
	LatencyMS int    `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
	CheckedAt string `json:"checked_at"`
}

// DeviceID returns this router's HWID, generated once and kept in StateDir so
// it survives reboots and upgrades (a new one would count as a new device).
func DeviceID() (string, error) {
	path := filepath.Join(StateDir, "hwid")
	if data, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(data))) > 0 {
		return strings.TrimSpace(string(data)), nil
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	id := hex.EncodeToString(raw)
	return id, writeAtomic(path, []byte(id+"\n"), 0600)
}

func LoadSubscriptionState(id string) SubscriptionState {
	var state SubscriptionState
	_ = readJSON(filepath.Join(StateDir, "subscription-"+id+".json"), &state)
	return state
}

func SaveSubscriptionState(id string, state SubscriptionState) error {
	return writeJSON(filepath.Join(StateDir, "subscription-"+id+".json"), state)
}

func DeleteSubscriptionState(id string) {
	_ = os.Remove(filepath.Join(StateDir, "subscription-"+id+".json"))
}

func LoadProbeResults() map[string]ProbeResult {
	results := map[string]ProbeResult{}
	_ = readJSON(filepath.Join(RunDir, "probe.json"), &results)
	return results
}

func SaveProbeResults(results map[string]ProbeResult) error {
	return writeJSON(filepath.Join(RunDir, "probe.json"), results)
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, v)
}

func writeJSON(path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return writeAtomic(path, data, 0600)
}
