// Package dialstats counts, per outbound, the connections sing-box opened
// through it and how many of them failed to reach the outbound's own server.
// It reads sing-box's info-level log: delay probes don't log a connection ID,
// so only real traffic is counted.
package dialstats

import (
	"net"
	"regexp"
	"sync"
	"time"
)

var (
	// [3552987913 1ms] outbound/vless[vp3]: outbound connection to example.com:443
	attemptLine = regexp.MustCompile(`\[(\d+) [^\]]*\] outbound/[^\[\s]+\[([^\]]+)\]: outbound (?:packet )?connection to `)
	// [3552987913 5.1s] connection: open connection to example.com:443 using outbound/vless[vp3]: dial tcp 84.32.149.31:443: i/o timeout
	failureLine = regexp.MustCompile(`\[(\d+) [^\]]*\] connection: .* using outbound/[^\[\s]+\[([^\]]+)\]: dial (?:tcp|udp) (\S+): `)
)

const (
	keepBuckets = 60              // minutes of history kept per outbound
	seenTTL     = 2 * time.Minute // how long a connection ID is remembered
)

type bucket struct {
	minute             int64
	attempts, failures int
}

type conn struct {
	tag    string
	at     time.Time
	failed bool
}

// Tracker holds per-outbound counts in one-minute buckets.
type Tracker struct {
	mu      sync.Mutex
	buckets map[string][]bucket
	seen    map[string]conn
	pruned  int64
}

func New() *Tracker {
	return &Tracker{buckets: map[string][]bucket{}, seen: map[string]conn{}}
}

// Feed reads one sing-box log line. servers maps an outbound tag to its
// "host:port". A failure counts only when the dial went to that server (its
// port, and its address when the server is an IP): a proxy that could not
// reach the destination says nothing about the server.
func (t *Tracker) Feed(line string, now time.Time, servers map[string]string) {
	if m := attemptLine.FindStringSubmatch(line); m != nil {
		id, tag := m[1], m[2]
		if _, known := servers[tag]; !known {
			return
		}
		t.mu.Lock()
		defer t.mu.Unlock()
		if _, dup := t.seen[id]; !dup { // sing-box may log a connection twice
			t.seen[id] = conn{tag: tag, at: now}
			t.add(tag, now, 1, 0)
		}
		t.prune(now)
		return
	}
	m := failureLine.FindStringSubmatch(line)
	if m == nil {
		return
	}
	id, tag, addr := m[1], m[2], m[3]
	if !dialedServer(addr, servers[tag]) {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	c, known := t.seen[id]
	if known && c.failed {
		return
	}
	attempts := 0
	if !known {
		attempts = 1 // the attempt line was missed (e.g. read started mid-connection)
	}
	t.seen[id] = conn{tag: tag, at: now, failed: true}
	t.add(tag, now, attempts, 1)
}

func dialedServer(addr string, server string) bool {
	if server == "" {
		return false
	}
	dialHost, dialPort, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	host, port, err := net.SplitHostPort(server)
	if err != nil || port != dialPort {
		return false
	}
	// A hostname server is dialed by its resolved IP, so only the port can be
	// compared.
	return net.ParseIP(host) == nil || net.ParseIP(host).Equal(net.ParseIP(dialHost))
}

func (t *Tracker) add(tag string, now time.Time, attempts, failures int) {
	minute := now.Unix() / 60
	list := t.buckets[tag]
	if n := len(list); n > 0 && list[n-1].minute == minute {
		list[n-1].attempts += attempts
		list[n-1].failures += failures
	} else {
		list = append(list, bucket{minute: minute, attempts: attempts, failures: failures})
		if len(list) > keepBuckets {
			list = list[len(list)-keepBuckets:]
		}
	}
	t.buckets[tag] = list
}

func (t *Tracker) prune(now time.Time) {
	if now.Unix()/60 == t.pruned {
		return
	}
	t.pruned = now.Unix() / 60
	for id, c := range t.seen {
		if now.Sub(c.at) > seenTTL {
			delete(t.seen, id)
		}
	}
}

// Stats sums attempts and failures for the given tags over the last window.
func (t *Tracker) Stats(tags []string, window time.Duration, now time.Time) (attempts, failures int) {
	since := now.Add(-window).Unix() / 60
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, tag := range tags {
		for _, b := range t.buckets[tag] {
			if b.minute > since {
				attempts += b.attempts
				failures += b.failures
			}
		}
	}
	return attempts, failures
}

// Reset forgets the counts of the given tags, so a member that was set aside
// is judged afresh when it is tried again.
func (t *Tracker) Reset(tags []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, tag := range tags {
		delete(t.buckets, tag)
	}
}
