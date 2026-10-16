package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	managerconfig "github.com/openwrt-singbox/singbox-manager/internal/config"
	"github.com/openwrt-singbox/singbox-manager/internal/firewall"
	"github.com/openwrt-singbox/singbox-manager/internal/health"
	"github.com/openwrt-singbox/singbox-manager/internal/render"
	"github.com/openwrt-singbox/singbox-manager/internal/runtime"
)

const (
	defaultConfigPath = "/etc/config/singbox-manager"
	defaultSocketPath = managerconfig.DefaultSocketPath
)

type Status struct {
	Daemon         bool `json:"daemon"`
	ManagerEnabled bool `json:"manager_enabled"`
	ManagerPaused  bool `json:"manager_paused"`
	Running        bool `json:"running"`
	SingBoxPID     int  `json:"sing_box_pid"`
	// Final is the effective route.final target; Chain the groups traversed
	// from it (via each group's live selection) to SelectedOutbound.
	Final            string     `json:"final"`
	ActiveGroup      string     `json:"active_group"`
	Chain            []string   `json:"chain"`
	ChainNames       []string   `json:"chain_names"`
	SelectedOutbound string     `json:"selected_outbound"`
	SelectedName     string     `json:"selected_name"`
	Hops             []chainHop `json:"hops"`
	Events           []Event    `json:"events"`
	RuntimeMode      string     `json:"runtime_mode"`
	Strategy         string     `json:"strategy"`
	Health           string     `json:"health"`
	LatencyMS        int        `json:"latency_ms"`
	MemoryKB         uint64     `json:"memory_kb"`
	Warnings         []string   `json:"warnings"`
	Connections      int        `json:"connections"`
	RxBytes          uint64     `json:"rx_bytes"`
	TxBytes          uint64     `json:"tx_bytes"`
	TProxyEnabled    bool       `json:"tproxy_enabled"`
	DNSHijack        bool       `json:"dns_hijack"`
	KillSwitch       bool       `json:"kill_switch"`
	NftablesInclude  string     `json:"nftables_include"`
	TUNEnabled       bool       `json:"tun_enabled"`
	// StaleDataPath is set when sing-box is not running but its firewall/policy
	// routing is still installed, which leaves in-scope devices without internet.
	StaleDataPath bool `json:"stale_datapath"`
}

type RPCRequest struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type RPCError struct {
	Error string `json:"error"`
}

type IDParams struct {
	ID string `json:"id"`
}

type EnabledParams struct {
	Enabled bool `json:"enabled"`
}

type LatencyParams struct {
	URL string `json:"url"`
}

type DNSTestParams struct {
	Server string `json:"server"`
	Domain string `json:"domain"`
}

type TransparentPayload struct {
	DefaultMode  string          `json:"default_mode"`
	LANIfnames   []string        `json:"lan_ifnames"`
	BypassSubnet []string        `json:"bypass_subnet"`
	Devices      []DevicePayload `json:"devices"`
	DNSHijack    bool            `json:"dns_hijack"`
	KillSwitch   bool            `json:"kill_switch"`
}

type DevicePayload struct {
	ID        string `json:"id"`
	Enabled   bool   `json:"enabled"`
	Name      string `json:"name"`
	MAC       string `json:"mac"`
	IPv4      string `json:"ipv4"`
	IPv6      string `json:"ipv6"`
	Mode      string `json:"mode"`
	BypassUDP bool   `json:"bypass_udp"`
	Group     string `json:"group"`
}

type TUNPayload struct {
	Enabled      bool   `json:"enabled"`
	AutoRoute    bool   `json:"auto_route"`
	AutoRedirect bool   `json:"auto_redirect"`
	Inet4Address string `json:"inet4_address"`
	Inet6Address string `json:"inet6_address"`
}

type LogsParams struct {
	Lines int `json:"lines"`
}

type RuntimeStats struct {
	Connections int    `json:"connections"`
	RxBytes     uint64 `json:"rx_bytes"`
	TxBytes     uint64 `json:"tx_bytes"`
}

type Device struct {
	IP     string `json:"ip"`
	MAC    string `json:"mac"`
	Name   string `json:"name"`
	Source string `json:"source"`
}

