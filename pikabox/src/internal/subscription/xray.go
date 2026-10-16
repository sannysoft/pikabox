package subscription

import (
	"encoding/json"
	"fmt"
	"strings"

	managerconfig "github.com/sannysoft/pikabox/internal/config"
)

// Xray JSON subscriptions (Remnawave/v2rayN "full config" format) are a list
// of complete Xray client configs, one per server — plus "auto" configs that
// bundle several servers behind a balancer. Proxy outbounds are converted to
// sing-box through the share-URI Node model, so TLS/Reality/transport
// handling is shared with URI subscriptions.

type xrayConfig struct {
	Remarks   string         `json:"remarks"`
	Outbounds []xrayOutbound `json:"outbounds"`
}

type xrayOutbound struct {
	Tag            string          `json:"tag"`
	Protocol       string          `json:"protocol"`
	Settings       json.RawMessage `json:"settings"`
	StreamSettings xrayStream      `json:"streamSettings"`
}

type xrayStream struct {
	Network         string         `json:"network"`
	Security        string         `json:"security"`
	TLSSettings     xrayTLS        `json:"tlsSettings"`
	RealitySettings xrayTLS        `json:"realitySettings"`
	WSSettings      xrayHostPath   `json:"wsSettings"`
	HTTPUpgrade     xrayHostPath   `json:"httpupgradeSettings"`
	GRPCSettings    xrayGRPC       `json:"grpcSettings"`
	Hysteria        xrayHysteria   `json:"hysteriaSettings"`
	TCPSettings     map[string]any `json:"tcpSettings"`
}

type xrayTLS struct {
	ServerName    string   `json:"serverName"`
	Fingerprint   string   `json:"fingerprint"`
	ALPN          []string `json:"alpn"`
	AllowInsecure bool     `json:"allowInsecure"`
	PublicKey     string   `json:"publicKey"`
	ShortID       string   `json:"shortId"`
}

type xrayHostPath struct {
	Host    string            `json:"host"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
}

type xrayGRPC struct {
	ServiceName string `json:"serviceName"`
}

type xrayHysteria struct {
	Version int    `json:"version"`
	Auth    string `json:"auth"`
}

// xrayServer covers vnext (vless/vmess) and servers (trojan/shadowsocks)
// entries, and hysteria's flat settings.
type xrayServer struct {
	Address  string `json:"address"`
	Port     int    `json:"port"`
	Password string `json:"password"`
	Method   string `json:"method"`
	Users    []struct {
		ID       string `json:"id"`
		Flow     string `json:"flow"`
		Security string `json:"security"`
	} `json:"users"`
}

type xraySettings struct {
	Vnext   []xrayServer `json:"vnext"`
	Servers []xrayServer `json:"servers"`
	xrayServer
	Version int `json:"version"`
}

// looksLikeXray reports whether a JSON payload is Xray configs (outbounds
// carry "protocol"), as opposed to sing-box ("type").
func looksLikeXray(data []byte) bool {
	var probe []struct {
		Outbounds []map[string]any `json:"outbounds"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		var single struct {
			Outbounds []map[string]any `json:"outbounds"`
		}
		if json.Unmarshal(data, &single) != nil {
			return false
		}
		probe = append(probe, single)
	}
	for _, cfg := range probe {
		for _, ob := range cfg.Outbounds {
			if _, ok := ob["protocol"]; ok {
				return true
			}
		}
	}
	return false
}

