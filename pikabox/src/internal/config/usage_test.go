package config

import (
	"reflect"
	"testing"
)

func TestUsedBy(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Manager.Final = "vp3"
	cfg.OutboundGroups["foreign"] = OutboundGroup{ID: "foreign", Name: "Foreign", Members: []string{"vp3", "sub"}}
	cfg.OutboundGroups["other"] = OutboundGroup{ID: "other", Members: []string{"sub"}}
	cfg.RuleGroups["ps"] = RuleGroup{ID: "ps", Name: "PlayStation"}
	cfg.Routes = []Route{{ID: "r1", RuleGroup: "ps", Outbound: "direct"}, {ID: "r2", RuleGroup: "ps", Outbound: "vp3"}}
	cfg.DNSServers["cf"] = DNSServer{ID: "cf", Name: "Cloudflare", Detour: "vp3"}
	cfg.Transparent.Devices = []Device{{ID: "tv", Name: "TV", Group: "vp3"}}

	want := []Usage{
		{Kind: "default"},
		{Kind: "group", ID: "foreign", Name: "Foreign"},
		{Kind: "route", ID: "r2", Name: "PlayStation", Position: 2},
		{Kind: "dns", ID: "cf", Name: "Cloudflare"},
		{Kind: "device", ID: "tv", Name: "TV"},
	}
	if got := cfg.UsedBy("vp3"); !reflect.DeepEqual(got, want) {
		t.Fatalf("UsedBy(vp3) = %+v, want %+v", got, want)
	}
	if got := cfg.UsedBy("unused"); len(got) != 0 {
		t.Fatalf("UsedBy(unused) = %+v, want none", got)
	}
}
