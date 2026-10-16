package subscription

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	managerconfig "github.com/openwrt-singbox/singbox-manager/internal/config"
)

const maxSubscriptionSize = 10 << 20

var ErrUnsupportedTransport = errors.New("unsupported transport")

// Item is one outbound parsed from a subscription payload or a source string.
type Item struct {
	Name     string
	Outbound map[string]any
}

// nonProxyTypes are sing-box outbound types that are plumbing, not servers.
var nonProxyTypes = map[string]bool{
	"selector": true, "urltest": true, "direct": true, "block": true, "dns": true,
}

// ParseOutbounds turns a subscription payload into outbounds. A sing-box JSON
// config (or bare outbounds array) is passed through verbatim, so every
// protocol and option sing-box knows survives; share-URI lists are converted.
//
// Entries sing-box cannot run (an unsupported protocol or transport such as
// xhttp) are skipped, not fatal: skipped summarises them. err is reserved for
// a payload that is not a subscription at all (e.g. an HTML page).
func ParseOutbounds(data []byte, format string) (items []Item, skipped string, err error) {
	payload, err := decodePayload(strings.TrimSpace(string(data)), format)
	if err != nil {
		return nil, "", err
	}
	if looksLikeJSONConfig(payload) {
		data := []byte(strings.TrimSpace(payload))
		if looksLikeXray(data) {
			var xraySkipped []string
			items, xraySkipped, err = parseXrayOutbounds(data)
			if len(xraySkipped) > 0 {
				skipped = summarizeErrors(xraySkipped)
			}
			return items, skipped, err
		}
		items, err = parseJSONOutbounds(data)
		return items, "", err
	}
	var parseErrors []string
	recognized := false
	for _, line := range strings.FieldsFunc(payload, func(r rune) bool {
		return r == '\n' || r == '\r' || r == '\t' || r == ' '
	}) {
		if scheme, _, ok := strings.Cut(line, "://"); ok && proxySchemes[strings.ToLower(scheme)] {
			recognized = true
		}
		item, err := ParseSource(line)
		if err != nil {
			parseErrors = append(parseErrors, err.Error())
			continue
		}
		items = append(items, item)
	}
	if len(parseErrors) > 0 {
		skipped = summarizeErrors(parseErrors)
	}
	if len(items) == 0 && !recognized {
		if skipped == "" {
			skipped = "empty payload"
		}
		return nil, "", fmt.Errorf("not a subscription: %s", skipped)
	}
	return items, skipped, nil
}

// proxySchemes are share-URI schemes of real proxies, supported or not; a
// payload containing any of them is a subscription even if none can be used.
var proxySchemes = map[string]bool{
	"vless": true, "vmess": true, "trojan": true, "ss": true, "ssr": true,
	"hysteria": true, "hysteria2": true, "hy2": true, "tuic": true,
	"wireguard": true, "wg": true, "socks": true, "socks5": true, "anytls": true,
}

// summarizeErrors collapses repeated per-entry errors ("19× unsupported
// transport ...") so a subscription of one unsupported kind reads clearly.
func summarizeErrors(messages []string) string {
	counts := map[string]int{}
	var order []string
	for _, message := range messages {
		if counts[message] == 0 {
			order = append(order, message)
		}
		counts[message]++
	}
	parts := make([]string, 0, len(order))
	for _, message := range order {
		if counts[message] > 1 {
			message = fmt.Sprintf("%d× %s", counts[message], message)
		}
		parts = append(parts, message)
	}
	return strings.Join(parts, "; ")
}

// ParseSource converts one outbound definition — a share URI or a sing-box
// outbound JSON object — into a sing-box outbound (tag removed; the caller
// assigns it).
func ParseSource(source string) (Item, error) {
	source = strings.TrimSpace(source)
	if strings.HasPrefix(source, "{") {
		var outbound map[string]any
		if err := json.Unmarshal([]byte(source), &outbound); err != nil {
			return Item{}, fmt.Errorf("invalid outbound JSON: %w", err)
		}
		if stringField(outbound, "type") == "" {
			return Item{}, fmt.Errorf("outbound JSON needs a type")
		}
		return jsonItem(outbound), nil
	}
	node, err := ParseURI(source)
	if err != nil {
		return Item{}, err
	}
	outbound, err := NodeOutbound(node)
	if err != nil {
		return Item{}, err
	}
	delete(outbound, "tag")
	return Item{Name: node.Name, Outbound: outbound}, nil
}

