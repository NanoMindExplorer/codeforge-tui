package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/codeforge/tui/internal/secrets"
)

// URLFetch safely fetches public HTTP(S) documentation for the agent.
type URLFetch struct {
	// optional allowlist host suffixes; empty = any public host (still blocks private IPs)
	AllowHosts []string
}

func (u *URLFetch) Name() string { return "fetch_url" }
func (u *URLFetch) Description() string {
	return `Fetch a public https URL and return text content (docs, raw files).
Blocks private/loopback IPs. Max 400KB. HTML is lightly stripped to text.
Use for official documentation — not for secrets or authenticated endpoints.`
}

func (u *URLFetch) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{"type": "string", "description": "https URL to fetch"},
			"max_bytes": map[string]any{"type": "integer", "description": "Max body size (default 200000)"},
		},
		"required": []string{"url"},
	}
}

type fetchInput struct {
	URL      string `json:"url"`
	MaxBytes int    `json:"max_bytes"`
}

func (u *URLFetch) Execute(input json.RawMessage) Result {
	var in fetchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return Result{Error: err.Error()}
	}
	rawURL := strings.TrimSpace(in.URL)
	if rawURL == "" {
		return Result{Error: "url required"}
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return Result{Error: "invalid url"}
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return Result{Error: "only http/https allowed"}
	}
	if parsed.Scheme == "http" {
		// allow but prefer https
	}
	host := parsed.Hostname()
	if host == "" {
		return Result{Error: "missing host"}
	}
	if err := assertPublicHost(host); err != nil {
		return Result{Error: err.Error()}
	}
	if len(u.AllowHosts) > 0 && !hostAllowed(host, u.AllowHosts) {
		return Result{Error: "host not in allowlist"}
	}
	max := in.MaxBytes
	if max <= 0 {
		max = 200_000
	}
	if max > 400_000 {
		max = 400_000
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", parsed.String(), nil)
	if err != nil {
		return Result{Error: err.Error()}
	}
	req.Header.Set("User-Agent", "CodeForgeTUI/0.6 (+https://github.com/NanoMindExplorer/codeforge)")
	req.Header.Set("Accept", "text/*, application/json, application/xml")

	client := &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return assertPublicHost(req.URL.Hostname())
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return Result{Error: fmt.Sprintf("fetch: %v", err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return Result{Error: fmt.Sprintf("HTTP %s", resp.Status)}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(max)+1))
	if err != nil {
		return Result{Error: err.Error()}
	}
	truncated := false
	if len(body) > max {
		body = body[:max]
		truncated = true
	}
	ct := resp.Header.Get("Content-Type")
	text := string(body)
	if strings.Contains(ct, "html") || strings.HasPrefix(strings.TrimSpace(text), "<!DOCTYPE") || strings.HasPrefix(strings.TrimSpace(text), "<html") {
		text = stripHTML(text)
	}
	text = secrets.Redact(text)
	if truncated {
		text += "\n… (truncated)"
	}
	return Result{Success: true, Output: fmt.Sprintf("URL: %s\nStatus: %s\nContent-Type: %s\n\n%s", rawURL, resp.Status, ct, text)}
}

func assertPublicHost(host string) error {
	h := strings.ToLower(host)
	if h == "localhost" || strings.HasSuffix(h, ".local") || h == "0.0.0.0" {
		return fmt.Errorf("private/local hosts blocked")
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		// DNS fail — still allow? safer to block
		return fmt.Errorf("dns: %w", err)
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return fmt.Errorf("private IP blocked for host %s", host)
		}
	}
	return nil
}

func hostAllowed(host string, allow []string) bool {
	h := strings.ToLower(host)
	for _, a := range allow {
		a = strings.ToLower(strings.TrimSpace(a))
		if a == "" {
			continue
		}
		if h == a || strings.HasSuffix(h, "."+a) {
			return true
		}
	}
	return false
}

func stripHTML(s string) string {
	// crude tag strip
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
			b.WriteByte(' ')
		case !inTag:
			b.WriteRune(r)
		}
	}
	out := b.String()
	// collapse whitespace
	out = strings.Join(strings.Fields(out), " ")
	if len(out) > 30_000 {
		out = out[:30_000] + "…"
	}
	return out
}
