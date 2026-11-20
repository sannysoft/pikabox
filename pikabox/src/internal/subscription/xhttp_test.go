package subscription

import (
	"reflect"
	"strings"
	"testing"
)

func init() { XHTTPSupported = true }

func TestSnakeCase(t *testing.T) {
	for in, want := range map[string]string{
		"xPaddingBytes": "x_padding_bytes", "noGRPCHeader": "no_grpc_header", "noSSEHeader": "no_sse_header",
		"uplinkHTTPMethod": "uplink_http_method", "hMaxRequestTimes": "h_max_request_times",
		"scMaxEachPostBytes": "sc_max_each_post_bytes", "downloadSettings": "download_settings", "headers": "headers",
	} {
		if got := snakeCase(in); got != want {
			t.Errorf("snakeCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestXHTTPFromShareLink(t *testing.T) {
	link := `vless://u@x.example:443?type=xhttp&security=reality&sni=www.mozilla.org&pbk=pk&sid=ab&fp=chrome&path=%2Fapi&host=cdn.example&mode=packet-up` +
		`&extra=%7B%22xPaddingBytes%22%3A%22100-1000%22%2C%22noGRPCHeader%22%3Atrue%2C%22headers%22%3A%7B%22Host%22%3A%22h%22%2C%22X-A%22%3A%22b%22%7D%2C%22xmux%22%3A%7B%22maxConcurrency%22%3A%2216-32%22%7D%2C%22unknownOpt%22%3A1%7D#x`
	items, skipped, err := ParseOutbounds([]byte(link), "plain")
	if err != nil || skipped != "" || len(items) != 1 {
		t.Fatalf("items=%v skipped=%q err=%v", items, skipped, err)
	}
	want := map[string]any{
		"type": "xhttp", "mode": "packet-up", "host": "cdn.example", "path": "/api",
		"x_padding_bytes": "100-1000", "no_grpc_header": true,
		"headers": map[string]string{"X-A": "b"}, // Host is carried by "host"
		"xmux":    map[string]any{"max_concurrency": "16-32"},
		// unknownOpt dropped: sing-box rejects unknown fields
	}
	if got := items[0].Outbound["transport"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("transport = %#v\nwant %#v", got, want)
	}
}

func TestXHTTPFromXrayJSON(t *testing.T) {
	payload := `[{"remarks":"FI | XHTTP","outbounds":[{"tag":"proxy","protocol":"vless",
	  "settings":{"vnext":[{"address":"x.example","port":443,"users":[{"id":"u","flow":""}]}]},
	  "streamSettings":{"network":"xhttp","security":"reality",
	    "realitySettings":{"serverName":"www.mozilla.org","publicKey":"pk","shortId":"ab","fingerprint":"chrome"},
	    "xhttpSettings":{"path":"/x","mode":"auto","extra":{"scMaxEachPostBytes":1000000}}}}]}]`
	items, skipped, err := ParseOutbounds([]byte(payload), "auto")
	if err != nil || skipped != "" || len(items) != 1 {
		t.Fatalf("items=%v skipped=%q err=%v", items, skipped, err)
	}
	tr := items[0].Outbound["transport"].(map[string]any)
	if tr["type"] != "xhttp" || tr["path"] != "/x" || tr["mode"] != "auto" || tr["sc_max_each_post_bytes"] != float64(1000000) ||
		tr["x_padding_bytes"] != "100-1000" { // default filled in: sing-box-extended rejects it unset
		t.Fatalf("transport = %#v", tr)
	}
}

func TestXHTTPSkipsUnmappable(t *testing.T) {
	for _, link := range []string{
		"vless://u@x.example:443?type=xhttp&mode=bogus#bad-mode",
		"vless://u@x.example:443?type=xhttp&extra=%7B%22downloadSettings%22%3A%7B%7D%7D#split-download",
		"vless://u@x.example:443?type=splithttp&extra=not-json#bad-extra",
	} {
		items, skipped, err := ParseOutbounds([]byte(link), "plain")
		if err != nil || len(items) != 0 || skipped == "" {
			t.Errorf("%s: items=%v skipped=%q err=%v", link, items, skipped, err)
		}
	}
}

func TestXHTTPNeedsExtended(t *testing.T) {
	XHTTPSupported = false
	defer func() { XHTTPSupported = true }()
	items, skipped, err := ParseOutbounds([]byte("vless://u@x.example:443?type=xhttp#x"), "plain")
	if err != nil || len(items) != 0 || !strings.Contains(skipped, "sing-box-extended") {
		t.Fatalf("plain sing-box must skip xhttp: items=%v skipped=%q err=%v", items, skipped, err)
	}
}
