package subscription

import (
	"encoding/base64"
	"reflect"
	"strings"
	"testing"
	"time"

	managerconfig "github.com/sannysoft/pikabox/internal/config"
)

func TestParseOutboundsBase64URIList(t *testing.T) {
	payload := strings.Join([]string{
		"vless://00000000-0000-0000-0000-000000000001@example.com:443?security=tls&sni=edge.example.com&type=ws&host=cdn.example.com&path=%2Fws#VLESS",
		"trojan://secret@example.org:443?security=tls&sni=trojan.example.org#Trojan",
		"ss://YWVzLTEyOC1nY206cGFzc0Bzcy5leGFtcGxlLmNvbTo4Mzg4#SS",
		"hysteria2://hy-pass@hy.example.net:443?sni=hy.example.net&insecure=1#HY2",
		"tuic://00000000-0000-0000-0000-000000000002:tuic-pass@tuic.example.net:443?congestion_control=bbr&udp_relay_mode=native&sni=tuic.example.net#TUIC",
		"bogus://nothing",
	}, "\n")
	items, _, err := ParseOutbounds([]byte(base64.StdEncoding.EncodeToString([]byte(payload))), "auto")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 5 {
		t.Fatalf("items = %d, want 5 (bogus line skipped)", len(items))
	}
	names := []string{}
	for _, item := range items {
		names = append(names, item.Name)
		if _, hasTag := item.Outbound["tag"]; hasTag {
			t.Fatalf("%s: tag must be left for the caller", item.Name)
		}
	}
	if !reflect.DeepEqual(names, []string{"VLESS", "Trojan", "SS", "HY2", "TUIC"}) {
		t.Fatalf("names = %v", names)
	}
	vless := items[0].Outbound
	transport := vless["transport"].(map[string]any)
	if vless["type"] != "vless" || vless["server"] != "example.com" || vless["server_port"] != 443 || transport["type"] != "ws" {
		t.Fatalf("vless = %#v", vless)
	}
	if tls := vless["tls"].(map[string]any); tls["server_name"] != "edge.example.com" {
		t.Fatalf("vless tls = %#v", tls)
	}
	if ss := items[2].Outbound; ss["type"] != "shadowsocks" || ss["method"] != "aes-128-gcm" || ss["password"] != "pass" {
		t.Fatalf("ss = %#v", ss)
	}
	if tuic := items[4].Outbound; tuic["congestion_control"] != "bbr" || tuic["udp_relay_mode"] != "native" {
		t.Fatalf("tuic = %#v", tuic)
	}
	if _, _, err := ParseOutbounds([]byte("bogus://a\nalso://b"), "plain"); err == nil {
		t.Fatal("a payload with no parsable line must fail")
	}
}

func TestParseOutboundsSingBoxJSONPassthrough(t *testing.T) {
	config := `{
		"outbounds": [
			{"type": "selector", "tag": "proxy", "outbounds": ["a"]},
			{"type": "urltest", "tag": "auto", "outbounds": ["a"]},
			{"type": "direct", "tag": "direct"},
			{"type": "dns", "tag": "dns-out"},
			{"type": "vless", "tag": "NL reality", "server": "nl.example.com", "server_port": 443, "uuid": "u",
				"flow": "xtls-rprx-vision", "detour": "proxy", "domain_resolver": "dns-remote",
				"tls": {"enabled": true, "server_name": "www.mozilla.org", "reality": {"enabled": true, "public_key": "PK", "short_id": "ab"}, "utls": {"enabled": true, "fingerprint": "chrome"}}},
			{"type": "anytls", "server": "any.example.com", "server_port": 443, "password": "p"},
			{"type": "vless", "tag": "no server"}
		]
	}`
	items, _, err := ParseOutbounds([]byte(config), "auto")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %#v, want 2 proxies", items)
	}
	reality := items[0]
	if reality.Name != "NL reality" {
		t.Fatalf("name = %q", reality.Name)
	}
	for _, stripped := range []string{"tag", "detour", "domain_resolver"} {
		if _, ok := reality.Outbound[stripped]; ok {
			t.Fatalf("%s must be stripped: %#v", stripped, reality.Outbound)
		}
	}
	tls := reality.Outbound["tls"].(map[string]any)
	if tls["reality"].(map[string]any)["public_key"] != "PK" || reality.Outbound["flow"] != "xtls-rprx-vision" {
		t.Fatalf("options must survive verbatim: %#v", reality.Outbound)
	}
	if items[1].Name != "any.example.com" || items[1].Outbound["type"] != "anytls" {
		t.Fatalf("protocol unknown to the URI parser must still pass through: %#v", items[1])
	}

	bare, _, err := ParseOutbounds([]byte(base64.StdEncoding.EncodeToString([]byte(`[{"type":"shadowsocks","tag":"ss","server":"ss.example.com","server_port":8388,"method":"aes-128-gcm","password":"pass"}]`))), "auto")
	if err != nil || len(bare) != 1 || bare[0].Name != "ss" {
		t.Fatalf("base64 bare array = %#v, %v", bare, err)
	}
	if _, _, err := ParseOutbounds([]byte(`{"outbounds":[{"type":"direct","tag":"d"}]}`), "auto"); err == nil {
		t.Fatal("config without proxies must fail")
	}
}