func main() {
	if filepath.Base(os.Args[0]) == "singbox.manager" {
		runRPCD(os.Args[1:])
		return
	}

	if len(os.Args) < 2 {
		runServe(os.Args[1:])
		return
	}

	switch os.Args[1] {
	case "serve":
		runServe(os.Args[2:])
	case "cleanup":
		runCleanup(os.Args[2:])
	case "rpcd":
		runRPCD(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(2)
	}
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "UCI config path")
	socketPath := fs.String("socket", "", "Unix socket path")
	_ = fs.Parse(args)

	socket := configuredSocket(*configPath)
	if *socketPath != "" {
		socket = *socketPath
	}

	if err := os.MkdirAll(filepath.Dir(socket), 0755); err != nil {
		log.Fatalf("failed to create socket directory: %v", err)
	}
	_ = os.Remove(socket)

	listener, err := net.Listen("unix", socket)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", socket, err)
	}
	defer listener.Close()
	if err := os.Chmod(socket, 0660); err != nil {
		log.Printf("failed to chmod socket: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", func(w http.ResponseWriter, r *http.Request) {
		handleRPC(w, r, *configPath)
	})

	startProbeScheduler(*configPath)
	startFallbackController(*configPath)
	startRuntimeSupervisor(*configPath)
	startSignalCleanup(*configPath)

	log.Printf("singbox-managerd listening on %s", socket)
	if err := http.Serve(listener, mux); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server stopped: %v", err)
	}
}

func runCleanup(args []string) {
	fs := flag.NewFlagSet("cleanup", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "UCI config path")
	_ = fs.Parse(args)

	if err := cleanupRuntime(*configPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func startSignalCleanup(configPath string) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-signals
		if err := cleanupRuntime(configPath); err != nil {
			log.Printf("runtime cleanup after %s failed: %v", sig, err)
		}
		os.Exit(0)
	}()
}

// cleanupResult tears down sing-box and every piece of transparent-proxy
// plumbing (nft include, policy routing) without touching the saved config.
func cleanupResult(configPath string) map[string]any {
	if err := cleanupRuntime(configPath); err != nil {
		return validationResult(false, runtime.Result{}, err)
	}
	return map[string]any{"ok": true, "message": "proxy rules removed"}
}

func cleanupRuntime(configPath string) error {
	cfg, err := managerconfig.Load(configPath)
	if err != nil {
		defaultCfg := managerconfig.DefaultConfig()
		cfg = &defaultCfg
	}
	_, err = runtime.Control(*cfg, runtime.ActionTeardown, runtime.DefaultPaths, render.Render)
	return err
}

func startRuntimeSupervisor(configPath string) {
	cfg, err := managerconfig.Load(configPath)
	if err != nil {
		log.Printf("runtime supervision skipped: %v", err)
		return
	}
	if managedRuntimePID(runtime.DefaultPaths) > 0 {
		runtime.Supervise(*cfg, runtime.DefaultPaths, render.Render)
		log.Printf("runtime supervision attached to existing sing-box process")
		return
	}
	if !cfg.Manager.Active() {
		// Management-only: keep serving the UI/RPC but never bring the proxy or
		// its firewall rules up on our own. Honours singbox-manager.main.paused
		// so a reboot does not silently divert the admin's LAN.
		log.Printf("runtime supervision idle (enabled=%v paused=%v); serving management RPC only", cfg.Manager.Enabled, cfg.Manager.Paused)
		return
	}
	// Cold start: after a reboot the pid file lives on tmpfs and nothing is
	// running, so the manager has to bring sing-box up itself. The supervisor
	// retries on a timer, which also covers the boot window where the network
	// or fw4 is not ready yet.
	runtime.Supervise(*cfg, runtime.DefaultPaths, render.Render)
	log.Printf("manager enabled with no running sing-box; starting runtime")
}

func runRPCD(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: singbox-managerd rpcd <list|call>")
		os.Exit(2)
	}

	switch args[0] {
	case "list":
		signatures := map[string]map[string]any{}
		for name, method := range methods {
			params := method.params
			if params == nil {
				params = map[string]any{}
			}
			signatures[name] = params
		}
		writeJSON(signatures)
	case "call":
		if len(args) < 2 {
			writeJSON(RPCError{Error: "missing method"})
			os.Exit(1)
		}
		callDaemon(args[1], os.Stdin)
	default:
		writeJSON(RPCError{Error: "unsupported rpcd command"})
		os.Exit(1)
	}
}

