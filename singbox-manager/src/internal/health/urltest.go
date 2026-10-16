package health

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/openwrt-singbox/singbox-manager/internal/render"
)

// defaultProbeConcurrency bounds how many delay tests run against the probe
// instance at once. The Clash API handles parallelism fine; the cap just keeps
// a large node list from opening hundreds of simultaneous dials on a router.
const defaultProbeConcurrency = 32

// ProbeOptions configures a real, through-the-proxy node test.
type ProbeOptions struct {
	// Binary is the sing-box executable used to stand up the throwaway probe
	// instance. Required; without it no genuine test is possible.
	Binary string
	// TestURL is fetched through each outbound. Defaults to DefaultTestURL.
	TestURL string
	// Timeout bounds a single node's delay test. Defaults to 5s.
	Timeout time.Duration
	// Concurrency bounds simultaneous delay tests. Defaults to 16.
	Concurrency int
}

// ProbeNodes delay-tests every node through its real outbound and returns a
// result per node ID. It stands up one throwaway sing-box instance holding all
// nodes as outbounds plus the Clash API, then hits /proxies/{tag}/delay for each
// node concurrently — the same end-to-end measurement v2rayNG's "real delay"
// performs (handshake + transport + auth + an actual HTTP GET through the
// tunnel), not a bare TCP connect to a CDN front.
//
// Unlike the old TCP probe it never reports a dead node as healthy: a node is
// "ok" only when a request genuinely completed through it.
// ProbeOutbounds delay-tests every outbound (keyed by tag) through its real
// path and returns a result per tag. It stands up one throwaway sing-box
// instance holding all outbounds plus the Clash API, then hits
// /proxies/{tag}/delay concurrently — handshake + transport + auth + an actual
// HTTP GET through the tunnel, never a bare TCP connect. An outbound is "ok"
// only when a request genuinely completed through it.
func ProbeOutbounds(ctx context.Context, opts ProbeOptions, outbounds map[string]map[string]any) map[string]EndpointResult {
	results := map[string]EndpointResult{}
	if len(outbounds) == 0 {
		return results
	}
	if opts.TestURL == "" {
		opts.TestURL = DefaultTestURL
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Second
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = defaultProbeConcurrency
	}
	if opts.Binary == "" {
		return markAll(results, outbounds, "error", "sing-box binary is not configured")
	}

	port, err := freeLocalPort()
	if err != nil {
		return markAll(results, outbounds, "error", fmt.Sprintf("allocate probe port: %v", err))
	}
	secret, err := randomSecret()
	if err != nil {
		return markAll(results, outbounds, "error", fmt.Sprintf("generate probe secret: %v", err))
	}
	api := render.ProbeAPI{Listen: fmt.Sprintf("127.0.0.1:%d", port), Secret: secret}

	dir, err := os.MkdirTemp("", "singbox-probe-")
	if err != nil {
		return markAll(results, outbounds, "error", fmt.Sprintf("create probe dir: %v", err))
	}
	defer os.RemoveAll(dir)

	cfgPath := filepath.Join(dir, "config.json")
	cfgData, err := render.BuildProbeConfig(outbounds, api)
	if err == nil {
		err = os.WriteFile(cfgPath, cfgData, 0600)
	}
	if err != nil {
		return markAll(results, outbounds, "error", fmt.Sprintf("build probe config: %v", err))
	}
	// One invalid outbound makes sing-box refuse the whole config. Only when the
	// combined check fails, check each outbound alone and drop the bad ones, so
	// a malformed entry is reported individually instead of poisoning the batch.
	if _, err := singBoxCheck(ctx, opts.Binary, cfgPath); err != nil {
		outbounds = precheck(ctx, opts, outbounds, dir, results)
		if len(outbounds) == 0 {
			return results
		}
		if cfgData, err = render.BuildProbeConfig(outbounds, api); err == nil {
			err = os.WriteFile(cfgPath, cfgData, 0600)
		}
		if err != nil {
			return markAll(results, outbounds, "error", fmt.Sprintf("build probe config: %v", err))
		}
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(runCtx, opts.Binary, "run", "-c", cfgPath)
	// Kill the probe if this daemon dies: a SIGTERM (procd stop, or a crash) runs
	// os.Exit from the signal handler and skips the defers below, which would
	// otherwise orphan the probe sing-box. Pdeathsig makes the kernel reap it.
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return markAll(results, outbounds, "error", fmt.Sprintf("start probe sing-box: %v", err))
	}
	defer func() {
		cancel()
		_ = cmd.Wait()
	}()

	client := &http.Client{}
	if err := waitClashReady(runCtx, client, api, 10*time.Second); err != nil {
		msg := fmt.Sprintf("probe instance did not come up: %v", err)
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			msg = fmt.Sprintf("%s: %s", msg, lastLine(detail))
		}
		return markAll(results, outbounds, "error", msg)
	}

	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		sem = make(chan struct{}, opts.Concurrency)
	)
	for tag := range outbounds {
		wg.Add(1)
		go func(tag string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			ms, derr := clashDelay(runCtx, client, api, tag, opts.TestURL, opts.Timeout)
			mu.Lock()
			if derr != nil {
				results[tag] = EndpointResult{ID: tag, Health: "down", Error: derr.Error(), Method: "url"}
			} else {
				results[tag] = EndpointResult{ID: tag, Health: "ok", LatencyMS: ms, Method: "url"}
			}
			mu.Unlock()
		}(tag)
	}
	wg.Wait()
	return results
}

