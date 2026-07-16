package git

import (
    "fmt"
    "os/exec"
    "strings"
    "time"

    "github.com/go-git/go-git/v5"
    "github.com/go-git/go-git/v5/plumbing"
    "github.com/go-git/go-git/v5/plumbing/object"
)

type Repo struct {
    repo   *git.Repository
    workdir string
}

func Open(path string) (*Repo, error) {
    repo, err := git.PlainOpen(path)
    if err != nil {
        if err == git.ErrRepositoryNotExists {
            repo, err = git.PlainInit(path, false)
            if err != nil {
                return nil, fmt.Errorf("git init: %w", err)
            }
        } else {
            return nil, fmt.Errorf("git open: %w", err)
        }
    }
    return &Repo{repo: repo, workdir: path}, nil
}

func (r *Repo) Status() (string, error) {
    wt, err := r.repo.Worktree()
    if err != nil {
        return "", err
    }
    status, err := wt.Status()
    if err != nil {
        return "", err
    }
    if status.IsClean() {
        return "Working tree clean", nil
    }
    var sb strings.Builder
    for file, s := range status {
        sb.WriteString(fmt.Sprintf("  %c%c  %s\n", s.Staging, s.Worktree, file))
    }
    return sb.String(), nil
}

func (r *Repo) AddAll() error {
    wt, err := r.repo.Worktree()
    if err != nil {
        return err
    }
    return wt.AddWithOptions(&git.AddOptions{All: true})
}

func (r *Repo) Commit(message string) (string, error) {
    wt, err := r.repo.Worktree()
    if err != nil {
        return "", err
    }
    hash, err := wt.Commit(message, &git.CommitOptions{
        Author: &object.Signature{
            Name:  "CodeForge TUI",
            Email: "codeforge@local",
            When:  time.Now(),
        },
    })
    if err != nil {
        return "", err
    }
    return hash.String()[:8], nil
}

func (r *Repo) Branch() (string, error) {
    head, err := r.repo.Head()
    if err != nil {
        return "", err
    }
    return head.Name().Short(), nil
}

func GenerateCommitMessage(changeType, scope, subject string) string {
    if changeType == "" {
        changeType = "chore"
    }
    if scope != "" {
        return fmt.Sprintf("%s(%s): %s", changeType, scope, subject)
    }
    return fmt.Sprintf("%s: %s", changeType, subject)
}

// Push pushes the current branch to origin (sets upstream on first push).
func (r *Repo) Push() (string, error) {
    cmd := exec.Command("git", "push", "-u", "origin", "HEAD")
    cmd.Dir = r.workdir
    out, err := cmd.CombinedOutput()
    if err != nil {
        return string(out), fmt.Errorf("git push: %w: %s", err, strings.TrimSpace(string(out)))
    }
    return strings.TrimSpace(string(out)), nil
}

// Pull fast-forwards from origin.
func (r *Repo) Pull() (string, error) {
    cmd := exec.Command("git", "pull", "--ff-only")
    cmd.Dir = r.workdir
    out, err := cmd.CombinedOutput()
    if err != nil {
        cmd2 := exec.Command("git", "pull")
        cmd2.Dir = r.workdir
        out2, err2 := cmd2.CombinedOutput()
        if err2 != nil {
            return string(out2), fmt.Errorf("git pull: %w: %s", err2, strings.TrimSpace(string(out2)))
        }
        return strings.TrimSpace(string(out2)), nil
    }
    return strings.TrimSpace(string(out)), nil
}

// CreateBranch creates and checks out name.
func (r *Repo) CreateBranch(name string) error {
    wt, err := r.repo.Worktree()
    if err != nil {
        return err
    }
    head, err := r.repo.Head()
    if err != nil {
        return err
    }
    ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName(name), head.Hash())
    if err := r.repo.Storer.SetReference(ref); err != nil {
        return err
    }
    return wt.Checkout(&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName(name)})
}

// RemoteURL returns origin URL if set.
func (r *Repo) RemoteURL() (string, error) {
    rem, err := r.repo.Remote("origin")
    if err != nil {
        return "", err
    }
    cfg := rem.Config()
    if len(cfg.URLs) == 0 {
        return "", fmt.Errorf("origin has no URLs")
    }
    return cfg.URLs[0], nil
}
