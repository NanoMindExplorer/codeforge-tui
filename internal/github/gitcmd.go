package github

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func runGitCmd(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	out := strings.TrimSpace(stdout.String())
	errOut := strings.TrimSpace(stderr.String())
	if out == "" {
		return errOut, nil
	}
	if errOut != "" {
		return out + "\n" + errOut, nil
	}
	return out, nil
}

// CreateBranch creates and checks out a new branch.
func (c *Client) CreateBranch(ctx context.Context, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("branch name required")
	}
	return runGitCmd(ctx, c.WorkDir, "checkout", "-b", name)
}

// CurrentBranch returns the current branch name.
func (c *Client) CurrentBranch(ctx context.Context) (string, error) {
	return runGitCmd(ctx, c.WorkDir, "branch", "--show-current")
}

// Pull runs git pull --ff-only (or rebase if requested later).
func (c *Client) Pull(ctx context.Context) (string, error) {
	out, err := runGitCmd(ctx, c.WorkDir, "pull", "--ff-only")
	if err != nil {
		// fallback plain pull
		return runGitCmd(ctx, c.WorkDir, "pull")
	}
	return out, nil
}

// LogRecent returns short git log.
func (c *Client) LogRecent(ctx context.Context, n int) (string, error) {
	if n <= 0 {
		n = 10
	}
	return runGitCmd(ctx, c.WorkDir, "log", fmt.Sprintf("-%d", n), "--oneline", "--decorate")
}
