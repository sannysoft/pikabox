package subscription

import (
	"fmt"
	"strings"
	"unicode"

	managerconfig "github.com/sannysoft/pikabox/internal/config"
)

// XHTTP is Xray's split-HTTP transport. Plain sing-box lacks it;
// sing-box-extended implements it with the same options in snake_case
// (xPaddingBytes → x_padding_bytes). Only options that build accepts are
// passed on: sing-box rejects unknown fields, and one bad node must not break
// the whole config.

// XHTTPSupported is set by the daemon when the sing-box binary is an extended
// build. Plain sing-box rejects an xhttp transport — and with it the whole
// config — so without it XHTTP servers are skipped like before.
var XHTTPSupported bool

var xhttpModes = map[string]bool{"": true, "auto": true, "packet-up": true, "stream-up": true, "stream-one": true}

// xhttpKeys are the sing-box-extended 1.14.3 XHTTP options set from "extra".
var xhttpKeys = map[string]bool{
	"headers": true, "x_padding_bytes": true, "no_grpc_header": true, "no_sse_header": true,
	"sc_max_each_post_bytes": true, "sc_min_posts_interval_ms": true, "sc_max_buffered_posts": true,
	"sc_stream_up_server_secs": true, "xmux": true, "x_padding_obfs_mode": true, "x_padding_key": true,
	"x_padding_header": true, "x_padding_placement": true, "x_padding_method": true,
	"uplink_http_method": true, "session_placement": true, "session_key": true, "seq_placement": true,
	"seq_key": true, "uplink_data_placement": true, "uplink_data_key": true, "uplink_chunk_size": true,
}

var xmuxKeys = map[string]bool{
	"max_concurrency": true, "max_connections": true, "c_max_reuse_times": true,
	"h_max_request_times": true, "h_max_reusable_secs": true, "h_keep_alive_period": true,
}

// addXHTTP fills a sing-box "xhttp" transport from node's host, path, mode and
// Xray "extra" options.
func addXHTTP(transport map[string]any, node managerconfig.Node) error {
	if !XHTTPSupported {
		return fmt.Errorf("%w \"xhttp\" (needs sing-box-extended)", ErrUnsupportedTransport)
	}
	if !xhttpModes[node.Mode] {
		return fmt.Errorf("%w xhttp mode %q", ErrUnsupportedTransport, node.Mode)
	}
	transport["mode"] = firstNonEmpty(node.Mode, "auto")
	if node.Host != "" {
		transport["host"] = node.Host
	}
	if node.Path != "" {
		transport["path"] = node.Path
	}
	// Xray-only extras this build doesn't know are left out.
	for key, value := range node.Extra {
		name := snakeCase(key)
		switch {
		case name == "download_settings":
			// A separate download connection (possibly another server) is not
			// mapped: without it the server may refuse, so skip the node.
			return fmt.Errorf("%w xhttp downloadSettings", ErrUnsupportedTransport)
		case name == "headers":
			if headers := xhttpHeaders(value); len(headers) > 0 {
				transport["headers"] = headers
			}
		case name == "xmux":
			if xmux := xhttpXmux(value); len(xmux) > 0 {
				transport["xmux"] = xmux
			}
		case xhttpKeys[name]:
			transport[name] = value
		}
	}
	// Xray's default; sing-box-extended refuses an unset (zero) range.
	if _, set := transport["x_padding_bytes"]; !set {
		transport["x_padding_bytes"] = "100-1000"
	}
	return nil
}

// xhttpHeaders keeps string headers except Host, which sing-box rejects there
// (the host option carries it).
func xhttpHeaders(value any) map[string]string {
	raw, _ := value.(map[string]any)
	headers := map[string]string{}
	for key, v := range raw {
		if s, ok := v.(string); ok && !strings.EqualFold(key, "host") {
			headers[key] = s
		}
	}
	return headers
}

func xhttpXmux(value any) map[string]any {
	raw, _ := value.(map[string]any)
	xmux := map[string]any{}
	for key, v := range raw {
		if name := snakeCase(key); xmuxKeys[name] {
			xmux[name] = v
		}
	}
	return xmux
}

// snakeCase converts Xray's camelCase option names, keeping acronyms whole:
// noGRPCHeader → no_grpc_header, uplinkHTTPMethod → uplink_http_method.
func snakeCase(name string) string {
	runes := []rune(name)
	var out strings.Builder
	for i, r := range runes {
		if unicode.IsUpper(r) {
			prevLower := i > 0 && !unicode.IsUpper(runes[i-1])
			acronymEnd := i > 0 && unicode.IsUpper(runes[i-1]) && i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if prevLower || acronymEnd {
				out.WriteByte('_')
			}
			r = unicode.ToLower(r)
		}
		out.WriteRune(r)
	}
	return out.String()
}
