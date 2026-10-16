package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	managerconfig "github.com/sannysoft/pikabox/internal/config"
	"github.com/sannysoft/pikabox/internal/runtime"
)

var linePrefix = regexp.MustCompile(`line \d+: `)

// BackupDir keeps the configs replaced by an import, newest kept first.
var BackupDir = "/etc/pikabox/backup"

const maxBackups = 10

func settingsRPC(configPath string, _ json.RawMessage) map[string]any {
	cfg, err := load(configPath)
	if err != nil {
		return fail(err)
	}
	m := cfg.Manager
	return ok(
		"settings", map[string]any{
			"log_level":      m.LogLevel,
			"probe_interval": m.ProbeInterval,
			"test_url":       m.TestURL,
			"bulk_test_url":  m.BulkTestURL,
			"mixed_listen":   m.MixedListen,
			"mixed_port":     m.MixedPort,
			"tproxy_port":    m.TProxyPort,
			"dns_port":       m.DNSPort,
			"api_listen":     m.APIListen,
			"sing_box_bin":   m.SingBoxBinary,
		},
		"versions", map[string]string{
			"sing_box": singBoxVersion(m.SingBoxBinary),
			"openwrt":  openwrtRelease(),
		},
		"backups", listBackups(),
	)
}

// openwrtRelease reads DISTRIB_DESCRIPTION ("OpenWrt 25.12.2 r33304-…").
func openwrtRelease() string {
	for _, line := range strings.Split(readFile("/etc/openwrt_release"), "\n") {
		if value, found := strings.CutPrefix(line, "DISTRIB_DESCRIPTION="); found {
			return strings.Trim(value, "'\"")
		}
	}
	return strings.TrimSpace(readFile("/etc/openwrt_version"))
}

func readFile(path string) string {
	data, _ := os.ReadFile(path)
	return string(data)
}

func singBoxVersion(binary string) string {
	out, err := exec.Command(binary, "version").Output()
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(out), "\n")
	return strings.TrimSpace(strings.TrimPrefix(line, "sing-box version "))
}

// settingsPayload is every app-wide option the Settings page edits; a nil
// field is left unchanged.
type settingsPayload struct {
	LogLevel      *string `json:"log_level"`
	ProbeInterval *string `json:"probe_interval"`
	TestURL       *string `json:"test_url"`
	BulkTestURL   *string `json:"bulk_test_url"`
	DNSFinal      *string `json:"dns_final"`
	MixedListen   *string `json:"mixed_listen"`
	MixedPort     *string `json:"mixed_port"`
	TProxyPort    *string `json:"tproxy_port"`
	DNSPort       *string `json:"dns_port"`
	APIListen     *string `json:"api_listen"`
	SingBoxBinary *string `json:"sing_box_bin"`
}

func (p *settingsPayload) UnmarshalJSON(data []byte) error {
	// Ports may arrive as numbers from the form; keep them as strings.
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	str := func(key string) *string {
		value, present := raw[key]
		if !present || value == nil {
			return nil
		}
		var s string
		switch v := value.(type) {
		case float64:
			s = strconv.Itoa(int(v))
		default:
			s = fmt.Sprint(v)
		}
		s = strings.TrimSpace(s)
		return &s
	}
	*p = settingsPayload{
		LogLevel: str("log_level"), ProbeInterval: str("probe_interval"), TestURL: str("test_url"), BulkTestURL: str("bulk_test_url"),
		DNSFinal: str("dns_final"), MixedListen: str("mixed_listen"), MixedPort: str("mixed_port"),
		TProxyPort: str("tproxy_port"), DNSPort: str("dns_port"), APIListen: str("api_listen"),
		SingBoxBinary: str("sing_box_bin"),
	}
	return nil
}

func setSettings(configPath string, raw json.RawMessage) map[string]any {
	settings, err := decode[settingsPayload](raw, "settings")
	if err != nil {
		return fail(err)
	}
	if v := settings.ProbeInterval; v != nil && *v != "" {
		if d, err := time.ParseDuration(*v); err != nil || d < time.Minute {
			return fail(fmt.Errorf("probe interval must be a duration of at least 1m (e.g. 10m), got %q", *v))
		}
	}
	if v := settings.TestURL; v != nil && *v != "" && !strings.HasPrefix(*v, "http://") && !strings.HasPrefix(*v, "https://") {
		return fail(fmt.Errorf("test URL must start with http:// or https://"))
	}
	if v := settings.BulkTestURL; v != nil && *v != "" && *v != "off" && !strings.HasPrefix(*v, "http://") && !strings.HasPrefix(*v, "https://") {
		return fail(fmt.Errorf("bulk test URL must start with http:// or https:// (or be off)"))
	}
	for key, value := range map[string]*string{
		"log_level": settings.LogLevel, "probe_interval": settings.ProbeInterval, "test_url": settings.TestURL, "bulk_test_url": settings.BulkTestURL,
		"dns_final": settings.DNSFinal, "mixed_listen": settings.MixedListen, "mixed_port": settings.MixedPort,
		"tproxy_port": settings.TProxyPort, "dns_port": settings.DNSPort, "api_listen": settings.APIListen,
		"sing_box_bin": settings.SingBoxBinary,
	} {
		if value == nil {
			continue
		}
		// Each write is validated as a whole config, so a bad port or log
		// level is rejected with the parser's own message.
		if err := managerconfig.SetManagerOption(configPath, key, *value); err != nil {
			// Drop the parser's "line N:" — the user edited a field, not a line.
			return fail(fmt.Errorf("%s", linePrefix.ReplaceAllString(err.Error(), "")))
		}
	}
	return ok()
}