// configuredSocket returns the daemon socket path from UCI, or the default.
func configuredSocket(configPath string) string {
	if cfg, err := managerconfig.LoadUnvalidated(configPath); err == nil && cfg.Manager.SocketPath != "" {
		return cfg.Manager.SocketPath
	}
	return defaultSocketPath
}

func callDaemon(method string, input io.Reader) {
	var params json.RawMessage
	body, _ := io.ReadAll(input)
	if len(strings.TrimSpace(string(body))) > 0 {
		params = body
	}

	timeout := quick
	if m, found := methods[method]; found {
		timeout = m.timeout
	}
	socket := configuredSocket(defaultConfigPath)
	reqBody, _ := json.Marshal(RPCRequest{Method: method, Params: params})
	client := http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				dialer := net.Dialer{}
				return dialer.DialContext(ctx, "unix", socket)
			},
		},
	}

	resp, err := client.Post("http://unix/rpc", "application/json", strings.NewReader(string(reqBody)))
	if err != nil {
		if method == "status" {
			status := collectStatus(defaultConfigPath)
			status.Daemon = false
			writeJSON(status)
			return
		}
		// With the daemon down nothing else can remove a leftover data path, so
		// tear it down from this (root) rpcd process directly.
		if method == "cleanup" {
			writeJSON(cleanupResult(defaultConfigPath))
			return
		}
		writeJSON(RPCError{Error: err.Error()})
		os.Exit(1)
	}
	defer resp.Body.Close()

	if _, err := io.Copy(os.Stdout, resp.Body); err != nil {
		writeJSON(RPCError{Error: err.Error()})
		os.Exit(1)
	}
}

func handleRPC(w http.ResponseWriter, r *http.Request, configPath string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeHTTPJSON(w, http.StatusBadRequest, RPCError{Error: err.Error()})
		return
	}
	method, found := methods[req.Method]
	if !found {
		writeHTTPJSON(w, http.StatusNotFound, RPCError{Error: "unknown method"})
		return
	}
	run := func() map[string]any {
		response := method.handler(configPath, req.Params)
		if method.reload {
			response = applyMutation(configPath, response)
		}
		return response
	}
	if method.timeout < probe {
		writeHTTPJSON(w, http.StatusOK, run())
		return
	}
	// Probing/fetching can outlast LuCI's 20s RPC timeout. Answer within
	// pendingAfter and let the work finish in the background; the UI re-reads.
	done := make(chan map[string]any, 1)
	go func() { done <- run() }()
	select {
	case response := <-done:
		writeHTTPJSON(w, http.StatusOK, response)
	case <-time.After(pendingAfter):
		writeHTTPJSON(w, http.StatusOK, map[string]any{"ok": true, "pending": true, "errors": []string{},
			"message": "still running in the background; refresh in a moment"})
	}
}

func setManagerEnabled(configPath string, raw json.RawMessage) map[string]any {
	var params EnabledParams
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return validationResult(false, runtime.Result{}, err)
		}
	}
	if err := managerconfig.SetManagerEnabled(configPath, params.Enabled); err != nil {
		return validationResult(false, runtime.Result{}, err)
	}
	return map[string]any{"ok": true, "enabled": params.Enabled}
}

func setManagerMode(configPath string, raw json.RawMessage) map[string]any {
	var params struct {
		Mode string `json:"mode"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return validationResult(false, runtime.Result{}, err)
		}
	}
	if err := managerconfig.SetManagerRuntimeMode(configPath, params.Mode); err != nil {
		return validationResult(false, runtime.Result{}, err)
	}
	return map[string]any{"ok": true, "mode": params.Mode}
}

func dnsWarnings(cfg managerconfig.Config) []string {
	warnings := []string{}
	if cfg.Transparent.Active() && cfg.Transparent.DNSHijack && !hasUsableDNSUpstream(cfg) {
		warnings = append(warnings, "DNS capture is enabled but no DNS server is enabled with a usable address")
	}
	return warnings
}

func hasUsableDNSUpstream(cfg managerconfig.Config) bool {
	for _, server := range cfg.DNSServers {
		if server.Enabled && server.Address != "" {
			return true
		}
	}
	return false
}

func renderedDNSDebug(cfg managerconfig.Config) ([]map[string]any, []map[string]any, map[string]any) {
	data, err := render.Render(cfg)
	if err != nil {
		return nil, nil, nil
	}
	var document struct {
		DNS *struct {
			Servers []map[string]any `json:"servers"`
			Rules   []map[string]any `json:"rules"`
		} `json:"dns"`
		Inbounds []map[string]any `json:"inbounds"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, nil, nil
	}
	// Router-bound DNS is nat-redirected into the dns-in inbound; off-router DNS
	// is tproxied into tproxy-in. sing-box's hijack-dns route rule answers both.
	// Report dns-in (falling back to tproxy-in) so the UI shows where capture
	// happens.
	var inbound map[string]any
	for _, candidate := range document.Inbounds {
		if candidate["tag"] == "dns-in" {
			inbound = candidate
			break
		}
		if candidate["tag"] == "tproxy-in" && inbound == nil {
			inbound = candidate
		}
	}
	if document.DNS == nil {
		return nil, nil, inbound
	}
	return document.DNS.Servers, document.DNS.Rules, inbound
}

