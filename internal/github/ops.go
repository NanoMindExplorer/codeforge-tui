package github

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// PRSummary is a short PR view for lists.
type PRSummary struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	URL    string `json:"url"`
	Head   string `json:"headRefName"`
	Base   string `json:"baseRefName"`
	Author string `json:"author"`
}

// IssueSummary is a short issue view.
type IssueSummary struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	URL    string `json:"url"`
	Author string `json:"author"`
}

// ListPRs lists open PRs (state: open|closed|merged|all).
func (c *Client) ListPRs(ctx context.Context, state string, limit int) ([]PRSummary, error) {
	if state == "" {
		state = "open"
	}
	if limit <= 0 {
		limit = 20
	}
	if c.HasCLI() {
		out, err := c.runGH(ctx, "pr", "list",
			"--state", state,
			"--limit", strconv.Itoa(limit),
			"--json", "number,title,state,url,headRefName,baseRefName,author",
		)
		if err != nil {
			return nil, err
		}
		var raw []struct {
			Number      int    `json:"number"`
			Title       string `json:"title"`
			State       string `json:"state"`
			URL         string `json:"url"`
			HeadRefName string `json:"headRefName"`
			BaseRefName string `json:"baseRefName"`
			Author      struct {
				Login string `json:"login"`
			} `json:"author"`
		}
		if err := json.Unmarshal([]byte(out), &raw); err != nil {
			return nil, err
		}
		outp := make([]PRSummary, 0, len(raw))
		for _, r := range raw {
			outp = append(outp, PRSummary{
				Number: r.Number, Title: r.Title, State: r.State, URL: r.URL,
				Head: r.HeadRefName, Base: r.BaseRefName, Author: r.Author.Login,
			})
		}
		return outp, nil
	}
	slug, err := c.RepoSlug(ctx)
	if err != nil {
		return nil, err
	}
	apiState := state
	if state == "merged" {
		apiState = "closed"
	}
	path := fmt.Sprintf("/repos/%s/pulls?state=%s&per_page=%d", slug, apiState, limit)
	raw, err := c.REST(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var items []struct {
		Number  int    `json:"number"`
		Title   string `json:"title"`
		State   string `json:"state"`
		HTMLURL string `json:"html_url"`
		Head    struct {
			Ref string `json:"ref"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
		User struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	outp := make([]PRSummary, 0, len(items))
	for _, r := range items {
		outp = append(outp, PRSummary{
			Number: r.Number, Title: r.Title, State: r.State, URL: r.HTMLURL,
			Head: r.Head.Ref, Base: r.Base.Ref, Author: r.User.Login,
		})
	}
	return outp, nil
}

// ViewPR returns detailed PR text (title, body, checks summary).
func (c *Client) ViewPR(ctx context.Context, number int) (string, error) {
	if c.HasCLI() {
		if number > 0 {
			return c.runGH(ctx, "pr", "view", strconv.Itoa(number),
				"--json", "number,title,state,url,body,headRefName,baseRefName,author,commits,statusCheckRollup,reviewDecision",
			)
		}
		return c.runGH(ctx, "pr", "view",
			"--json", "number,title,state,url,body,headRefName,baseRefName,author,statusCheckRollup,reviewDecision",
		)
	}
	slug, err := c.RepoSlug(ctx)
	if err != nil {
		return "", err
	}
	if number <= 0 {
		return "", fmt.Errorf("PR number required without gh CLI")
	}
	raw, err := c.REST(ctx, "GET", fmt.Sprintf("/repos/%s/pulls/%d", slug, number), nil)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// CreatePR creates a pull request. base defaults to main/master if empty.
func (c *Client) CreatePR(ctx context.Context, title, body, base, head string, draft bool) (string, error) {
	if title == "" {
		return "", fmt.Errorf("title required")
	}
	if c.HasCLI() {
		args := []string{"pr", "create", "--title", title, "--body", body}
		if base != "" {
			args = append(args, "--base", base)
		}
		if head != "" {
			args = append(args, "--head", head)
		}
		if draft {
			args = append(args, "--draft")
		}
		// fill if empty body
		if body == "" {
			args = append(args, "--fill")
			// remove empty body flag conflict — rebuild
			args = []string{"pr", "create", "--title", title, "--fill"}
			if base != "" {
				args = append(args, "--base", base)
			}
			if head != "" {
				args = append(args, "--head", head)
			}
			if draft {
				args = append(args, "--draft")
			}
		}
		out, err := c.runGH(ctx, args...)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(out), nil
	}
	slug, err := c.RepoSlug(ctx)
	if err != nil {
		return "", err
	}
	if base == "" {
		base = "main"
	}
	if head == "" {
		return "", fmt.Errorf("head branch required without gh CLI")
	}
	payload := map[string]any{
		"title": title,
		"body":  body,
		"base":  base,
		"head":  head,
		"draft": draft,
	}
	raw, err := c.REST(ctx, "POST", fmt.Sprintf("/repos/%s/pulls", slug), payload)
	if err != nil {
		return "", err
	}
	var pr struct {
		HTMLURL string `json:"html_url"`
		Number  int    `json:"number"`
	}
	_ = json.Unmarshal(raw, &pr)
	return fmt.Sprintf("PR #%d %s", pr.Number, pr.HTMLURL), nil
}

// MergePR merges a pull request (merge|squash|rebase).
func (c *Client) MergePR(ctx context.Context, number int, method string) (string, error) {
	if method == "" {
		method = "squash"
	}
	if c.HasCLI() {
		args := []string{"pr", "merge", strconv.Itoa(number), "--" + method}
		out, err := c.runGH(ctx, args...)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(out), nil
	}
	slug, err := c.RepoSlug(ctx)
	if err != nil {
		return "", err
	}
	payload := map[string]any{"merge_method": method}
	raw, err := c.REST(ctx, "PUT", fmt.Sprintf("/repos/%s/pulls/%d/merge", slug, number), payload)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// ListIssues lists issues (not PRs when using gh).
func (c *Client) ListIssues(ctx context.Context, state string, limit int) ([]IssueSummary, error) {
	if state == "" {
		state = "open"
	}
	if limit <= 0 {
		limit = 20
	}
	if c.HasCLI() {
		out, err := c.runGH(ctx, "issue", "list",
			"--state", state,
			"--limit", strconv.Itoa(limit),
			"--json", "number,title,state,url,author",
		)
		if err != nil {
			return nil, err
		}
		var raw []struct {
			Number int    `json:"number"`
			Title  string `json:"title"`
			State  string `json:"state"`
			URL    string `json:"url"`
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
		}
		if err := json.Unmarshal([]byte(out), &raw); err != nil {
			return nil, err
		}
		outp := make([]IssueSummary, 0, len(raw))
		for _, r := range raw {
			outp = append(outp, IssueSummary{
				Number: r.Number, Title: r.Title, State: r.State, URL: r.URL, Author: r.Author.Login,
			})
		}
		return outp, nil
	}
	slug, err := c.RepoSlug(ctx)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/repos/%s/issues?state=%s&per_page=%d", slug, state, limit)
	raw, err := c.REST(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var items []struct {
		Number      int    `json:"number"`
		Title       string `json:"title"`
		State       string `json:"state"`
		HTMLURL     string `json:"html_url"`
		PullRequest *any   `json:"pull_request"`
		User        struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	outp := make([]IssueSummary, 0, len(items))
	for _, r := range items {
		if r.PullRequest != nil {
			continue // skip PRs in issues list
		}
		outp = append(outp, IssueSummary{
			Number: r.Number, Title: r.Title, State: r.State, URL: r.HTMLURL, Author: r.User.Login,
		})
	}
	return outp, nil
}

// CreateIssue opens a new issue.
func (c *Client) CreateIssue(ctx context.Context, title, body string, labels []string) (string, error) {
	if title == "" {
		return "", fmt.Errorf("title required")
	}
	if c.HasCLI() {
		args := []string{"issue", "create", "--title", title, "--body", body}
		for _, l := range labels {
			if l != "" {
				args = append(args, "--label", l)
			}
		}
		out, err := c.runGH(ctx, args...)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(out), nil
	}
	slug, err := c.RepoSlug(ctx)
	if err != nil {
		return "", err
	}
	payload := map[string]any{"title": title, "body": body}
	if len(labels) > 0 {
		payload["labels"] = labels
	}
	raw, err := c.REST(ctx, "POST", fmt.Sprintf("/repos/%s/issues", slug), payload)
	if err != nil {
		return "", err
	}
	var iss struct {
		HTMLURL string `json:"html_url"`
		Number  int    `json:"number"`
	}
	_ = json.Unmarshal(raw, &iss)
	return fmt.Sprintf("Issue #%d %s", iss.Number, iss.HTMLURL), nil
}

// ViewIssue returns issue JSON/detail.
func (c *Client) ViewIssue(ctx context.Context, number int) (string, error) {
	if c.HasCLI() {
		return c.runGH(ctx, "issue", "view", strconv.Itoa(number),
			"--json", "number,title,state,url,body,author,labels,comments",
		)
	}
	slug, err := c.RepoSlug(ctx)
	if err != nil {
		return "", err
	}
	raw, err := c.REST(ctx, "GET", fmt.Sprintf("/repos/%s/issues/%d", slug, number), nil)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// Checks returns CI status for a PR or current branch.
func (c *Client) Checks(ctx context.Context, prNumber int) (string, error) {
	if c.HasCLI() {
		if prNumber > 0 {
			out, err := c.runGH(ctx, "pr", "checks", strconv.Itoa(prNumber))
			if err != nil {
				// fallback json
				return c.runGH(ctx, "pr", "view", strconv.Itoa(prNumber), "--json", "statusCheckRollup")
			}
			return out, nil
		}
		out, err := c.runGH(ctx, "pr", "checks")
		if err != nil {
			return c.runGH(ctx, "pr", "view", "--json", "statusCheckRollup")
		}
		return out, nil
	}
	return "", fmt.Errorf("checks require `gh` CLI for best results")
}

// Push pushes the current branch (and optionally sets upstream).
func (c *Client) Push(ctx context.Context, setUpstream bool) (string, error) {
	if c.HasCLI() {
		args := []string{"repo", "sync"} // not ideal
		_ = args
	}
	// use git push
	args := []string{"push"}
	if setUpstream {
		args = []string{"push", "-u", "origin", "HEAD"}
	}
	cmdOut, err := runGit(ctx, c.WorkDir, args...)
	if err != nil {
		// try with -u if first push failed
		if !setUpstream {
			return runGit(ctx, c.WorkDir, "push", "-u", "origin", "HEAD")
		}
		return "", err
	}
	return cmdOut, nil
}

// RepoView returns short repo metadata.
func (c *Client) RepoView(ctx context.Context) (string, error) {
	if c.HasCLI() {
		return c.runGH(ctx, "repo", "view",
			"--json", "nameWithOwner,description,url,defaultBranchRef,isPrivate,viewerPermission",
		)
	}
	slug, err := c.RepoSlug(ctx)
	if err != nil {
		return "", err
	}
	raw, err := c.REST(ctx, "GET", "/repos/"+slug, nil)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// FormatPRList renders PR list for chat.
func FormatPRList(prs []PRSummary) string {
	if len(prs) == 0 {
		return "No pull requests found."
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%d pull request(s):\n\n", len(prs)))
	for _, p := range prs {
		b.WriteString(fmt.Sprintf("  #%d  [%s]  %s\n", p.Number, p.State, p.Title))
		b.WriteString(fmt.Sprintf("       %s → %s  @%s\n", p.Head, p.Base, p.Author))
		if p.URL != "" {
			b.WriteString(fmt.Sprintf("       %s\n", p.URL))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// FormatIssueList renders issues for chat.
func FormatIssueList(issues []IssueSummary) string {
	if len(issues) == 0 {
		return "No issues found."
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%d issue(s):\n\n", len(issues)))
	for _, it := range issues {
		b.WriteString(fmt.Sprintf("  #%d  [%s]  %s\n", it.Number, it.State, it.Title))
		b.WriteString(fmt.Sprintf("       @%s  %s\n\n", it.Author, it.URL))
	}
	return b.String()
}

func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	// local helper without importing os/exec at top of every call site messily
	return runGitCmd(ctx, dir, args...)
}