func jsonItem(outbound map[string]any) Item {
	name := stringField(outbound, "tag")
	if name == "" {
		name = stringField(outbound, "server")
	}
	// Tags and cross-references belong to the source document, not ours.
	delete(outbound, "tag")
	delete(outbound, "detour")
	delete(outbound, "domain_resolver")
	return Item{Name: name, Outbound: outbound}
}

func parseJSONOutbounds(data []byte) ([]Item, error) {
	var outbounds []any
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err == nil {
		raw, ok := doc["outbounds"].([]any)
		if !ok {
			return nil, fmt.Errorf("sing-box config has no outbounds array")
		}
		outbounds = raw
	} else if err2 := json.Unmarshal(data, &outbounds); err2 != nil {
		return nil, fmt.Errorf("invalid sing-box JSON config: %w", err)
	}
	var items []Item
	for _, raw := range outbounds {
		outbound, ok := raw.(map[string]any)
		if !ok || nonProxyTypes[stringField(outbound, "type")] || stringField(outbound, "server") == "" {
			continue
		}
		items = append(items, jsonItem(outbound))
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("no proxy outbounds found in sing-box config")
	}
	return items, nil
}

// Candidates gives each outbound a stable tag "<subscription>.<hash of its
// definition>", so probe history and selection survive refetches that reorder
// the list. Exact duplicates collapse into one.
func Candidates(subscriptionID string, items []Item) []managerconfig.Candidate {
	seen := map[string]bool{}
	var candidates []managerconfig.Candidate
	for _, item := range items {
		definition, _ := json.Marshal(item.Outbound)
		sum := sha1.Sum(definition)
		id := subscriptionID + "." + hex.EncodeToString(sum[:])[:10]
		if seen[id] {
			continue
		}
		seen[id] = true
		outbound := map[string]any{}
		for k, v := range item.Outbound {
			outbound[k] = v
		}
		outbound["tag"] = id
		candidates = append(candidates, managerconfig.Candidate{ID: id, Name: item.Name, Outbound: outbound})
	}
	return candidates
}

// Match returns the IDs of the candidates that pass a filter expression.
func Match(filter string, candidates []managerconfig.Candidate) ([]string, error) {
	keep, err := managerconfig.Filter(filter)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for _, candidate := range candidates {
		if keep(managerconfig.FilterEnv{
			Name:     candidate.Name,
			Type:     stringField(candidate.Outbound, "type"),
			Server:   stringField(candidate.Outbound, "server"),
			Port:     intField(candidate.Outbound, "server_port"),
			Outbound: candidate.Outbound,
		}) {
			ids = append(ids, candidate.ID)
		}
	}
	return ids, nil
}

// Due reports whether a subscription should be refetched.
func Due(source managerconfig.Subscription, now time.Time) bool {
	if !source.Enabled || source.URL == "" {
		return false
	}
	updated, err := time.Parse(time.RFC3339, source.LastUpdate)
	if err != nil {
		return true
	}
	interval, err := time.ParseDuration(source.UpdateInterval)
	if err != nil || interval <= 0 {
		interval = 24 * time.Hour
	}
	if interval < time.Minute {
		interval = time.Minute
	}
	return !updated.Add(interval).After(now)
}

// Fetch downloads a subscription, through proxyAddr (an HTTP proxy such as the
// local mixed inbound) when it is set.
func Fetch(ctx context.Context, rawURL string, proxyAddr string, headers map[string]string) ([]byte, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("subscription URL must use http or https")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	if proxyAddr != "" {
		client.Transport = &http.Transport{Proxy: http.ProxyURL(&url.URL{Scheme: "http", Host: proxyAddr})}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "singbox-manager/1.0")
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("subscription fetch failed: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSubscriptionSize+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxSubscriptionSize {
		return nil, fmt.Errorf("subscription response exceeds %d bytes", maxSubscriptionSize)
	}
	return body, nil
}