func deleteDNSRule(configPath string, raw json.RawMessage) map[string]any {
	params, err := decodeIDParams(raw)
	if err != nil {
		return validationResult(false, runtime.Result{}, err)
	}
	if err := managerconfig.DeleteDNSRule(configPath, params.ID); err != nil {
		return validationResult(false, runtime.Result{}, err)
	}
	return map[string]any{"ok": true, "rule": params.ID}
}

func deleteDNSServer(configPath string, raw json.RawMessage) map[string]any {
	params, err := decodeIDParams(raw)
	if err != nil {
		return validationResult(false, runtime.Result{}, err)
	}
	if err := managerconfig.DeleteDNSServer(configPath, params.ID); err != nil {
		return validationResult(false, runtime.Result{}, err)
	}
	return map[string]any{"ok": true, "server": params.ID}
}

func runtimeStats() map[string]any {
	stats := collectRuntimeStats()
	return map[string]any{
		"ok":          true,
		"connections": stats.Connections,
		"rx_bytes":    stats.RxBytes,
		"tx_bytes":    stats.TxBytes,
	}
}

func logs(raw json.RawMessage) map[string]any {
	params := LogsParams{Lines: 200}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &params)
	}
	if params.Lines <= 0 {
		params.Lines = 200
	}
	if params.Lines > 2000 {
		params.Lines = 2000
	}
	text := readManagerLogs(params.Lines)
	return map[string]any{"ok": true, "lines": params.Lines, "text": text}
}

func devices() map[string]any {
	return map[string]any{"ok": true, "devices": discoverDevices()}
}

func transparentStatus(configPath string) map[string]any {
	cfg, err := managerconfig.Load(configPath)
	if err != nil {
		return validationResult(false, runtime.Result{}, err)
	}
	preview := ""
	if cfg.Transparent.Active() {
		data, err := firewall.Render(*cfg)
		if err != nil {
			return validationResult(false, runtime.Result{}, err)
		}
		preview = string(data)
	}
	devices := make([]map[string]any, 0, len(cfg.Transparent.Devices))
	for _, device := range cfg.Transparent.Devices {
		devices = append(devices, map[string]any{
			"id":         device.ID,
			"enabled":    device.Enabled,
			"name":       device.Name,
			"mac":        device.MAC,
			"ipv4":       device.IPv4,
			"ipv6":       device.IPv6,
			"mode":       device.Mode,
			"bypass_udp": device.BypassUDP,
			"effective":  cfg.Transparent.Effective(device),
			"group":      device.Group,
		})
	}
	return map[string]any{
		"ok":               true,
		"default_mode":     cfg.Transparent.DefaultMode,
		"active":           cfg.Transparent.Active(),
		"uses_tproxy":      cfg.UsesTProxy(),
		"lan_ifnames":      cfg.Transparent.LANIfnames,
		"bypass_subnet":    cfg.Transparent.BypassSubnet,
		"devices":          devices,
		"dns_hijack":       cfg.Transparent.DNSHijack,
		"kill_switch":      cfg.Transparent.KillSwitch,
		"tproxy_port":      cfg.Manager.TProxyPort,
		"dns_port":         cfg.Manager.DNSPort,
		"nftables_include": runtime.DefaultPaths.NftablesInclude,
		"nftables_present": fileExists(runtime.DefaultPaths.NftablesInclude),
		"nftables_preview": preview,
		"lan_devices":      discoverDevices(),
	}
}

