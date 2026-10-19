package health

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	managerconfig "github.com/sannysoft/pikabox/internal/config"
)

const DefaultTestURL = "https://www.gstatic.com/generate_204"

var httpClient = http.DefaultClient

type EndpointResult struct {
	ID        string `json:"id"`
	Health    string `json:"health"`
	LatencyMS int    `json:"latency_ms"`
	Method    string `json:"method,omitempty"`
	Error     string `json:"error,omitempty"`
	// Bulk is set when a bulk download check ran for this outbound.
	Bulk  *BulkResult  `json:"bulk,omitempty"`
	Speed *SpeedResult `json:"speed,omitempty"`
}

func TestURL(ctx context.Context, url string) (EndpointResult, error) {
	if url == "" {
		url = DefaultTestURL
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	started := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return EndpointResult{ID: url, Health: "error", Error: err.Error()}, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return EndpointResult{ID: url, Health: "error", Error: err.Error()}, err
	}
	defer resp.Body.Close()

	latency := latencyMS(started)
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return EndpointResult{ID: url, Health: "ok", LatencyMS: latency}, nil
	}
	err = fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
	return EndpointResult{ID: url, Health: "error", LatencyMS: latency, Error: err.Error()}, err
}

func TestDNS(ctx context.Context, server managerconfig.DNSServer, domain string) (EndpointResult, error) {
	if domain == "" {
		domain = "example.com"
	}
	dnsType := normalizeDNSType(server.Type)
	host, port := parseDNSAddress(server.Address, dnsType)
	if host == "" {
		err := fmt.Errorf("dns server address is required")
		return EndpointResult{ID: server.ID, Health: "error", Error: err.Error()}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	started := time.Now()

	if dnsType == "udp" || dnsType == "tcp" {
		resolver := net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network string, address string) (net.Conn, error) {
				var dialer net.Dialer
				return dialer.DialContext(ctx, dnsType, net.JoinHostPort(host, strconv.Itoa(port)))
			},
		}
		if _, err := resolver.LookupHost(ctx, domain); err != nil {
			return EndpointResult{ID: server.ID, Health: "down", LatencyMS: latencyMS(started), Error: err.Error()}, err
		}
		return EndpointResult{ID: server.ID, Health: "ok", LatencyMS: latencyMS(started)}, nil
	}

	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return EndpointResult{ID: server.ID, Health: "down", LatencyMS: latencyMS(started), Error: err.Error()}, err
	}
	_ = conn.Close()
	return EndpointResult{ID: server.ID, Health: "ok", LatencyMS: latencyMS(started)}, nil
}

func latencyMS(started time.Time) int {
	elapsed := time.Since(started)
	ms := int(elapsed / time.Millisecond)
	if ms < 1 {
		return 1
	}
	return ms
}

func normalizeDNSType(value string) string {
	switch value {
	case "doh", "https":
		return "https"
	case "dot", "tls":
		return "tls"
	case "doq", "quic":
		return "quic"
	case "tcp":
		return "tcp"
	default:
		return "udp"
	}
}

func parseDNSAddress(address string, dnsType string) (string, int) {
	if address == "" {
		return "", defaultDNSPort(dnsType)
	}
	parsed, err := url.Parse(address)
	if err == nil && parsed.Scheme != "" {
		host := parsed.Hostname()
		port := parseOptionalPort(parsed.Port())
		if host == "" && parsed.Opaque != "" {
			host, port = splitDNSHostPort(parsed.Opaque)
		}
		return host, valueOrDefaultPort(port, defaultDNSPort(dnsType))
	}
	host, port := splitDNSHostPort(address)
	return host, valueOrDefaultPort(port, defaultDNSPort(dnsType))
}

func splitDNSHostPort(address string) (string, int) {
	host, portText, err := net.SplitHostPort(address)
	if err == nil {
		return host, parseOptionalPort(portText)
	}
	if strings.Count(address, ":") == 1 {
		host, portText, found := strings.Cut(address, ":")
		if found {
			return host, parseOptionalPort(portText)
		}
	}
	return address, 0
}

func parseOptionalPort(value string) int {
	if value == "" {
		return 0
	}
	port, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return port
}

func valueOrDefaultPort(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func defaultDNSPort(dnsType string) int {
	switch dnsType {
	case "https":
		return 443
	case "tls", "quic":
		return 853
	default:
		return 53
	}
}
