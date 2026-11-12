package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"time"

	managerconfig "github.com/sannysoft/pikabox/internal/config"
	"github.com/sannysoft/pikabox/internal/health"
	"github.com/sannysoft/pikabox/internal/render"
	"github.com/sannysoft/pikabox/internal/runtime"
)

const (
	// unknownRestartAfter is how long the exit's health must stay unreadable
	// before sing-box is restarted.
	unknownRestartAfter = 90 * time.Second
	// restartGrace leaves a freshly started sing-box alone.
	restartGrace = 2 * time.Minute
	// restartSpacing is the least time between two automatic restarts.
	restartSpacing = 10 * time.Minute
)

// exitLeaf follows the live selection from the final target down to the
// outbound actually carrying traffic; chain holds the groups on the way.
func exitLeaf(proxies map[string]clashProxy, final string) (chain []string, leaf string) {
	leaf = final
	for i := 0; i < 8 && proxies[leaf].Now != ""; i++ {
		chain = append(chain, leaf)
		leaf = proxies[leaf].Now
	}
	return chain, leaf
}

// leafHealth is the status page's verdict on the exit outbound.
func leafHealth(p clashProxy) string {
	switch {
	case p.lastDelay() > 0:
		return "ok"
	case len(p.History) > 0:
		return "down"
	}
	return "unknown"
}

// watchdog holds the state of the stuck-sing-box check.
var watchdog struct {
	unknownSince time.Time
	lastRestart  time.Time
}

// checkStuck restarts sing-box when the status page would show health
// "unknown" for unknownRestartAfter: the Clash API does not answer, or the
// exit cannot even be delay-tested through it. An exit that simply has no
// delay result yet is tested first, so a healthy idle exit is never taken
// for a stuck one.
func checkStuck(configPath string, cfg managerconfig.Config, now time.Time) {
	pid := managedRuntimePID(runtime.DefaultPaths)
	api := newClashAPI(cfg)
	if pid == 0 || api == nil || cfg.Manager.RuntimeMode == "direct" {
		watchdog.unknownSince = time.Time{}
		return
	}
	if exitHealthReadable(api, render.BuildPlan(cfg).Final(cfg), firstNonEmptyString(cfg.Manager.TestURL, health.DefaultTestURL)) {
		watchdog.unknownSince = time.Time{}
		return
	}
	if watchdog.unknownSince.IsZero() {
		watchdog.unknownSince = now
	}
	if !shouldRestart(watchdog.unknownSince, processStart(pid), watchdog.lastRestart, now) {
		return
	}
	watchdog.lastRestart, watchdog.unknownSince = now, time.Time{}
	result := restartRuntime(configPath)
	if ok, _ := result["ok"].(bool); !ok {
		recordEvent("error", "sing-box health unknown for %s; restart failed: %v", shortDuration(unknownRestartAfter), result["errors"])
		return
	}
	recordEvent("reload", "sing-box restarted: health unknown for %s (controller API not answering)", shortDuration(unknownRestartAfter))
}

// exitHealthReadable reports whether the exit's health is known (ok or down)
// or can be made known with one delay test.
func exitHealthReadable(api *clashAPI, final, testURL string) bool {
	proxies, err := api.proxies()
	if err != nil {
		return false
	}
	_, leaf := exitLeaf(proxies, final)
	if leaf == "direct" || leaf == "block" || leafHealth(proxies[leaf]) != "unknown" {
		return true
	}
	// A failed test still makes the health known ("down"); only the API not
	// answering leaves it unknown.
	_, err = api.delay(leaf, testURL)
	var transport *url.Error
	return err == nil || !errors.As(err, &transport)
}

// shouldRestart: unknown long enough, sing-box past its start-up grace, and
// no automatic restart too recently.
func shouldRestart(unknownSince, started, lastRestart, now time.Time) bool {
	return !unknownSince.IsZero() && now.Sub(unknownSince) >= unknownRestartAfter &&
		now.Sub(started) >= restartGrace && now.Sub(lastRestart) >= restartSpacing
}

// processStart approximates when pid started from its /proc entry.
func processStart(pid int) time.Time {
	if info, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err == nil {
		return info.ModTime()
	}
	return time.Time{}
}