func ParseURI(raw string) (managerconfig.Node, error) {
	switch {
	case strings.HasPrefix(raw, "vmess://"):
		return parseVMess(raw)
	case strings.HasPrefix(raw, "vless://"):
		return parseStandardURI(raw, "vless")
	case strings.HasPrefix(raw, "trojan://"):
		return parseStandardURI(raw, "trojan")
	case strings.HasPrefix(raw, "ss://"):
		return parseShadowsocks(raw)
	case strings.HasPrefix(raw, "hysteria2://"):
		return parseStandardURI(raw, "hysteria2")
	case strings.HasPrefix(raw, "hy2://"):
		return parseStandardURI("hysteria2://"+strings.TrimPrefix(raw, "hy2://"), "hysteria2")
	case strings.HasPrefix(raw, "tuic://"):
		return parseStandardURI(raw, "tuic")
	default:
		return managerconfig.Node{}, fmt.Errorf("unsupported subscription URI %q", raw)
	}
}

// parseSingBoxConfig extracts proxy nodes from a full sing-box JSON config or a
// bare array of outbound objects.
func decodePayload(payload string, format string) (string, error) {
	switch format {
	case "", "auto":
		if looksPlain(payload) || looksLikeJSONConfig(payload) {
			return payload, nil
		}
		if decoded, ok := decodeBase64(payload); ok && (looksPlain(decoded) || looksLikeJSONConfig(decoded)) {
			return decoded, nil
		}
		return payload, nil
	case "plain":
		return payload, nil
	case "base64":
		decoded, ok := decodeBase64(payload)
		if !ok {
			return "", fmt.Errorf("subscription payload is not valid base64")
		}
		return decoded, nil
	default:
		return "", fmt.Errorf("unsupported subscription format %q", format)
	}
}

func looksPlain(payload string) bool {
	return strings.Contains(payload, "vmess://") ||
		strings.Contains(payload, "vless://") ||
		strings.Contains(payload, "trojan://") ||
		strings.Contains(payload, "ss://") ||
		strings.Contains(payload, "hysteria2://") ||
		strings.Contains(payload, "hy2://") ||
		strings.Contains(payload, "tuic://")
}

func looksLikeJSONConfig(payload string) bool {
	trimmed := strings.TrimSpace(payload)
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
}

func decodeBase64(value string) (string, bool) {
	compact := strings.NewReplacer("\n", "", "\r", "", " ", "", "\t", "").Replace(value)
	for _, encoding := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	} {
		data, err := encoding.DecodeString(compact)
		if err == nil {
			return string(data), true
		}
	}
	if remainder := len(compact) % 4; remainder != 0 {
		padded := compact + strings.Repeat("=", 4-remainder)
		data, err := base64.StdEncoding.DecodeString(padded)
		if err == nil {
			return string(data), true
		}
		data, err = base64.URLEncoding.DecodeString(padded)
		if err == nil {
			return string(data), true
		}
	}
	return "", false
}

func parseVMess(raw string) (managerconfig.Node, error) {
	body := strings.TrimPrefix(raw, "vmess://")
	// The newer "standard" share form is vmess://uuid@host:port?params#name
	// (same shape as vless). The classic form is vmess://base64(json); base64
	// alphabets never contain '@', so an '@' before the fragment disambiguates.
	head := body
	if idx := strings.Index(head, "#"); idx >= 0 {
		head = head[:idx]
	}
	if strings.Contains(head, "@") {
		return parseStandardURI(raw, "vmess")
	}
	decoded, ok := decodeBase64(body)
	if !ok {
		return managerconfig.Node{}, fmt.Errorf("invalid vmess payload")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(decoded), &payload); err != nil {
		return managerconfig.Node{}, err
	}
	node := managerconfig.Node{
		Enabled:   true,
		Type:      "vmess",
		Name:      stringField(payload, "ps"),
		Server:    stringField(payload, "add"),
		Port:      intField(payload, "port"),
		UUID:      stringField(payload, "id"),
		Security:  stringField(payload, "scy"),
		TLS:       stringField(payload, "tls") == "tls",
		Transport: normalizeTransport(stringField(payload, "net")),
		Host:      stringField(payload, "host"),
		Path:      stringField(payload, "path"),
		SNI:       stringField(payload, "sni"),
	}
	if node.Name == "" {
		node.Name = node.Server
	}
	return node, nil
}

