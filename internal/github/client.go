// Package github integrates CodeForge with GitHub the way modern AI coding
// agents do: prefer the official `gh` CLI when available, fall back to the
// REST API with GITHUB_TOKEN / GH_TOKEN.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Client talks to GitHub via gh CLI and/or REST.
type Client struct {
	WorkDir    string
	Token      string
	Host       string // api host, default api.github.com
	HTTPClient *http.Client
	// PreferCLI forces gh when true (default true if gh is on PATH).
	PreferCLI bool
	hasCLI    *bool
}

// New creates a client for workdir. Token is resolved from env if empty.
func New(workdir string) *Client {
	tok := os.Getenv("GITHUB_TOKEN")
	if tok == "" {
		tok = os.Getenv("GH_TOKEN")
	}
	return &Client{
		WorkDir:    workdir,
		Token:      tok,
		Host:       "https://api.github.com",
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
		PreferCLI:  true,
	}
}

// Available reports whether any GitHub auth path works.
func (c *Client) Available() bool {
	if c.HasCLI() {
		if _, err := c.AuthStatus(context.Background()); err == nil {
			return true
		}
	}
	return c.Token != ""
}

// HasCLI reports if `gh` is installed.
func (c *Client) HasCLI() bool {
	if c.hasCLI != nil {
		return *c.hasCLI
	}
	_, err := exec.LookPath("gh")
	ok := err == nil
	c.hasCLI = &ok
	return ok
}

// AuthStatus returns a short auth summary.
func (c *Client) AuthStatus(ctx context.Context) (string, error) {
	if c.HasCLI() {
		out, err := c.runGH(ctx, "auth", "status")
		if err == nil {
			return strings.TrimSpace(out), nil
		}
		// fall through to token check
	}
	if c.Token == "" {
		return "", fmt.Errorf("not authenticated: install gh and run `gh auth login`, or set GITHUB_TOKEN / GH_TOKEN")
	}
	u, err := c.REST(ctx, "GET", "/user", nil)
	if err != nil {
		return "", err
	}
	var user struct {
		Login string `json:"login"`
		Name  string `json:"name"`
	}
	_ = json.Unmarshal(u, &user)
	return fmt.Sprintf("REST token auth as %s (%s)", user.Login, user.Name), nil
}

// WhoAmI returns the logged-in username.
func (c *Client) WhoAmI(ctx context.Context) (string, error) {
	if c.HasCLI() {
		out, err := c.runGH(ctx, "api", "user", "--jq", ".login")
		if err == nil && strings.TrimSpace(out) != "" {
			return strings.TrimSpace(out), nil
		}
	}
	if c.Token == "" {
		return "", fmt.Errorf("no GitHub auth")
	}
	raw, err := c.REST(ctx, "GET", "/user", nil)
	if err != nil {
		return "", err
	}
	var user struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(raw, &user); err != nil {
		return "", err
	}
	return user.Login, nil
}

// RepoSlug returns owner/name for the current git remote (origin).
func (c *Client) RepoSlug(ctx context.Context) (string, error) {
	if c.HasCLI() {
		out, err := c.runGH(ctx, "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner")
		if err == nil && strings.TrimSpace(out) != "" {
			return strings.TrimSpace(out), nil
		}
	}
	// parse git remote
	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", "origin")
	cmd.Dir = c.WorkDir
	b, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("no origin remote: %w", err)
	}
	return parseRemoteSlug(string(b))
}

func parseRemoteSlug(url string) (string, error) {
	url = strings.TrimSpace(url)
	url = strings.TrimSuffix(url, ".git")
	// git@github.com:owner/repo
	if strings.HasPrefix(url, "git@") {
		parts := strings.SplitN(url, ":", 2)
		if len(parts) == 2 {
			return parts[1], nil
		}
	}
	// https://github.com/owner/repo
	for _, p := range []string{"https://github.com/", "http://github.com/", "https://www.github.com/"} {
		if strings.HasPrefix(url, p) {
			return strings.TrimPrefix(url, p), nil
		}
	}
	// ssh://git@github.com/owner/repo
	if i := strings.Index(url, "github.com/"); i >= 0 {
		return url[i+len("github.com/"):], nil
	}
	return "", fmt.Errorf("cannot parse remote slug from %q", url)
}

// runGH executes `gh` with args in WorkDir.
func (c *Client) runGH(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = c.WorkDir
	// Prefer explicit token if set so gh inherits it
	env := os.Environ()
	if c.Token != "" {
		env = append(env, "GH_TOKEN="+c.Token, "GITHUB_TOKEN="+c.Token)
	}
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("gh %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}

// REST performs an authenticated GitHub API call. path starts with /.
func (c *Client) REST(ctx context.Context, method, path string, body any) (json.RawMessage, error) {
	if c.Token == "" {
		// try gh api as transport
		if c.HasCLI() {
			args := []string{"api", "-X", method, path}
			if body != nil {
				raw, _ := json.Marshal(body)
				args = append(args, "--input", "-")
				cmd := exec.CommandContext(ctx, "gh", args...)
				cmd.Dir = c.WorkDir
				cmd.Stdin = bytes.NewReader(raw)
				var stdout, stderr bytes.Buffer
				cmd.Stdout = &stdout
				cmd.Stderr = &stderr
				if err := cmd.Run(); err != nil {
					return nil, fmt.Errorf("gh api: %s", strings.TrimSpace(stderr.String()))
				}
				return json.RawMessage(stdout.Bytes()), nil
			}
			out, err := c.runGH(ctx, args...)
			if err != nil {
				return nil, err
			}
			return json.RawMessage(out), nil
		}
		return nil, fmt.Errorf("no GITHUB_TOKEN and gh unavailable")
	}
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Host+path, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("github API %s %s: %s — %s", method, path, resp.Status, truncate(string(data), 300))
	}
	return json.RawMessage(data), nil
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