func tunStatus(configPath string) map[string]any {
	cfg, err := managerconfig.Load(configPath)
	if err != nil {
		return validationResult(false, runtime.Result{}, err)
	}
	return map[string]any{
		"ok":            true,
		"enabled":       cfg.TUN.Enabled,
		"auto_route":    cfg.TUN.AutoRoute,
		"auto_redirect": cfg.TUN.AutoRedirect,
		"inet4_address": cfg.TUN.Inet4Address,
		"inet6_address": cfg.TUN.Inet6Address,
		"interface":     "singbox0",
	}
}

func setTransparent(configPath string, raw json.RawMessage) map[string]any {
	payload, err := decodeTransparent(raw)
	if err != nil {
		return validationResult(false, runtime.Result{}, err)
	}
	defaultMode := payload.DefaultMode
	if defaultMode == "" {
		defaultMode = "off"
	}
	transparent := managerconfig.Transparent{
		DefaultMode:  defaultMode,
		LANIfnames:   payload.LANIfnames,
		BypassSubnet: payload.BypassSubnet,
		DNSHijack:    payload.DNSHijack,
		KillSwitch:   payload.KillSwitch,
	}
	for _, device := range payload.Devices {
		mode := device.Mode
		if mode == "" {
			mode = "default"
		}
		// An empty ID lets the config layer derive the section name from Name.
		transparent.Devices = append(transparent.Devices, managerconfig.Device{
			ID:        device.ID,
			Enabled:   device.Enabled,
			Name:      device.Name,
			MAC:       device.MAC,
			IPv4:      device.IPv4,
			IPv6:      device.IPv6,
			Mode:      mode,
			BypassUDP: device.BypassUDP,
			Group:     device.Group,
		})
	}
	if err := managerconfig.UpsertTransparent(configPath, transparent); err != nil {
		return validationResult(false, runtime.Result{}, err)
	}
	return map[string]any{"ok": true, "default_mode": transparent.DefaultMode, "active": transparent.Active()}
}

func setTUN(configPath string, raw json.RawMessage) map[string]any {
	payload, err := decodeTUN(raw)
	if err != nil {
		return validationResult(false, runtime.Result{}, err)
	}
	tun := managerconfig.TUN{
		Enabled:      payload.Enabled,
		AutoRoute:    payload.AutoRoute,
		AutoRedirect: payload.AutoRedirect,
		Inet4Address: payload.Inet4Address,
		Inet6Address: payload.Inet6Address,
	}
	if err := managerconfig.UpsertTUN(configPath, tun); err != nil {
		return validationResult(false, runtime.Result{}, err)
	}
	return map[string]any{"ok": true, "enabled": tun.Enabled}
}

func latencyTest(raw json.RawMessage) map[string]any {
	var params LatencyParams
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return validationResult(false, runtime.Result{}, err)
		}
	}
	result, err := health.TestURL(context.Background(), params.URL)
	response := map[string]any{
		"ok":         err == nil,
		"url":        result.ID,
		"health":     result.Health,
		"latency_ms": result.LatencyMS,
	}
	if err != nil {
		response["errors"] = []string{err.Error()}
	} else {
		response["errors"] = []string{}
	}
	return response
}

func dnsTest(configPath string, raw json.RawMessage) map[string]any {
	var params DNSTestParams
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return validationResult(false, runtime.Result{}, err)
		}
	}
	if params.Server == "" {
		return validationResult(false, runtime.Result{}, fmt.Errorf("server is required"))
	}
	cfg, err := managerconfig.Load(configPath)
	if err != nil {
		return validationResult(false, runtime.Result{}, err)
	}
	server, ok := cfg.DNSServers[params.Server]
	if !ok {
		return validationResult(false, runtime.Result{}, fmt.Errorf("dns server %q not found", params.Server))
	}
	result, err := health.TestDNS(context.Background(), server, params.Domain)
	response := map[string]any{
		"ok":         err == nil,
		"server":     result.ID,
		"domain":     params.Domain,
		"health":     result.Health,
		"latency_ms": result.LatencyMS,
	}
	if err != nil {
		response["errors"] = []string{err.Error()}
	} else {
		response["errors"] = []string{}
	}
	return response
}

func validateRuntimeConfig(configPath string) map[string]any {
	cfg, err := managerconfig.Load(configPath)
	if err != nil {
		return validationResult(false, runtime.Result{}, err)
	}

	result, err := runtime.Validate(*cfg, runtime.DefaultPaths, render.Render)
	response := validationResult(err == nil, result, err)
	response["warnings"] = dnsWarnings(*cfg)
	return response
}