func parseStandardURI(raw string, typ string) (managerconfig.Node, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return managerconfig.Node{}, err
	}
	name, _ := url.QueryUnescape(parsed.Fragment)
	host := parsed.Hostname()
	port, _ := strconv.Atoi(parsed.Port())
	query := parsed.Query()

	node := managerconfig.Node{
		Enabled:          true,
		Type:             typ,
		Name:             firstNonEmpty(name, host),
		Server:           host,
		Port:             port,
		Security:         firstNonEmpty(query.Get("security"), query.Get("tls")),
		TLS:              inSet(firstNonEmpty(query.Get("security"), query.Get("tls")), "tls", "reality"),
		Transport:        normalizeTransport(firstNonEmpty(query.Get("type"), query.Get("transport"))),
		Host:             query.Get("host"),
		Path:             query.Get("path"),
		SNI:              firstNonEmpty(query.Get("sni"), query.Get("peer"), query.Get("server_name")),
		ALPN:             query.Get("alpn"),
		Insecure:         isTruthy(firstNonEmpty(query.Get("insecure"), query.Get("allowInsecure"))),
		Flow:             query.Get("flow"),
		RealityPublicKey: query.Get("pbk"),
		RealityShortID:   query.Get("sid"),
		Fingerprint:      query.Get("fp"),
		Congestion:       query.Get("congestion_control"),
		UDPRelayMode:     query.Get("udp_relay_mode"),
	}

	switch typ {
	case "vless":
		node.UUID = parsed.User.Username()
	case "vmess":
		node.UUID = parsed.User.Username()
		// For vmess the query "security" denotes TLS (already captured in
		// node.TLS above); the VMess cipher comes from "encryption"/"scy" and
		// must not be confused with the TLS setting.
		cipher := firstNonEmpty(query.Get("encryption"), query.Get("scy"))
		if cipher == "" || cipher == "tls" || cipher == "reality" {
			cipher = "auto"
		}
		node.Security = cipher
	case "trojan", "hysteria2":
		password, _ := url.QueryUnescape(parsed.User.Username())
		node.Password = password
		if node.Security == "" && (node.SNI != "" || strings.EqualFold(query.Get("security"), "tls")) {
			node.Security = "tls"
		}
		node.TLS = node.TLS || node.Security == "tls" || node.SNI != ""
	case "tuic":
		node.UUID = parsed.User.Username()
		password, _ := parsed.User.Password()
		node.Password, _ = url.QueryUnescape(password)
		if node.Security == "" {
			node.Security = "tls"
		}
		node.TLS = true
	}
	if node.Security == "1" || node.Security == "true" {
		node.Security = "tls"
		node.TLS = true
	}
	return node, nil
}

