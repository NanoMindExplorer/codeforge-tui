package tool

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// WorktreeSession is an isolated git worktree for a subagent.
type WorktreeSession struct {
	Path   string // absolute worktree path
	Branch string
	Root   string // original repo root
}

// CreateWorktree adds a detached worktree under .codeforge/worktrees/<id>.
// Returns nil session (with no error) when not a git repo — caller should fall back.
func CreateWorktree(repoRoot, label string) (*WorktreeSession, error) {
	repoRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, err
	}
	// verify git
	if _, err := os.Stat(filepath.Join(repoRoot, ".git")); err != nil {
		// maybe worktree itself
		cmd := exec.Command("git", "-C", repoRoot, "rev-parse", "--is-inside-work-tree")
		if out, err := cmd.Output(); err != nil || strings.TrimSpace(string(out)) != "true" {
			return nil, fmt.Errorf("not a git repository (isolation=worktree requires git)")
		}
	}

	id := fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000_000)
	safe := sanitizeLabel(label)
	branch := fmt.Sprintf("codeforge/subagent-%s-%s", safe, id)
	base := filepath.Join(repoRoot, ".codeforge", "worktrees")
	if err := os.MkdirAll(base, 0755); err != nil {
		return nil, err
	}
	path := filepath.Join(base, safe+"-"+id)

	// Prefer branch from current HEAD
	cmd := exec.Command("git", "-C", repoRoot, "worktree", "add", "-b", branch, path, "HEAD")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git worktree add: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return &WorktreeSession{Path: path, Branch: branch, Root: repoRoot}, nil
}

// Cleanup removes the worktree and optional branch (best-effort).
func (w *WorktreeSession) Cleanup() {
	if w == nil || w.Path == "" {
		return
	}
	_ = exec.Command("git", "-C", w.Root, "worktree", "remove", "--force", w.Path).Run()
	_ = os.RemoveAll(w.Path)
	if w.Branch != "" {
		_ = exec.Command("git", "-C", w.Root, "branch", "-D", w.Branch).Run()
	}
}

func sanitizeLabel(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r == '-' || r == '_' {
			b.WriteByte('-')
		}
		if b.Len() >= 24 {
			break
		}
	}
	if b.Len() == 0 {
		return "task"
	}
	return b.String()
}
