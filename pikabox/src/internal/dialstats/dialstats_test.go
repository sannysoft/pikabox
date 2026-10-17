package dialstats

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var servers = map[string]string{
	"vp3": "84.32.149.31:443",
	"hk":  "hk.vpn.lease:61201",
}

func TestFeedCountsServerFailuresOnly(t *testing.T) {
	tr := New()
	now := time.Unix(1_800_000_000, 0)
	lines := []string{
		// two attempts through vp3; sing-box logs the first twice
		`+0000 2026-10-17 17:54:23 INFO [1 1ms] outbound/vless[vp3]: outbound connection to example.com:443`,
		`+0000 2026-10-17 17:54:23 INFO [1 40ms] outbound/vless[vp3]: outbound connection to example.com:443`,
		`+0000 2026-10-17 17:54:23 INFO [2 1ms] outbound/vless[vp3]: outbound packet connection to 8.8.8.8:53`,
		// the server itself timed out: counts
		`+0000 2026-10-17 17:54:28 ERROR [1 5.1s] connection: open connection to example.com:443 using outbound/vless[vp3]: dial tcp 84.32.149.31:443: i/o timeout`,
		// a delay probe (no connection ID): ignored
		`+0000 2026-10-17 17:54:30 INFO outbound/vless[vp3]: outbound connection to www.gstatic.com:443`,
		// hostname server: matched on port; a dial to another port is the destination
		`+0000 2026-10-17 17:54:31 INFO [3 0ms] outbound/trojan[hk]: outbound connection to a.com:443`,
		`+0000 2026-10-17 17:54:36 ERROR [3 5.0s] connection: open connection to a.com:443 using outbound/trojan[hk]: dial tcp 103.1.2.3:61201: i/o timeout`,
		`+0000 2026-10-17 17:54:37 INFO [4 0ms] outbound/trojan[hk]: outbound connection to b.com:80`,
		`+0000 2026-10-17 17:54:42 ERROR [4 5.0s] connection: open connection to b.com:80 using outbound/trojan[hk]: dial tcp 1.1.1.1:80: connection refused`,
		// direct is not a proxy: ignored
		`+0000 2026-10-17 17:54:43 INFO [5 0ms] outbound/direct[direct]: outbound connection to 1.2.3.4:6881`,
		`+0000 2026-10-17 17:54:48 ERROR [5 5.0s] connection: open connection to 1.2.3.4:6881 using outbound/direct[direct]: dial tcp 1.2.3.4:6881: i/o timeout`,
	}
	for _, line := range lines {
		tr.Feed(line, now, servers)
	}
	if a, f := tr.Stats([]string{"vp3"}, 5*time.Minute, now); a != 2 || f != 1 {
		t.Fatalf("vp3 = %d attempts %d failures, want 2/1", a, f)
	}
	if a, f := tr.Stats([]string{"hk"}, 5*time.Minute, now); a != 2 || f != 1 {
		t.Fatalf("hk = %d attempts %d failures, want 2/1", a, f)
	}
	if a, f := tr.Stats([]string{"vp3", "hk"}, 5*time.Minute, now); a != 4 || f != 2 {
		t.Fatalf("both = %d/%d, want 4/2", a, f)
	}
	if a, _ := tr.Stats([]string{"direct"}, 5*time.Minute, now); a != 0 {
		t.Fatalf("direct counted: %d", a)
	}
}

func TestStatsWindowAndReset(t *testing.T) {
	tr := New()
	start := time.Unix(1_800_000_000, 0)
	for i := 0; i < 10; i++ {
		at := start.Add(time.Duration(i) * time.Minute)
		tr.Feed(`INFO [`+string(rune('0'+i))+` 0ms] outbound/vless[vp3]: outbound connection to x:443`, at, servers)
	}
	now := start.Add(9 * time.Minute)
	if a, _ := tr.Stats([]string{"vp3"}, 5*time.Minute, now); a != 5 {
		t.Fatalf("5m window = %d attempts, want 5", a)
	}
	tr.Reset([]string{"vp3"})
	if a, _ := tr.Stats([]string{"vp3"}, time.Hour, now); a != 0 {
		t.Fatalf("after reset = %d attempts", a)
	}
}

func TestFollowTailsAndTruncates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sing-box.log")
	os.WriteFile(path, []byte("old line\n"), 0644)
	got := make(chan string, 10)
	go Follow(path, 64, func(line string) { got <- line })
	time.Sleep(1500 * time.Millisecond) // past the initial open at the end

	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	defer f.Close()
	f.WriteString("first\nsecond " + strings.Repeat("x", 80) + "\n")
	for _, want := range []string{"first", "second " + strings.Repeat("x", 80)} {
		select {
		case line := <-got:
			if line != want {
				t.Fatalf("line = %q, want %q", line, want)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for %q", want)
		}
	}
	// Over maxSize: the file is emptied and reading continues at its start.
	deadline := time.Now().Add(3 * time.Second)
	for info, _ := os.Stat(path); info.Size() != 0; info, _ = os.Stat(path) {
		if time.Now().After(deadline) {
			t.Fatalf("log not truncated (size %d)", info.Size())
		}
		time.Sleep(100 * time.Millisecond)
	}
	f.WriteString("third\n")
	select {
	case line := <-got:
		if line != "third" {
			t.Fatalf("after truncate = %q", line)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no line after truncate")
	}
}