func TestParseSource(t *testing.T) {
	item, err := ParseSource(`  {"type":"trojan","tag":"x","server":"t.example.com","server_port":443,"password":"p"} `)
	if err != nil || item.Name != "x" || item.Outbound["server"] != "t.example.com" {
		t.Fatalf("json source = %#v, %v", item, err)
	}
	if _, ok := item.Outbound["tag"]; ok {
		t.Fatal("tag must be stripped")
	}
	item, err = ParseSource("trojan://secret@example.org:443#Mine")
	if err != nil || item.Name != "Mine" || item.Outbound["type"] != "trojan" {
		t.Fatalf("uri source = %#v, %v", item, err)
	}
	for _, bad := range []string{`{"server":"x"}`, `{nope`, "http://example.com", ""} {
		if _, err := ParseSource(bad); err == nil {
			t.Errorf("ParseSource(%q) must fail", bad)
		}
	}
}

func TestCandidatesStableIDsAndDedup(t *testing.T) {
	items := []Item{
		{Name: "A", Outbound: map[string]any{"type": "trojan", "server": "a", "server_port": 1}},
		{Name: "B", Outbound: map[string]any{"type": "trojan", "server": "b", "server_port": 1}},
		{Name: "A again", Outbound: map[string]any{"type": "trojan", "server": "a", "server_port": 1}},
	}
	first := Candidates("sub1", items)
	if len(first) != 2 {
		t.Fatalf("candidates = %#v, want duplicate collapsed", first)
	}
	reordered := Candidates("sub1", []Item{items[1], items[0]})
	if reordered[0].ID != first[1].ID || reordered[1].ID != first[0].ID {
		t.Fatal("IDs must depend on the definition, not the position")
	}
	for _, c := range first {
		if !strings.HasPrefix(c.ID, "sub1.") || c.Outbound["tag"] != c.ID {
			t.Fatalf("candidate = %#v", c)
		}
	}
	if _, tagged := items[0].Outbound["tag"]; tagged {
		t.Fatal("Candidates must not mutate the input outbound")
	}
}

func TestMatch(t *testing.T) {
	candidates := Candidates("s", []Item{
		{Name: "NL 1", Outbound: map[string]any{"type": "vless", "server": "nl", "server_port": float64(443)}},
		{Name: "US 1", Outbound: map[string]any{"type": "trojan", "server": "us", "server_port": float64(443)}},
		{Name: "NL 2", Outbound: map[string]any{"type": "vless", "server": "nl2", "server_port": float64(8443)}},
	})
	cases := []struct {
		filter string
		want   int
	}{
		{"", 3},
		{`type == "vless"`, 2},
		{`name matches "^NL" && port == 443`, 1},
		{`outbound.tls.enabled == true`, 0}, // missing nested field: rejected, no error
	}
	for _, tc := range cases {
		ids, err := Match(tc.filter, candidates)
		if err != nil || len(ids) != tc.want {
			t.Errorf("Match(%q) = %v, %v; want %d", tc.filter, ids, err, tc.want)
		}
	}
	if _, err := Match("type ==", candidates); err == nil {
		t.Fatal("bad filter must error")
	}
}

