package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	gh "github.com/codeforge/tui/internal/github"
	"github.com/codeforge/tui/internal/ui/components"
)

func (m *Model) handleGitHubCommand(args []string) tea.Cmd {
	if m.ghClient == nil {
		m.chat.AddSystemMessage("GitHub client not available")
		return nil
	}
	if len(args) == 0 {
		m.chat.AddSystemMessage(githubHelpText())
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	// note: cancel after sync work; long ops still bounded
	defer cancel()

	head := strings.ToLower(args[0])
	rest := args[1:]

	// Allow /gh pr … and also when called as /pr via prepend
	switch head {
	case "help", "h", "?":
		m.chat.AddSystemMessage(githubHelpText())
		return nil

	case "auth", "status":
		out, err := m.ghClient.AuthStatus(ctx)
		if err != nil {
			m.chat.AddSystemMessage("⚠ GitHub auth: " + err.Error() +
				"\n\nSetup:\n  gh auth login\n  — or —\n  export GITHUB_TOKEN=ghp_...")
			return nil
		}
		user, _ := m.ghClient.WhoAmI(ctx)
		slug, _ := m.ghClient.RepoSlug(ctx)
		m.status.GitHubUser = user
		m.status.GitHubRepo = slug
		m.status.GitHubOK = true
		m.chat.AddSystemMessage("GitHub auth:\n" + out +
			fmt.Sprintf("\n\nUser: %s\nRepo: %s", user, slug))
		return nil

	case "repo":
		out, err := m.ghClient.RepoView(ctx)
		if err != nil {
			m.chat.AddSystemMessage("⚠ " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage("Repository:\n" + out)
		return nil

	case "push":
		out, err := m.ghClient.Push(ctx, true)
		if err != nil {
			m.chat.AddSystemMessage("⚠ push: " + err.Error())
			m.toast = components.NewToast("Push failed", "error", 3*time.Second)
			return nil
		}
		m.chat.AddSystemMessage("✓ Pushed\n" + out)
		m.toast = components.NewToast("Pushed to origin", "success", 3*time.Second)
		return nil

	case "pull":
		out, err := m.ghClient.Pull(ctx)
		if err != nil {
			m.chat.AddSystemMessage("⚠ pull: " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage("✓ Pulled\n" + out)
		m.toast = components.NewToast("Pulled", "success", 2*time.Second)
		return nil

	case "log":
		out, err := m.ghClient.LogRecent(ctx, 15)
		if err != nil {
			m.chat.AddSystemMessage("⚠ " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage("Recent commits:\n" + out)
		return nil

	case "branch":
		if len(rest) == 0 {
			br, err := m.ghClient.CurrentBranch(ctx)
			if err != nil {
				m.chat.AddSystemMessage("⚠ " + err.Error())
				return nil
			}
			m.chat.AddSystemMessage("Current branch: " + br)
			return nil
		}
		name := rest[0]
		out, err := m.ghClient.CreateBranch(ctx, name)
		if err != nil {
			m.chat.AddSystemMessage("⚠ branch: " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage("✓ Branch: " + name + "\n" + out)
		m.toast = components.NewToast("Branch "+name, "success", 2*time.Second)
		return nil

	case "pr":
		return m.handlePRSubcommand(ctx, rest)

	case "issue":
		return m.handleIssueSubcommand(ctx, rest)

	case "checks":
		return m.handlePRSubcommand(ctx, append([]string{"checks"}, rest...))

	default:
		// Treat unknown as agent task about github
		return m.chat.SubmitAgent("GitHub task: " + strings.Join(args, " ") +
			" — use the github tool with the appropriate action.")
	}
}

func (m *Model) handlePRSubcommand(ctx context.Context, args []string) tea.Cmd {
	if len(args) == 0 {
		args = []string{"list"}
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	switch sub {
	case "list", "ls":
		state := "open"
		if len(rest) > 0 {
			state = rest[0]
		}
		prs, err := m.ghClient.ListPRs(ctx, state, 20)
		if err != nil {
			m.chat.AddSystemMessage("⚠ " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage(gh.FormatPRList(prs))
		return nil
	case "view", "show":
		n := 0
		if len(rest) > 0 {
			n, _ = strconv.Atoi(strings.TrimPrefix(rest[0], "#"))
		}
		out, err := m.ghClient.ViewPR(ctx, n)
		if err != nil {
			m.chat.AddSystemMessage("⚠ " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage("Pull request:\n" + out)
		return nil
	case "create", "new":
		if len(rest) == 0 {
			m.chat.AddSystemMessage("Usage:\n  /pr create <title>\n  /pr create <title> | <body markdown>\n  /gh pr create \"title\" (agent can also open PRs via github tool)")
			return nil
		}
		joined := strings.Join(rest, " ")
		title, body := joined, ""
		if i := strings.Index(joined, " | "); i >= 0 {
			title = strings.TrimSpace(joined[:i])
			body = strings.TrimSpace(joined[i+3:])
		}
		out, err := m.ghClient.CreatePR(ctx, title, body, "", "", false)
		if err != nil {
			m.chat.AddSystemMessage("⚠ pr create: " + err.Error())
			m.toast = components.NewToast("PR create failed", "error", 3*time.Second)
			return nil
		}
		m.chat.AddSystemMessage("✓ Pull request created\n" + out)
		m.toast = components.NewToast("PR created", "success", 3*time.Second)
		return nil
	case "merge":
		if len(rest) == 0 {
			m.chat.AddSystemMessage("Usage: /pr merge <number> [squash|merge|rebase]")
			return nil
		}
		n, err := strconv.Atoi(strings.TrimPrefix(rest[0], "#"))
		if err != nil {
			m.chat.AddSystemMessage("⚠ invalid PR number")
			return nil
		}
		method := "squash"
		if len(rest) > 1 {
			method = rest[1]
		}
		out, err := m.ghClient.MergePR(ctx, n, method)
		if err != nil {
			m.chat.AddSystemMessage("⚠ merge: " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage("✓ Merged PR #" + strconv.Itoa(n) + "\n" + out)
		m.toast = components.NewToast("Merged #"+strconv.Itoa(n), "success", 3*time.Second)
		return nil
	case "checks", "ci":
		n := 0
		if len(rest) > 0 {
			n, _ = strconv.Atoi(strings.TrimPrefix(rest[0], "#"))
		}
		out, err := m.ghClient.Checks(ctx, n)
		if err != nil {
			m.chat.AddSystemMessage("⚠ checks: " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage("CI checks:\n" + out)
		return nil
	case "diff":
		n := 0
		if len(rest) > 0 {
			n, _ = strconv.Atoi(strings.TrimPrefix(rest[0], "#"))
		}
		out, err := m.ghClient.PRDiff(ctx, n)
		if err != nil {
			m.chat.AddSystemMessage("⚠ pr diff: " + err.Error())
			return nil
		}
		if len(out) > 8000 {
			out = out[:8000] + "\n… (truncated)"
		}
		m.chat.AddSystemMessage("PR diff:\n" + out)
		return nil
	case "comment":
		if len(rest) < 2 {
			m.chat.AddSystemMessage("Usage: /pr comment <number> <body…>")
			return nil
		}
		n, _ := strconv.Atoi(strings.TrimPrefix(rest[0], "#"))
		body := strings.Join(rest[1:], " ")
		out, err := m.ghClient.CommentOnPR(ctx, n, body)
		if err != nil {
			m.chat.AddSystemMessage("⚠ " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage("✓ Comment posted\n" + out)
		m.toast = components.NewToast("PR comment posted", "success", 2*time.Second)
		return nil
	case "review", "reviewers":
		if len(rest) < 2 {
			m.chat.AddSystemMessage("Usage: /pr review <number> user1,user2")
			return nil
		}
		n, _ := strconv.Atoi(strings.TrimPrefix(rest[0], "#"))
		var revs []string
		for _, r := range strings.Split(strings.Join(rest[1:], " "), ",") {
			r = strings.TrimSpace(r)
			if r != "" {
				revs = append(revs, r)
			}
		}
		out, err := m.ghClient.RequestReviewers(ctx, n, revs)
		if err != nil {
			m.chat.AddSystemMessage("⚠ " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage("✓ Reviewers requested\n" + out)
		return nil
	case "commits":
		n := 0
		if len(rest) > 0 {
			n, _ = strconv.Atoi(strings.TrimPrefix(rest[0], "#"))
		}
		out, err := m.ghClient.PRCommits(ctx, n)
		if err != nil {
			m.chat.AddSystemMessage("⚠ " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage("PR commits:\n" + out)
		return nil
	case "babysit":
		// /pr babysit [n] [--fix]
		n := 0
		fix := false
		for _, a := range rest {
			if a == "--fix" || a == "fix" {
				fix = true
				continue
			}
			if v, err := strconv.Atoi(strings.TrimPrefix(a, "#")); err == nil {
				n = v
			}
		}
		m.chat.AddSystemMessage(fmt.Sprintf("⏳ Babysitting PR checks (pr=%d)… poll every 20s", n))
		m.toast = components.NewToast("PR babysit started", "info", 2*time.Second)
		ghc := m.ghClient
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()
			cs, err := ghc.Babysit(ctx, gh.BabysitOptions{
				PRNumber: n,
				Interval: 20 * time.Second,
				Timeout:  15 * time.Minute,
			})
			return BabysitDoneMsg{Status: cs, Err: err, PR: n, Fix: fix}
		}
	default:
		m.chat.AddSystemMessage("Unknown /pr subcommand. " + githubHelpText())
		return nil
	}
}

func (m *Model) handleIssueSubcommand(ctx context.Context, args []string) tea.Cmd {
	if len(args) == 0 {
		args = []string{"list"}
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	switch sub {
	case "list", "ls":
		state := "open"
		if len(rest) > 0 {
			state = rest[0]
		}
		issues, err := m.ghClient.ListIssues(ctx, state, 20)
		if err != nil {
			m.chat.AddSystemMessage("⚠ " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage(gh.FormatIssueList(issues))
		return nil
	case "view", "show":
		if len(rest) == 0 {
			m.chat.AddSystemMessage("Usage: /issue view <number>")
			return nil
		}
		n, _ := strconv.Atoi(strings.TrimPrefix(rest[0], "#"))
		out, err := m.ghClient.ViewIssue(ctx, n)
		if err != nil {
			m.chat.AddSystemMessage("⚠ " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage("Issue:\n" + out)
		return nil
	case "create", "new":
		if len(rest) == 0 {
			m.chat.AddSystemMessage("Usage: /issue create <title> [| body]")
			return nil
		}
		joined := strings.Join(rest, " ")
		title, body := joined, ""
		if i := strings.Index(joined, " | "); i >= 0 {
			title = strings.TrimSpace(joined[:i])
			body = strings.TrimSpace(joined[i+3:])
		}
		out, err := m.ghClient.CreateIssue(ctx, title, body, nil)
		if err != nil {
			m.chat.AddSystemMessage("⚠ " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage("✓ Issue created\n" + out)
		m.toast = components.NewToast("Issue created", "success", 3*time.Second)
		return nil
	case "comment":
		if len(rest) < 2 {
			m.chat.AddSystemMessage("Usage: /issue comment <number> <body…>")
			return nil
		}
		n, _ := strconv.Atoi(strings.TrimPrefix(rest[0], "#"))
		body := strings.Join(rest[1:], " ")
		out, err := m.ghClient.CommentOnIssue(ctx, n, body)
		if err != nil {
			m.chat.AddSystemMessage("⚠ " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage("✓ Issue comment\n" + out)
		return nil
	default:
		m.chat.AddSystemMessage("Unknown /issue subcommand. " + githubHelpText())
		return nil
	}
}

func githubHelpText() string {
	return `GitHub integration (gh CLI or GITHUB_TOKEN)

AUTH & REPO
  /gh auth              Auth status + user + repo slug
  /gh repo              Repository metadata
  /gh log               Recent commits
  /gh branch [name]     Show or create branch

SYNC
  /push                 git push -u origin HEAD
  /pull                 git pull
  /commit [message]     Stage all + commit

PULL REQUESTS
  /pr list [state]      List PRs (open|closed|merged|all)
  /pr view [number]     View PR (current branch if omitted)
  /pr create <title> [| body]
  /pr merge <n> [squash|merge|rebase]
  /pr checks [n]        CI status
  /pr diff [n]          Full PR diff
  /pr comment <n> body  Comment on PR
  /pr review <n> u1,u2  Request reviewers
  /pr commits [n]       Commits on PR
  /pr babysit [n] [--fix]  Poll CI until green; --fix runs agent on failure

ISSUES
  /issue list [state]
  /issue view <n>
  /issue create <title> [| body]
  /issue comment <n> body

AGENT TOOLS
  search_replace · apply_patch · github (babysit, pr_diff, …)

Setup:  gh auth login   OR   export GITHUB_TOKEN=ghp_...`
}

func parseGitStatus(status string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(status, "\n") {
		line = strings.TrimSpace(line)
		if len(line) < 4 {
			continue
		}
		// format from git.Status: "  XY  path"
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		code := fields[0]
		path := fields[len(fields)-1]
		out[path] = code
	}
	return out
}

// ────────────────────────────────────────────────────────────
// Messages & pumps
// ────────────────────────────────────────────────────────────
