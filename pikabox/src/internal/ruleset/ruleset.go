package ruleset

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	managerconfig "github.com/sannysoft/pikabox/internal/config"
)

const maxDownloadSize = 64 << 20

type Result struct {
	ID    string `json:"id"`
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

// Download fetches a remote rule-set and writes it to its cache path. When
// proxyAddr is non-empty (e.g. "127.0.0.1:2080", the local sing-box mixed
// inbound) the request is routed through it. Rule-set hosts are commonly
// censored on the networks this runs on, so downloading through the running
// tunnel is what makes a manual refresh succeed.
func Download(ctx context.Context, ruleset managerconfig.RuleSet, proxyAddr string) (Result, error) {
	if ruleset.URL == "" {
		return Result{}, fmt.Errorf("ruleset %q url is required", ruleset.ID)
	}
	parsed, err := url.Parse(ruleset.URL)
	if err != nil {
		return Result{}, err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return Result{}, fmt.Errorf("ruleset URL must use http or https")
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ruleset.URL, nil)
	if err != nil {
		return Result{}, err
	}
	resp, err := newClient(proxyAddr).Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("ruleset fetch failed: HTTP %d", resp.StatusCode)
	}

	path := Path(ruleset)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return Result{}, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".")
	if err != nil {
		return Result{}, err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	written, err := io.Copy(tmp, io.LimitReader(resp.Body, maxDownloadSize+1))
	if err != nil {
		_ = tmp.Close()
		return Result{}, err
	}
	if written > maxDownloadSize {
		_ = tmp.Close()
		return Result{}, fmt.Errorf("ruleset response exceeds %d bytes", maxDownloadSize)
	}
	if err := tmp.Chmod(0644); err != nil {
		_ = tmp.Close()
		return Result{}, err
	}
	if err := tmp.Close(); err != nil {
		return Result{}, err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return Result{}, err
	}
	return Result{ID: ruleset.ID, Path: path, Bytes: written}, nil
}

// newClient returns an HTTP client that routes through proxyAddr (host:port)
// when set, otherwise a direct client. The local mixed inbound speaks HTTP
// CONNECT, so an http_proxy URL is sufficient for both http and https targets.
func newClient(proxyAddr string) *http.Client {
	if strings.TrimSpace(proxyAddr) == "" {
		return http.DefaultClient
	}
	proxyURL := &url.URL{Scheme: "http", Host: proxyAddr}
	return &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
	}
}

func Path(ruleset managerconfig.RuleSet) string {
	return ruleset.Path
}

// Due reports whether the cached copy is missing or older than a week.
func Due(ruleset managerconfig.RuleSet, now time.Time) bool {
	info, err := os.Stat(ruleset.Path)
	return err != nil || now.Sub(info.ModTime()) >= 168*time.Hour
}