func parseShadowsocks(raw string) (managerconfig.Node, error) {
	body := strings.TrimPrefix(raw, "ss://")
	fragment := ""
	if idx := strings.Index(body, "#"); idx >= 0 {
		fragment, _ = url.QueryUnescape(body[idx+1:])
		body = body[:idx]
	}

	if strings.Contains(body, "@") {
		parsed, err := url.Parse("ss://" + body)
		if err != nil {
			return managerconfig.Node{}, err
		}
		user := parsed.User.Username()
		if decoded, ok := decodeBase64(user); ok {
			user = decoded
		}
		method, password := splitPair(user, ":")
		return managerconfig.Node{
			Enabled:  true,
			Type:     "shadowsocks",
			Name:     firstNonEmpty(fragment, parsed.Hostname()),
			Server:   parsed.Hostname(),
			Port:     mustPort(parsed.Port()),
			Method:   method,
			Password: passwordFromURL(password),
		}, nil
	}

	queryStart := strings.Index(body, "?")
	if queryStart >= 0 {
		body = body[:queryStart]
	}
	decoded, ok := decodeBase64(body)
	if !ok {
		return managerconfig.Node{}, fmt.Errorf("invalid shadowsocks payload")
	}
	methodPassword, serverPort := splitPair(decoded, "@")
	method, password := splitPair(methodPassword, ":")
	server, portText := splitLast(serverPort, ":")
	return managerconfig.Node{
		Enabled:  true,
		Type:     "shadowsocks",
		Name:     firstNonEmpty(fragment, server),
		Server:   server,
		Port:     mustPort(portText),
		Method:   method,
		Password: passwordFromURL(password),
	}, nil
}