// precheck validates each outbound alone with `sing-box check`, concurrently,
// records failures in results, and returns the outbounds that pass.
func precheck(ctx context.Context, opts ProbeOptions, outbounds map[string]map[string]any, dir string, results map[string]EndpointResult) map[string]map[string]any {
	var (
		mu    sync.Mutex
		wg    sync.WaitGroup
		sem   = make(chan struct{}, opts.Concurrency)
		valid = map[string]map[string]any{}
		i     int
	)
	for tag, outbound := range outbounds {
		i++
		wg.Add(1)
		go func(i int, tag string, outbound map[string]any) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			reason := ""
			data, err := render.BuildProbeConfig(map[string]map[string]any{tag: outbound}, render.ProbeAPI{Listen: "127.0.0.1:0"})
			path := filepath.Join(dir, fmt.Sprintf("check-%d.json", i))
			if err == nil {
				err = os.WriteFile(path, data, 0600)
			}
			if err != nil {
				reason = err.Error()
			} else if out, err := singBoxCheck(ctx, opts.Binary, path); err != nil {
				if reason = lastLine(out); reason == "" {
					reason = err.Error()
				}
			}
			mu.Lock()
			defer mu.Unlock()
			if reason != "" {
				results[tag] = EndpointResult{ID: tag, Health: "error", Error: reason, Method: "url"}
			} else {
				valid[tag] = outbound
			}
		}(i, tag, outbound)
	}
	wg.Wait()
	return valid
}

func singBoxCheck(ctx context.Context, binary, configPath string) (string, error) {
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(checkCtx, binary, "check", "-c", configPath).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// clashDelay performs one Clash API delay test and returns the measured latency
// in milliseconds. A non-2xx status, timeout, or zero delay is a failure.
func clashDelay(ctx context.Context, client *http.Client, api render.ProbeAPI, name, testURL string, timeout time.Duration) (int, error) {
	reqCtx, cancel := context.WithTimeout(ctx, timeout+3*time.Second)
	defer cancel()

	endpoint := fmt.Sprintf("http://%s/proxies/%s/delay?timeout=%d&url=%s",
		api.Listen, url.PathEscape(name), timeout.Milliseconds(), url.QueryEscape(testURL))
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	if api.Secret != "" {
		req.Header.Set("Authorization", "Bearer "+api.Secret)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return parseDelayResponse(resp.StatusCode, body)
}

// parseDelayResponse interprets a Clash API /delay reply. Kept pure for testing.
func parseDelayResponse(status int, body []byte) (int, error) {
	if status == http.StatusOK {
		var ok struct {
			Delay int `json:"delay"`
		}
		if err := json.Unmarshal(body, &ok); err != nil {
			return 0, fmt.Errorf("invalid delay response: %w", err)
		}
		if ok.Delay <= 0 {
			return 0, fmt.Errorf("proxy returned zero delay")
		}
		return ok.Delay, nil
	}
	var failure struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &failure) == nil && failure.Message != "" {
		return 0, fmt.Errorf("%s", failure.Message)
	}
	return 0, fmt.Errorf("delay test failed with status %d", status)
}

func waitClashReady(ctx context.Context, client *http.Client, api render.ProbeAPI, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	endpoint := fmt.Sprintf("http://%s/version", api.Listen)
	for {
		reqCtx, cancel := context.WithTimeout(ctx, time.Second)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
		if err != nil {
			cancel()
			return err
		}
		if api.Secret != "" {
			req.Header.Set("Authorization", "Bearer "+api.Secret)
		}
		resp, err := client.Do(req)
		cancel()
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().After(deadline) {
			if err != nil {
				return err
			}
			return fmt.Errorf("clash api not ready after %s", timeout)
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func freeLocalPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func randomSecret() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func lastLine(text string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
func markAll(results map[string]EndpointResult, outbounds map[string]map[string]any, healthState, errMsg string) map[string]EndpointResult {
	for tag := range outbounds {
		results[tag] = EndpointResult{ID: tag, Health: healthState, Error: errMsg, Method: "url"}
	}
	return results
}
