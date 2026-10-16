package main

import (
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	managerconfig "github.com/openwrt-singbox/singbox-manager/internal/config"
)

func TestCmdlineHasArgMatchesExactRuntimeConfig(t *testing.T) {
	cmdline := []byte("/usr/bin/sing-box\x00run\x00-c\x00/var/run/sing-box/config.json\x00")
	if !cmdlineHasArg(cmdline, "/var/run/sing-box/config.json") {
		t.Fatal("expected runtime config argument to match")
	}
	if cmdlineHasArg(cmdline, "/tmp/other-config.json") {
		t.Fatal("unexpected match for other config")
	}
}

func TestCollectDevicesDropsInfraAndKeepsHosts(t *testing.T) {
	leases := []Device{
		{IP: "192.168.1.10", MAC: "aa:bb:cc:dd:ee:10", Name: "laptop", Source: "dhcp"},
		{IP: "192.168.1.1", MAC: "aa:bb:cc:dd:ee:01", Name: "router", Source: "dhcp"},
	}
	arp := []Device{
		{IP: "192.168.1.20", MAC: "aa:bb:cc:dd:ee:20", Source: "arp"},
		{IP: "192.168.1.10", MAC: "aa:bb:cc:dd:ee:10", Source: "arp"},  // dup of lease
		{IP: "192.168.1.255", MAC: "ff:ff:ff:ff:ff:ff", Source: "arp"}, // broadcast
		{IP: "224.0.0.251", MAC: "01:00:5e:00:00:fb", Source: "arp"},   // multicast
		{IP: "169.254.1.5", MAC: "aa:bb:cc:dd:ee:fe", Source: "arp"},   // link-local
	}
	skip := map[string]bool{
		"192.168.1.1":   true, // gateway / router self
		"192.168.1.255": true, // directed broadcast
	}

	got := collectDevices(leases, arp, skip)
	if len(got) != 2 {
		t.Fatalf("collectDevices returned %d devices, want 2: %#v", len(got), got)
	}
	if got[0].IP != "192.168.1.10" || got[1].IP != "192.168.1.20" {
		t.Fatalf("devices = %#v, want sorted 192.168.1.10, 192.168.1.20", got)
	}
}

func TestBroadcastIP(t *testing.T) {
	cases := []struct {
		cidr string
		want string
	}{
		{"192.168.1.1/24", "192.168.1.255"},
		{"10.0.0.1/8", "10.255.255.255"},
		{"172.16.5.4/30", "172.16.5.7"},
		{"fe80::1/64", ""}, // IPv6 has no broadcast
	}
	for _, tc := range cases {
		ip, ipNet, err := net.ParseCIDR(tc.cidr)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.cidr, err)
		}
		ipNet.IP = ip // use the host address, not the network address
		if got := broadcastIP(ipNet); got != tc.want {
			t.Errorf("broadcastIP(%q) = %q, want %q", tc.cidr, got, tc.want)
		}
	}
}

func TestDefaultGatewaysParsesLittleEndianHex(t *testing.T) {
	dir := t.TempDir()
	routePath := filepath.Join(dir, "route")
	// Default route via 192.168.2.1; the LAN route has no gateway (00000000).
	route := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"eth0\t00000000\t0102A8C0\t0003\t0\t0\t0\t00000000\t0\t0\t0\n" +
		"br-lan\t0002A8C0\t00000000\t0001\t0\t0\t0\t00FFFFFF\t0\t0\t0\n"
	if err := os.WriteFile(routePath, []byte(route), 0644); err != nil {
		t.Fatalf("write route: %v", err)
	}

	gws := defaultGateways(routePath)
	if len(gws) != 1 || gws[0] != "192.168.2.1" {
		t.Fatalf("defaultGateways = %#v, want [192.168.2.1]", gws)
	}
}

func TestIsPickableHostIP(t *testing.T) {
	pickable := []string{"192.168.1.10", "10.0.0.5", "2001:db8::5"}
	for _, ip := range pickable {
		if !isPickableHostIP(ip) {
			t.Errorf("isPickableHostIP(%q) = false, want true", ip)
		}
	}
	rejected := []string{"", "not-an-ip", "0.0.0.0", "127.0.0.1", "255.255.255.255", "224.0.0.1", "169.254.0.1", "fe80::1", "::1"}
	for _, ip := range rejected {
		if isPickableHostIP(ip) {
			t.Errorf("isPickableHostIP(%q) = true, want false", ip)
		}
	}
}

func TestSelectFastest(t *testing.T) {
	candidates := []managerconfig.Candidate{{ID: "s.a"}, {ID: "s.b"}, {ID: "s.c"}, {ID: "s.d"}}
	results := map[string]managerconfig.ProbeResult{
		"s.a": {Health: "ok", LatencyMS: 300},
		"s.b": {Health: "down"},
		"s.c": {Health: "ok", LatencyMS: 100},
		"s.d": {Health: "ok", LatencyMS: 200},
	}
	cases := []struct {
		count int
		want  []string
	}{
		{2, []string{"s.c", "s.d"}},
		{0, []string{"s.c", "s.d", "s.a"}},
		{10, []string{"s.c", "s.d", "s.a"}},
	}
	for _, tc := range cases {
		if got := selectFastest(candidates, results, tc.count); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("selectFastest(count=%d) = %v, want %v", tc.count, got, tc.want)
		}
	}
	if got := selectFastest(candidates, nil, 3); len(got) != 0 {
		t.Errorf("no results must select nothing, got %v", got)
	}
}

func TestSameSet(t *testing.T) {
	cases := []struct {
		a, b []string
		want bool
	}{
		{[]string{"a", "b"}, []string{"b", "a"}, true},
		{nil, []string{}, true},
		{[]string{"a"}, []string{"a", "b"}, false},
		{[]string{"a", "b"}, []string{"a", "c"}, false},
	}
	for _, tc := range cases {
		if got := sameSet(tc.a, tc.b); got != tc.want {
			t.Errorf("sameSet(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