func stringField(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func intField(payload map[string]any, key string) int {
	switch value := payload[key].(type) {
	case float64:
		return int(value)
	case string:
		port, _ := strconv.Atoi(value)
		return port
	default:
		return 0
	}
}

func normalizeTransport(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "websocket" {
		return "ws"
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func inSet(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func splitPair(value string, sep string) (string, string) {
	left, right, _ := strings.Cut(value, sep)
	return left, right
}

func splitLast(value string, sep string) (string, string) {
	index := strings.LastIndex(value, sep)
	if index < 0 {
		return value, ""
	}
	return value[:index], value[index+len(sep):]
}

func mustPort(value string) int {
	port, _ := strconv.Atoi(value)
	return port
}

func passwordFromURL(value string) string {
	password, err := url.QueryUnescape(value)
	if err != nil {
		return value
	}
	return password
}

func isTruthy(value string) bool {
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
func NodeOutbound(node managerconfig.Node) (map[string]any, error) {
	tag := node.Tag
	if tag == "" {
		tag = node.ID
	}
	server := node.Server
	if server == "" {
		server = node.Address
	}

	switch node.Type {
	case "direct":
		return map[string]any{"type": "direct", "tag": tag}, nil
	case "shadowsocks":
		return map[string]any{
			"type":        "shadowsocks",
			"tag":         tag,
			"server":      server,
			"server_port": node.Port,
			"method":      node.Method,
			"password":    node.Password,
		}, nil
	case "trojan":
		outbound := map[string]any{
			"type":        "trojan",
			"tag":         tag,
			"server":      server,
			"server_port": node.Port,
			"password":    node.Password,
		}
		addTLS(outbound, node)
		if err := addTransport(outbound, node); err != nil {
			return nil, err
		}
		return outbound, nil
	case "vmess":
		outbound := map[string]any{
			"type":        "vmess",
			"tag":         tag,
			"server":      server,
			"server_port": node.Port,
			"uuid":        node.UUID,
		}
		// For VMess, "security" is the cipher — not the transport security. Share
		// links overload the field with "tls"/"reality", which sing-box rejects as
		// a cipher ("unsupported security type: tls"); those are transport security
		// and belong to the TLS block that addTLS renders. Only forward a genuine
		// VMess cipher here and let anything else fall through to addTLS.
		if cipher := vmessCipher(node.Security); cipher != "" {
			outbound["security"] = cipher
		}
		addTLS(outbound, node)
		if err := addTransport(outbound, node); err != nil {
			return nil, err
		}
		return outbound, nil
	case "vless":
		outbound := map[string]any{
			"type":        "vless",
			"tag":         tag,
			"server":      server,
			"server_port": node.Port,
			"uuid":        node.UUID,
		}
		if node.Flow != "" {
			outbound["flow"] = node.Flow
		}
		addTLS(outbound, node)
		if err := addTransport(outbound, node); err != nil {
			return nil, err
		}
		return outbound, nil
	case "hysteria2":
		outbound := map[string]any{
			"type":        "hysteria2",
			"tag":         tag,
			"server":      server,
			"server_port": node.Port,
			"password":    node.Password,
		}
		addTLS(outbound, node)
		return outbound, nil
	case "tuic":
		outbound := map[string]any{
			"type":        "tuic",
			"tag":         tag,
			"server":      server,
			"server_port": node.Port,
			"uuid":        node.UUID,
			"password":    node.Password,
		}
		if node.Congestion != "" {
			outbound["congestion_control"] = node.Congestion
		}
		if node.UDPRelayMode != "" {
			outbound["udp_relay_mode"] = node.UDPRelayMode
		}
		addTLS(outbound, node)
		return outbound, nil
	default:
		return nil, fmt.Errorf("node %q type %q is not renderable", node.ID, node.Type)
	}
}

func addTLS(outbound map[string]any, node managerconfig.Node) {
	if !node.TLS && node.Security != "tls" && node.Security != "reality" && node.ALPN == "" && !node.Insecure {
		return
	}
	tls := map[string]any{"enabled": true}
	if node.SNI != "" {
		tls["server_name"] = node.SNI
	}
	if node.ALPN != "" {
		tls["alpn"] = splitCSV(node.ALPN)
	}
	if node.Insecure {
		tls["insecure"] = true
	}
	if node.Security == "reality" {
		// Reality replaces the normal certificate handshake: sing-box needs the
		// server's public key and short ID, and mandates a uTLS fingerprint to
		// mimic a real client. Reality is incompatible with insecure, so drop it.
		reality := map[string]any{"enabled": true}
		if node.RealityPublicKey != "" {
			reality["public_key"] = node.RealityPublicKey
		}
		if node.RealityShortID != "" {
			reality["short_id"] = node.RealityShortID
		}
		tls["reality"] = reality
		delete(tls, "insecure")
		tls["utls"] = map[string]any{
			"enabled":     true,
			"fingerprint": firstNonEmptyString(node.Fingerprint, "chrome"),
		}
	} else if node.Fingerprint != "" {
		// A uTLS fingerprint can also apply to plain TLS outbounds.
		tls["utls"] = map[string]any{"enabled": true, "fingerprint": node.Fingerprint}
	}
	outbound["tls"] = tls
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func addTransport(outbound map[string]any, node managerconfig.Node) error {
	if node.Transport == "" || node.Transport == "tcp" {
		return nil
	}
	transport := map[string]any{"type": node.Transport}
	switch node.Transport {
	case "httpupgrade":
		if node.Host != "" {
			transport["host"] = node.Host
		}
		if node.Path != "" {
			transport["path"] = node.Path
		}
	case "ws":
		if node.Path != "" {
			transport["path"] = node.Path
		}
		if node.Host != "" {
			transport["headers"] = map[string]any{"Host": node.Host}
		}
	case "grpc":
		if node.Path != "" {
			transport["service_name"] = strings.TrimPrefix(node.Path, "/")
		}
	case "http":
		if node.Host != "" {
			transport["host"] = []string{node.Host}
		}
		if node.Path != "" {
			transport["path"] = node.Path
		}
	default:
		// sing-box only implements ws, grpc, http, httpupgrade, and quic V2Ray
		// transports. xhttp/splithttp are Xray-only and have no sing-box
		// equivalent, so a node using them cannot be rendered.
		return fmt.Errorf("%w %q (sing-box implements ws, grpc, http, httpupgrade; xhttp/splithttp are Xray-only)", ErrUnsupportedTransport, node.Transport)
	}
	outbound["transport"] = transport
	return nil
}

// vmessCipher returns value only when it is a VMess cipher sing-box accepts;
// otherwise "" so the outbound omits it (sing-box defaults to "auto"). This
// filters out transport-security markers like "tls"/"reality" that subscription
// links wrongly place in the security field.
func vmessCipher(value string) string {
	switch value {
	case "auto", "none", "zero", "aes-128-gcm", "chacha20-poly1305":
		return value
	default:
		return ""
	}
}

func splitCSV(value string) []string {
	var values []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}
