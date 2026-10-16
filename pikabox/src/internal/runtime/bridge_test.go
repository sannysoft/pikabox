package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDisableBridgeNetfilter(t *testing.T) {
	dir := t.TempDir()
	on := filepath.Join(dir, "bridge-nf-call-iptables")
	off := filepath.Join(dir, "bridge-nf-call-ip6tables")
	missing := filepath.Join(dir, "absent")
	os.WriteFile(on, []byte("1\n"), 0644)
	os.WriteFile(off, []byte("0\n"), 0644)
	saved := bridgeNetfilterSysctls
	bridgeNetfilterSysctls = []string{on, off, missing}
	defer func() { bridgeNetfilterSysctls = saved }()

	disableBridgeNetfilter()

	for _, path := range []string{on, off} {
		if data, _ := os.ReadFile(path); string(data) != "0\n" {
			t.Fatalf("%s = %q, want 0", filepath.Base(path), data)
		}
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("absent sysctl must not be created")
	}
}