// parseXrayOutbounds converts every proxy outbound in a list (or a single)
// Xray config. Single-server configs are taken first so a server is named
// after its own config ("🇩🇪 Germany | HYSTERIA2 | №1") rather than after an
// "auto" bundle it also appears in; Candidates later drops the duplicates.
func parseXrayOutbounds(data []byte) ([]Item, []string, error) {
	var configs []xrayConfig
	if err := json.Unmarshal(data, &configs); err != nil {
		var single xrayConfig
		if err := json.Unmarshal(data, &single); err != nil {
			return nil, nil, fmt.Errorf("invalid Xray JSON: %w", err)
		}
		configs = []xrayConfig{single}
	}
	var singles, bundles []Item
	var skipped []string
	for _, cfg := range configs {
		var items []Item
		proxies := 0
		for _, ob := range cfg.Outbounds {
			switch ob.Protocol {
			case "freedom", "blackhole", "dns", "loopback", "":
				continue
			}
			proxies++
			node, err := xrayNode(ob)
			if err == nil {
				var outbound map[string]any
				if outbound, err = NodeOutbound(node); err == nil {
					delete(outbound, "tag")
					items = append(items, Item{Outbound: outbound})
					continue
				}
			}
			skipped = append(skipped, err.Error())
		}
		for i := range items {
			items[i].Name = cfg.Remarks
			if proxies > 1 {
				items[i].Name = fmt.Sprintf("%s #%d", cfg.Remarks, i+1)
			}
		}
		if proxies == 1 {
			singles = append(singles, items...)
		} else {
			bundles = append(bundles, items...)
		}
	}
	return append(singles, bundles...), skipped, nil
}

func xrayNode(ob xrayOutbound) (managerconfig.Node, error) {
	var settings xraySettings
	if len(ob.Settings) > 0 {
		if err := json.Unmarshal(ob.Settings, &settings); err != nil {
			return managerconfig.Node{}, fmt.Errorf("xray %s settings: %w", ob.Protocol, err)
		}
	}
	server := settings.xrayServer
	if len(settings.Vnext) > 0 {
		server = settings.Vnext[0]
	} else if len(settings.Servers) > 0 {
		server = settings.Servers[0]
	}
	st := ob.StreamSettings
	node := managerconfig.Node{Enabled: true, Server: server.Address, Port: server.Port, Transport: normalizeTransport(st.Network)}

	switch ob.Protocol {
	case "vless", "vmess":
		node.Type = ob.Protocol
		if len(server.Users) > 0 {
			node.UUID, node.Flow = server.Users[0].ID, server.Users[0].Flow
			if ob.Protocol == "vmess" {
				node.Security = server.Users[0].Security
			}
		}
	case "trojan":
		node.Type, node.Password = "trojan", server.Password
	case "shadowsocks":
		node.Type, node.Method, node.Password = "shadowsocks", server.Method, server.Password
	case "hysteria":
		if st.Hysteria.Version != 2 && settings.Version != 2 {
			return node, fmt.Errorf("unsupported protocol \"hysteria\" v1")
		}
		node.Type, node.Password, node.Transport = "hysteria2", st.Hysteria.Auth, ""
	default:
		return node, fmt.Errorf("unsupported protocol %q", ob.Protocol)
	}

	switch node.Transport {
	case "ws":
		node.Host, node.Path = firstNonEmpty(st.WSSettings.Host, st.WSSettings.Headers["Host"]), st.WSSettings.Path
	case "httpupgrade":
		node.Host, node.Path = st.HTTPUpgrade.Host, st.HTTPUpgrade.Path
	case "grpc":
		node.Path = st.GRPCSettings.ServiceName
	case "raw":
		node.Transport = "tcp"
	}

	tlsSettings := st.TLSSettings
	switch st.Security {
	case "tls":
		node.TLS, node.Security = true, "tls"
	case "reality":
		tlsSettings = st.RealitySettings
		node.TLS, node.Security = true, "reality"
		node.RealityPublicKey, node.RealityShortID = tlsSettings.PublicKey, tlsSettings.ShortID
	}
	if node.TLS {
		node.SNI, node.Insecure = tlsSettings.ServerName, tlsSettings.AllowInsecure
		node.ALPN = strings.Join(tlsSettings.ALPN, ",")
		// sing-box has no uTLS for QUIC (hysteria2): a fingerprint there is
		// rejected, and the QUIC handshake is not a browser TLS one anyway.
		if node.Type != "hysteria2" {
			node.Fingerprint = tlsSettings.Fingerprint
		}
	}
	if node.Server == "" || node.Port == 0 {
		return node, fmt.Errorf("xray %s outbound %q has no address/port", ob.Protocol, ob.Tag)
	}
	return node, nil
}