func controlRuntime(configPath string, action runtime.Action) map[string]any {
	cfg, err := managerconfig.Load(configPath)
	if err != nil {
		return validationResult(false, runtime.Result{}, err)
	}

	result, err := runtime.Control(*cfg, action, runtime.DefaultPaths, render.Render)
	response := validationResult(err == nil, result, err)
	if result.Message != "" {
		response["message"] = result.Message
	}
	if result.PID > 0 {
		response["pid"] = result.PID
	}
	return response
}

// startRuntime brings the proxy up and clears the management-only hold, so the
// supervisor keeps it running and a reboot restores it.
func startRuntime(configPath string) map[string]any {
	_ = managerconfig.SetManagerPaused(configPath, false)
	return controlRuntime(configPath, runtime.ActionStart)
}

// restartRuntime restarts the proxy and clears the management-only hold.
func restartRuntime(configPath string) map[string]any {
	_ = managerconfig.SetManagerPaused(configPath, false)
	return controlRuntime(configPath, runtime.ActionRestart)
}

// stopRuntime takes the proxy down and engages the management-only hold, so
// neither the supervisor nor a reboot brings it back until an explicit start.
// This is the LAN-safety guarantee: stopping stays stopped.
func stopRuntime(configPath string) map[string]any {
	result := controlRuntime(configPath, runtime.ActionStop)
	_ = managerconfig.SetManagerPaused(configPath, true)
	return result
}

