package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	gh "github.com/codeforge/tui/internal/github"
)

// GitHubTool exposes GitHub operations (PR, issues, push, checks) to the agent.
// Mirrors the capability set used by modern AI coding companions (gh + API).
type GitHubTool struct {
	Client *gh.Client
}

func (g *GitHubTool) Name() string { return "github" }
func (g *GitHubTool) Description() string {
	return `Interact with GitHub for the current repository (like gh CLI).
Actions:
  auth_status — who is logged in
  repo_view — repository metadata
  pr_list — list pull requests (state: open|closed|merged|all)
  pr_view — view PR details (number optional = current branch PR)
  pr_create — create PR (title required; body, base, head, draft optional)
  pr_merge — merge PR (number, method: merge|squash|rebase)
  issue_list — list issues
  issue_view — view issue by number
  issue_create — create issue (title, body, labels)
  checks — CI checks for a PR or current branch PR
  push — git push current branch (-u origin HEAD)
  pull — git pull
  branch_create — create and checkout branch (name)
  log — recent commits`
}

func (g *GitHubTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "Operation to perform (see tool description)",
				"enum": []string{
					"auth_status", "repo_view",
					"pr_list", "pr_view", "pr_create", "pr_merge",
					"issue_list", "issue_view", "issue_create",
					"checks", "push", "pull", "branch_create", "log",
				},
			},
			"title":  map[string]any{"type": "string", "description": "PR or issue title"},
			"body":   map[string]any{"type": "string", "description": "PR or issue body (markdown)"},
			"base":   map[string]any{"type": "string", "description": "Base branch for PR (default main)"},
			"head":   map[string]any{"type": "string", "description": "Head branch for PR"},
			"draft":  map[string]any{"type": "boolean", "description": "Create draft PR"},
			"number": map[string]any{"type": "integer", "description": "PR or issue number"},
			"state":  map[string]any{"type": "string", "description": "Filter state: open|closed|merged|all"},
			"method": map[string]any{"type": "string", "description": "Merge method: merge|squash|rebase"},
			"name":   map[string]any{"type": "string", "description": "Branch name for branch_create"},
			"labels": map[string]any{
				"type":        "string",
				"description": "Comma-separated labels for issue_create",
			},
			"limit": map[string]any{"type": "integer", "description": "Max list items (default 20)"},
		},
		"required": []string{"action"},
	}
}

type githubInput struct {
	Action string `json:"action"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	Base   string `json:"base"`
	Head   string `json:"head"`
	Draft  bool   `json:"draft"`
	Number int    `json:"number"`
	State  string `json:"state"`
	Method string `json:"method"`
	Name   string `json:"name"`
	Labels string `json:"labels"`
	Limit  int    `json:"limit"`
}

func (g *GitHubTool) Execute(input json.RawMessage) Result {
	if g.Client == nil {
		return Result{Error: "GitHub client not configured"}
	}
	var in githubInput
	if err := json.Unmarshal(input, &in); err != nil {
		return Result{Error: fmt.Sprintf("invalid: %v", err)}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	action := strings.ToLower(strings.TrimSpace(in.Action))
	var (
		out string
		err error
	)
	switch action {
	case "auth_status":
		out, err = g.Client.AuthStatus(ctx)
	case "repo_view":
		out, err = g.Client.RepoView(ctx)
	case "pr_list":
		prs, e := g.Client.ListPRs(ctx, in.State, in.Limit)
		err = e
		if err == nil {
			out = gh.FormatPRList(prs)
		}
	case "pr_view":
		out, err = g.Client.ViewPR(ctx, in.Number)
	case "pr_create":
		out, err = g.Client.CreatePR(ctx, in.Title, in.Body, in.Base, in.Head, in.Draft)
	case "pr_merge":
		if in.Number <= 0 {
			return Result{Error: "number required for pr_merge"}
		}
		out, err = g.Client.MergePR(ctx, in.Number, in.Method)
	case "issue_list":
		issues, e := g.Client.ListIssues(ctx, in.State, in.Limit)
		err = e
		if err == nil {
			out = gh.FormatIssueList(issues)
		}
	case "issue_view":
		if in.Number <= 0 {
			return Result{Error: "number required for issue_view"}
		}
		out, err = g.Client.ViewIssue(ctx, in.Number)
	case "issue_create":
		var labels []string
		if in.Labels != "" {
			for _, l := range strings.Split(in.Labels, ",") {
				l = strings.TrimSpace(l)
				if l != "" {
					labels = append(labels, l)
				}
			}
		}
		out, err = g.Client.CreateIssue(ctx, in.Title, in.Body, labels)
	case "checks":
		out, err = g.Client.Checks(ctx, in.Number)
	case "push":
		out, err = g.Client.Push(ctx, true)
	case "pull":
		out, err = g.Client.Pull(ctx)
	case "branch_create":
		out, err = g.Client.CreateBranch(ctx, in.Name)
	case "log":
		n := in.Limit
		if n == 0 {
			n = 15
		}
		out, err = g.Client.LogRecent(ctx, n)
	default:
		return Result{Error: fmt.Sprintf("unknown action %q — use auth_status|repo_view|pr_list|pr_view|pr_create|pr_merge|issue_list|issue_view|issue_create|checks|push|pull|branch_create|log", in.Action)}
	}
	if err != nil {
		return Result{Success: false, Error: err.Error(), Output: out}
	}
	return Result{Success: true, Output: out}
}

// ParsePRNumber is a helper for slash commands.
func ParsePRNumber(s string) (int, error) {
	s = strings.TrimPrefix(s, "#")
	return strconv.Atoi(s)
}