func TestNodeOutboundVMessSecurityTLSIsNotACipher(t *testing.T) {
	ob, err := NodeOutbound(managerconfig.Node{ID: "v", Type: "vmess", Server: "example.com", Port: 443, UUID: "u", Security: "tls", TLS: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ob["security"]; ok {
		t.Fatalf("security=tls must not become a cipher: %v", ob["security"])
	}
	if _, ok := ob["tls"]; !ok {
		t.Fatal("expected TLS block")
	}
	ob, _ = NodeOutbound(managerconfig.Node{ID: "v2", Type: "vmess", Server: "example.com", Port: 443, UUID: "u", Security: "aes-128-gcm"})
	if ob["security"] != "aes-128-gcm" {
		t.Fatalf("cipher = %v", ob["security"])
	}
}

func TestNodeOutboundReality(t *testing.T) {
	ob, err := NodeOutbound(managerconfig.Node{
		ID: "r", Type: "vless", Server: "example.com", Port: 9443, UUID: "u",
		Security: "reality", SNI: "www.speedtest.net", RealityPublicKey: "PUBKEY", RealityShortID: "sid123", Insecure: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	tls := ob["tls"].(map[string]any)
	reality := tls["reality"].(map[string]any)
	if reality["public_key"] != "PUBKEY" || reality["short_id"] != "sid123" {
		t.Fatalf("reality = %v", reality)
	}
	if utls := tls["utls"].(map[string]any); utls["fingerprint"] != "chrome" {
		t.Fatalf("expected default chrome fingerprint, got %v", utls)
	}
	if _, ok := tls["insecure"]; ok {
		t.Fatal("insecure must not be set alongside reality")
	}
}

func TestNodeOutboundTransports(t *testing.T) {
	ob, err := NodeOutbound(managerconfig.Node{ID: "n", Type: "vless", Server: "1.2.3.4", Port: 443, UUID: "u", TLS: true, Transport: "http", Host: "h.example", Path: "/p"})
	if err != nil {
		t.Fatal(err)
	}
	if tr := ob["transport"].(map[string]any); tr["type"] != "http" || tr["path"] != "/p" {
		t.Fatalf("transport = %#v", tr)
	}
	_, err = NodeOutbound(managerconfig.Node{ID: "x", Type: "vless", Server: "1.2.3.4", Port: 443, UUID: "u", Transport: "xhttp"})
	if err == nil || !strings.Contains(err.Error(), "xhttp") {
		t.Fatalf("err = %v, want xhttp unsupported", err)
	}
	if _, err := NodeOutbound(managerconfig.Node{ID: "z", Type: "wireguard"}); err == nil {
		t.Fatal("unknown type must fail")
	}
}

func TestParseVMessDoesNotUseTransportHostAsSNI(t *testing.T) {
	payload := `{"v":"2","ps":"VMess","add":"edge.example.com","port":"80","id":"00000000-0000-0000-0000-000000000001","aid":"0","scy":"auto","net":"httpupgrade","type":"none","host":"front.example.com","path":"/upgrade","tls":""}`
	uri := "vmess://" + base64.StdEncoding.EncodeToString([]byte(payload))

	node, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("parse vmess: %v", err)
	}
	if node.Transport != "httpupgrade" || node.Host != "front.example.com" || node.Path != "/upgrade" {
		t.Fatalf("vmess transport fields = %#v", node)
	}
	if node.SNI != "" {
		t.Fatalf("SNI = %q, want empty when only transport host is set", node.SNI)
	}
	if node.TLS {
		t.Fatal("TLS = true, want false for vmess tls empty")
	}
}

func TestParseVMessStandardURI(t *testing.T) {
	uri := "vmess://abd4fd93-f23d-4c2e-ac8a-bce395947e6d@104.21.33.234:80?type=httpupgrade&host=v2.mokhtari94.ir&path=/qlb4hqR5wdecX3r8BQxdso&packetEncoding=xudp#Auto%20http%20httpupgrade%20CDN%20vmess"

	node, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("parse standard vmess: %v", err)
	}
	if node.Type != "vmess" {
		t.Fatalf("type = %q, want vmess", node.Type)
	}
	if node.UUID != "abd4fd93-f23d-4c2e-ac8a-bce395947e6d" {
		t.Fatalf("uuid = %q", node.UUID)
	}
	if node.Server != "104.21.33.234" || node.Port != 80 {
		t.Fatalf("server/port = %s:%d", node.Server, node.Port)
	}
	if node.Transport != "httpupgrade" || node.Host != "v2.mokhtari94.ir" || node.Path != "/qlb4hqR5wdecX3r8BQxdso" {
		t.Fatalf("transport fields = %#v", node)
	}
	if node.TLS {
		t.Fatal("TLS = true, want false on a port-80 vmess link with no security param")
	}
	if node.Security != "auto" {
		t.Fatalf("cipher = %q, want auto (must not be the TLS setting)", node.Security)
	}
}

func TestDueUsesSubscriptionUpdateInterval(t *testing.T) {
	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	source := managerconfig.Subscription{
		ID:             "sub",
		Enabled:        true,
		URL:            "https://example.com/sub",
		UpdateInterval: "2h",
		LastUpdate:     now.Add(-3 * time.Hour).Format(time.RFC3339),
	}
	if !Due(source, now) {
		t.Fatal("expected subscription to be due")
	}

	source.LastUpdate = now.Add(-30 * time.Minute).Format(time.RFC3339)
	if Due(source, now) {
		t.Fatal("expected subscription not to be due")
	}

	source.Enabled = false
	if Due(source, now) {
		t.Fatal("disabled subscription should not be due")
	}
}

func TestParseOutboundsSkipsUnsupportedEntries(t *testing.T) {
	payload := "vless://u@a.example:443?type=xhttp&security=reality#x1\n" +
		"vless://u@b.example:443?type=xhttp&security=reality#x2\n" +
		"trojan://pw@c.example:443?security=tls&sni=c.example#ok"
	items, skipped, err := ParseOutbounds([]byte(payload), "plain")
	if err != nil || len(items) != 1 || items[0].Name != "ok" {
		t.Fatalf("items=%v err=%v", items, err)
	}
	if !strings.HasPrefix(skipped, "2× ") || !strings.Contains(skipped, "xhttp") {
		t.Fatalf("skipped = %q", skipped)
	}
	// Only unsupported entries: a valid (empty) result, not an error.
	items, skipped, err = ParseOutbounds([]byte("vless://u@a.example:443?type=xhttp#x"), "plain")
	if err != nil || len(items) != 0 || skipped == "" {
		t.Fatalf("all-unsupported: items=%v skipped=%q err=%v", items, skipped, err)
	}
	// Not a subscription at all (an HTML page) is still an error.
	if _, _, err := ParseOutbounds([]byte("<!DOCTYPE html><html></html>"), "auto"); err == nil {
		t.Fatal("HTML payload must be an error")
	}
}

func TestParseOutboundsXrayJSONConfigs(t *testing.T) {
	payload := `[
	 {"remarks":"Auto","outbounds":[
	   {"tag":"a","protocol":"hysteria","settings":{"address":"h1.example","port":3443,"version":2},
	    "streamSettings":{"network":"hysteria","hysteriaSettings":{"version":2,"auth":"pw1"},"security":"tls","tlsSettings":{"serverName":"h1.example","fingerprint":"chrome","alpn":["h3"]}}},
	   {"tag":"b","protocol":"vless","settings":{"vnext":[{"address":"v1.example","port":8443,"users":[{"id":"u1","flow":""}]}]},
	    "streamSettings":{"network":"xhttp","security":"reality","realitySettings":{"serverName":"x.com","publicKey":"pk","shortId":"ab"}}},
	   {"protocol":"freedom"},{"protocol":"blackhole"}]},
	 {"remarks":"DE | HYSTERIA2","outbounds":[
	   {"tag":"proxy","protocol":"hysteria","settings":{"address":"h1.example","port":3443,"version":2},
	    "streamSettings":{"network":"hysteria","hysteriaSettings":{"version":2,"auth":"pw1"},"security":"tls","tlsSettings":{"serverName":"h1.example","fingerprint":"chrome","alpn":["h3"]}}},
	   {"protocol":"freedom"}]},
	 {"remarks":"NL | VLESS","outbounds":[
	   {"tag":"proxy","protocol":"vless","settings":{"vnext":[{"address":"v2.example","port":443,"users":[{"id":"u2","flow":"xtls-rprx-vision"}]}]},
	    "streamSettings":{"network":"tcp","security":"reality","realitySettings":{"serverName":"www.mozilla.org","publicKey":"pk2","shortId":"cd","fingerprint":"chrome"}}}]}
	]`
	items, skipped, err := ParseOutbounds([]byte(payload), "auto")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(skipped, "xhttp") {
		t.Fatalf("xhttp entry must be skipped, got %q", skipped)
	}
	candidates := Candidates("imp", items)
	if len(candidates) != 2 {
		t.Fatalf("want hysteria2 (deduped) + vless, got %d: %#v", len(candidates), candidates)
	}
	hy := candidates[0]
	if hy.Name != "DE | HYSTERIA2" {
		t.Fatalf("a server must be named after its own config, got %q", hy.Name)
	}
	tls, _ := hy.Outbound["tls"].(map[string]any)
	if hy.Outbound["type"] != "hysteria2" || hy.Outbound["password"] != "pw1" || hy.Outbound["server_port"] != 3443 ||
		tls["server_name"] != "h1.example" || tls["utls"] != nil {
		t.Fatalf("hysteria2 = %#v", hy.Outbound)
	}
	vl := candidates[1].Outbound
	vtls, _ := vl["tls"].(map[string]any)
	reality, _ := vtls["reality"].(map[string]any)
	if vl["type"] != "vless" || vl["flow"] != "xtls-rprx-vision" || reality["public_key"] != "pk2" || reality["short_id"] != "cd" {
		t.Fatalf("vless reality = %#v", vl)
	}
}