// setManagerPaused toggles the management-only hold and reconciles the data path
// immediately: pausing tears the proxy down, resuming brings it back when the
// manager is enabled.
func setManagerPaused(configPath string, raw json.RawMessage) map[string]any {
	var params struct {
		Paused bool `json:"paused"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return validationResult(false, runtime.Result{}, err)
		}
	}
	if err := managerconfig.SetManagerPaused(configPath, params.Paused); err != nil {
		return validationResult(false, runtime.Result{}, err)
	}
	if params.Paused {
		controlRuntime(configPath, runtime.ActionStop)
	} else if cfg, err := managerconfig.Load(configPath); err == nil && cfg.Manager.Enabled {
		controlRuntime(configPath, runtime.ActionStart)
	}
	return map[string]any{"ok": true, "paused": params.Paused}
}

func validationResult(ok bool, result runtime.Result, err error) map[string]any {
	errors := []string{}
	if err != nil {
		errors = append(errors, managerconfig.ErrorStrings(err)...)
	}

	response := map[string]any{
		"ok":             ok,
		"errors":         errors,
		"generated_path": result.GeneratedPath,
		"runtime_path":   result.RuntimePath,
		"nftables_path":  result.NftablesPath,
		"check_output":   strings.TrimSpace(result.CheckOutput),
	}
	return response
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// localProxyAddr returns the local mixed-inbound address ("127.0.0.1:port")
// when sing-box is running, so the daemon can route its own outbound HTTP
// (e.g. rule-set downloads) through the tunnel. Empty means "go direct".
func localProxyAddr(cfg managerconfig.Config) string {
	if managedRuntimePID(runtime.DefaultPaths) == 0 {
		return ""
	}
	if cfg.Manager.MixedPort <= 0 {
		return ""
	}
	return fmt.Sprintf("127.0.0.1:%d", cfg.Manager.MixedPort)
}

// applyMutation reloads the running sing-box so a just-saved config change
// takes effect immediately. It is a no-op when the change failed or nothing is
// running, and it never turns a successful save into a failure: reload trouble
// is reported via the "reloaded"/"reload_error" fields, not "ok".
func applyMutation(configPath string, response map[string]any) map[string]any {
	if response == nil {
		return response
	}
	if ok, _ := response["ok"].(bool); !ok {
		return response
	}
	if managedRuntimePID(runtime.DefaultPaths) == 0 {
		response["reloaded"] = false
		return response
	}
	cfg, err := managerconfig.Load(configPath)
	if err != nil {
		log.Printf("auto-reload skipped: %v", err)
		response["reloaded"] = false
		response["reload_error"] = err.Error()
		return response
	}
	if _, err := runtime.Control(*cfg, runtime.ActionReload, runtime.DefaultPaths, render.Render); err != nil {
		log.Printf("auto-reload after config change failed: %v", err)
		response["reloaded"] = false
		response["reload_error"] = err.Error()
		return response
	}
	response["reloaded"] = true
	return response
}

func managedRuntimePID(paths runtime.Paths) int {
	pid := runtime.RunningPID(paths)
	if pid == 0 || !processHasArg(pid, paths.RuntimeConfig) {
		return 0
	}
	return pid
}

func processHasArg(pid int, arg string) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return false
	}
	return cmdlineHasArg(data, arg)
}

func cmdlineHasArg(data []byte, arg string) bool {
	for _, part := range strings.Split(strings.TrimRight(string(data), "\x00"), "\x00") {
		if part == arg {
			return true
		}
	}
	return false
}

func findProcess(name string) int {
	out, err := exec.Command("pidof", name).Output()
	if err != nil {
		return 0
	}
	for _, part := range strings.Fields(string(out)) {
		pid, err := strconv.Atoi(part)
		if err == nil && pid > 0 {
			return pid
		}
	}
	return 0
}

func readRSS(pid int) uint64 {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0
		}
		return value
	}
	return 0
}

func collectRuntimeStats() RuntimeStats {
	rx, tx := readTrafficCounters()
	return RuntimeStats{
		Connections: readConnectionCount(),
		RxBytes:     rx,
		TxBytes:     tx,
	}
}

func readTrafficCounters() (uint64, uint64) {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return 0, 0
	}
	var rx, tx uint64
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if strings.TrimSpace(parts[0]) == "lo" {
			continue
		}
		fields := strings.Fields(parts[1])
		if len(fields) < 16 {
			continue
		}
		in, _ := strconv.ParseUint(fields[0], 10, 64)
		out, _ := strconv.ParseUint(fields[8], 10, 64)
		rx += in
		tx += out
	}
	return rx, tx
}

func readConnectionCount() int {
	return countEstablishedTCP("/proc/net/tcp") + countEstablishedTCP("/proc/net/tcp6")
}

func countEstablishedTCP(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	count := 0
	for i, line := range strings.Split(string(data), "\n") {
		if i == 0 {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 4 && fields[3] == "01" {
			count++
		}
	}
	return count
}

func readManagerLogs(lines int) string {
	out, err := exec.Command("logread", "-e", "singbox-manager", "-l", strconv.Itoa(lines)).Output()
	if err == nil && len(strings.TrimSpace(string(out))) > 0 {
		return tailLines(string(out), lines)
	}
	return tailFilteredFile("/var/log/messages", "singbox-manager", lines)
}

func tailFilteredFile(path string, needle string, lines int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	matched := []string{}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, needle) {
			matched = append(matched, line)
		}
	}
	return strings.Join(lastStrings(matched, lines), "\n")
}

func tailLines(text string, lines int) string {
	return strings.Join(lastStrings(strings.Split(strings.TrimRight(text, "\n"), "\n"), lines), "\n")
}

func lastStrings(values []string, n int) []string {
	if n <= 0 || len(values) <= n {
		return values
	}
	return values[len(values)-n:]
}

func discoverDevices() []Device {
	return collectDevices(
		parseDHCPLeases("/tmp/dhcp.leases"),
		parseARP("/proc/net/arp"),
		infraAddrs("/proc/net/route"),
	)
}

// collectDevices merges DHCP leases with ARP neighbours into a deduplicated,
// sorted device list, dropping infrastructure addresses (the router's own
// IPs, subnet broadcasts and gateways) and anything that is not a routable
// unicast host — those are never end-user LAN devices a user would pick.
func collectDevices(leases, arp []Device, skip map[string]bool) []Device {
	byIP := map[string]Device{}
	for _, device := range leases {
		byIP[device.IP] = device
	}
	for _, device := range arp {
		if existing, ok := byIP[device.IP]; ok {
			if existing.MAC == "" {
				existing.MAC = device.MAC
				byIP[device.IP] = existing
			}
			continue
		}
		byIP[device.IP] = device
	}
	devices := make([]Device, 0, len(byIP))
	for _, device := range byIP {
		if skip[device.IP] || !isPickableHostIP(device.IP) {
			continue
		}
		devices = append(devices, device)
	}
	sort.Slice(devices, func(i, j int) bool {
		return devices[i].IP < devices[j].IP
	})
	return devices
}

// infraAddrs returns the set of addresses the device picker hides: every
// router interface IP, the broadcast address of each local IPv4 subnet, and
// the default gateway(s) read from routePath (/proc/net/route).
func infraAddrs(routePath string) map[string]bool {
	skip := map[string]bool{}
	if ifaces, err := net.Interfaces(); err == nil {
		for _, iface := range ifaces {
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				ipNet, ok := addr.(*net.IPNet)
				if !ok {
					continue
				}
				if ip := ipNet.IP.String(); ip != "<nil>" {
					skip[ip] = true
				}
				if bcast := broadcastIP(ipNet); bcast != "" {
					skip[bcast] = true
				}
			}
		}
	}
	for _, gw := range defaultGateways(routePath) {
		skip[gw] = true
	}
	return skip
}

// broadcastIP returns the directed-broadcast address of an IPv4 network, or
// "" for IPv6 networks (which have no broadcast).
func broadcastIP(ipNet *net.IPNet) string {
	ip4 := ipNet.IP.To4()
	if ip4 == nil {
		return ""
	}
	mask := ipNet.Mask
	if len(mask) == net.IPv6len {
		mask = mask[12:]
	}
	if len(mask) != net.IPv4len {
		return ""
	}
	bcast := make(net.IP, net.IPv4len)
	for i := 0; i < net.IPv4len; i++ {
		bcast[i] = ip4[i] | ^mask[i]
	}
	return bcast.String()
}

// defaultGateways parses /proc/net/route for default-route (0.0.0.0) gateways
// and returns them as dotted-quad strings.
func defaultGateways(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	gateways := []string{}
	for i, line := range strings.Split(string(data), "\n") {
		if i == 0 {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[1] != "00000000" {
			continue
		}
		if gw := decodeRouteIP(fields[2]); gw != "" {
			gateways = append(gateways, gw)
		}
	}
	return gateways
}

// decodeRouteIP decodes the little-endian hex IPv4 address used in the
// Gateway/Destination columns of /proc/net/route.
func decodeRouteIP(hexStr string) string {
	if len(hexStr) != 8 {
		return ""
	}
	raw, err := hex.DecodeString(hexStr)
	if err != nil || len(raw) != 4 {
		return ""
	}
	return net.IPv4(raw[3], raw[2], raw[1], raw[0]).String()
}

// isPickableHostIP reports whether an address is a normal routable host a user
// would assign a per-device override to — rejecting the unspecified, loopback,
// link-local, multicast and limited-broadcast ranges.
func isPickableHostIP(s string) bool {
	ip := net.ParseIP(s)
	if ip == nil {
		return false
	}
	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsMulticast() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	if ip4 := ip.To4(); ip4 != nil && ip4.Equal(net.IPv4bcast) {
		return false
	}
	return true
}

func parseDHCPLeases(path string) []Device {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	devices := []Device{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		name := ""
		if len(fields) >= 4 && fields[3] != "*" {
			name = fields[3]
		}
		devices = append(devices, Device{
			IP:     fields[2],
			MAC:    fields[1],
			Name:   name,
			Source: "dhcp",
		})
	}
	return devices
}

func parseARP(path string) []Device {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	devices := []Device{}
	for i, line := range strings.Split(string(data), "\n") {
		if i == 0 {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[3] == "00:00:00:00:00:00" {
			continue
		}
		devices = append(devices, Device{
			IP:     fields[0],
			MAC:    fields[3],
			Source: "arp",
		})
	}
	return devices
}

func decodeIDParams(raw json.RawMessage) (IDParams, error) {
	var params IDParams
	if len(raw) == 0 {
		return params, fmt.Errorf("id is required")
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return params, err
	}
	if params.ID == "" {
		return params, fmt.Errorf("id is required")
	}
	return params, nil
}

func decodeTransparent(raw json.RawMessage) (TransparentPayload, error) {
	var envelope struct {
		Transparent *TransparentPayload `json:"transparent"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && envelope.Transparent != nil {
		return *envelope.Transparent, nil
	}
	var payload TransparentPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return TransparentPayload{}, err
	}
	return payload, nil
}

func decodeTUN(raw json.RawMessage) (TUNPayload, error) {
	var envelope struct {
		TUN *TUNPayload `json:"tun"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && envelope.TUN != nil {
		return *envelope.TUN, nil
	}
	var payload TUNPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return TUNPayload{}, err
	}
	return payload, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func mustMarshal(value any) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}

func writeHTTPJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSON(value any) {
	_ = json.NewEncoder(os.Stdout).Encode(value)
}