// ---- export / import ----

func exportConfigRPC(configPath string, _ json.RawMessage) map[string]any {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fail(err)
	}
	host, _ := os.Hostname()
	name := fmt.Sprintf("pikabox-%s-%s.uci", firstNonEmptyString(host, "openwrt"), time.Now().Format("20060102-1504"))
	return ok("text", string(data), "filename", name)
}

type importSummary struct {
	Outbounds      int `json:"outbounds"`
	Subscriptions  int `json:"subscriptions"`
	OutboundGroups int `json:"outbound_groups"`
	RuleGroups     int `json:"rule_groups"`
	Routes         int `json:"routes"`
	DNSServers     int `json:"dns_servers"`
	DNSRules       int `json:"dns_rules"`
	Devices        int `json:"devices"`
}

func summarize(cfg *managerconfig.Config) importSummary {
	return importSummary{
		Outbounds: len(cfg.Outbounds), Subscriptions: len(cfg.Subscriptions), OutboundGroups: len(cfg.OutboundGroups),
		RuleGroups: len(cfg.RuleGroups), Routes: len(cfg.Routes), DNSServers: len(cfg.DNSServers),
		DNSRules: len(cfg.DNSRules), Devices: len(cfg.Transparent.Devices),
	}
}

// importConfigRPC validates an exported config and, unless dry_run is set,
// replaces the current one with it (keeping a backup), then reloads and
// fetches subscriptions that have no state on this router yet.
func importConfigRPC(configPath string, raw json.RawMessage) map[string]any {
	var params struct {
		Text   string `json:"text"`
		DryRun bool   `json:"dry_run"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return fail(err)
	}
	if strings.TrimSpace(params.Text) == "" {
		return fail(fmt.Errorf("the file is empty"))
	}
	tmp, err := os.CreateTemp(filepath.Dir(configPath), ".import-")
	if err != nil {
		return fail(err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(params.Text); err != nil {
		tmp.Close()
		return fail(err)
	}
	tmp.Close()
	cfg, err := managerconfig.Load(tmp.Name())
	if err != nil {
		return fail(fmt.Errorf("not a valid pikabox config: %w", err))
	}
	current, _ := load(configPath)
	response := ok("summary", summarize(cfg))
	if current != nil {
		response["current"] = summarize(current)
	}
	if params.DryRun {
		return response
	}

	backup, err := backupConfig(configPath)
	if err != nil {
		return fail(fmt.Errorf("backup failed, nothing imported: %w", err))
	}
	if err := os.Rename(tmp.Name(), configPath); err != nil {
		return fail(err)
	}
	// Keep this router's runtime posture: importing a config from another
	// one must not silently enable or pause the proxy here.
	if current != nil {
		_ = managerconfig.SetManagerOption(configPath, "enabled", boolOption(current.Manager.Enabled))
		_ = managerconfig.SetManagerOption(configPath, "paused", boolOption(current.Manager.Paused))
	}
	recordEvent("reload", "config imported (previous saved as %s)", filepath.Base(backup))
	go func() {
		for id, sub := range cfg.Subscriptions {
			if sub.Enabled && len(managerconfig.LoadSubscriptionState(id).Candidates) == 0 {
				if _, err := refreshSubscription(configPath, id); err != nil {
					recordEvent("error", "subscription %s refresh after import failed: %v", sub.Name, err)
				}
			}
		}
		reloadIfRunning(configPath)
	}()
	response["backup"] = filepath.Base(backup)
	response["reloaded"] = managedRuntimePID(runtime.DefaultPaths) > 0
	return response
}

func boolOption(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

func backupConfig(configPath string) (string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(BackupDir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(BackupDir, "config-"+time.Now().Format("20060102-150405")+".uci")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return "", err
	}
	backups := listBackups()
	for _, old := range backups[min(len(backups), maxBackups):] {
		_ = os.Remove(filepath.Join(BackupDir, old["name"]))
	}
	return path, nil
}

// listBackups returns saved configs, newest first.
func listBackups() []map[string]string {
	entries, _ := os.ReadDir(BackupDir)
	var out []map[string]string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".uci") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		out = append(out, map[string]string{"name": entry.Name(), "time": info.ModTime().UTC().Format(time.RFC3339), "size": strconv.FormatInt(info.Size(), 10)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["name"] > out[j]["name"] })
	if out == nil {
		out = []map[string]string{}
	}
	return out
}

func backupReadRPC(_ string, raw json.RawMessage) map[string]any {
	var params struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &params); err != nil || params.Name == "" || params.Name != filepath.Base(params.Name) {
		return fail(fmt.Errorf("a backup name is required"))
	}
	data, err := os.ReadFile(filepath.Join(BackupDir, params.Name))
	if err != nil {
		return fail(err)
	}
	return ok("text", string(data), "filename", params.Name)
}
