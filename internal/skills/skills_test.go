package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	raw := `---
name: commit
description: Create conventional commits. Use when user wants to commit.
when-to-use: commit changes
user-invocable: true
---

# Git Commit

1. Run git status
2. Commit
`
	sk, err := Parse(raw, "/tmp/x/SKILL.md", SourceUser)
	if err != nil {
		t.Fatal(err)
	}
	if sk.Name != "commit" {
		t.Fatal(sk.Name)
	}
	if !strings.Contains(sk.Description, "conventional") {
		t.Fatal(sk.Description)
	}
	if !strings.Contains(sk.Body, "git status") {
		t.Fatal(sk.Body)
	}
	if !sk.UserInvocable {
		t.Fatal("invocable")
	}
}

func TestLoadDiscover(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, ".codeforge", "skills", "review-pr")
	_ = os.MkdirAll(skillDir, 0755)
	content := `---
name: review-pr
description: Review pull requests carefully.
---
Look at the diff and comment.
`
	_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644)

	// lower priority user skill with same name should not override
	home := t.TempDir()
	t.Setenv("HOME", home)
	userDir := filepath.Join(home, ".codeforge", "skills", "review-pr")
	_ = os.MkdirAll(userDir, 0755)
	_ = os.WriteFile(filepath.Join(userDir, "SKILL.md"), []byte(`---
name: review-pr
description: USER copy
---
user body
`), 0644)

	r := Load(Options{WorkDir: dir, CompatClaude: true})
	sk, ok := r.Get("review-pr")
	if !ok {
		t.Fatal("missing skill")
	}
	if sk.Source != SourceLocal {
		t.Fatalf("expected local, got %s", sk.Source)
	}
	if strings.Contains(sk.Description, "USER") {
		t.Fatal("local should win over user")
	}
	if r.Count() < 1 {
		t.Fatal(r.Count())
	}
	cat := r.CatalogForPrompt()
	if !strings.Contains(cat, "/review-pr") {
		t.Fatal(cat)
	}
	prompt := sk.InvokePrompt("fix typo")
	if !strings.Contains(prompt, "Look at the diff") || !strings.Contains(prompt, "fix typo") {
		t.Fatal(prompt)
	}
}

func TestCommandsFlat(t *testing.T) {
	dir := t.TempDir()
	cmdDir := filepath.Join(dir, ".codeforge", "commands")
	_ = os.MkdirAll(cmdDir, 0755)
	_ = os.WriteFile(filepath.Join(cmdDir, "ship.md"), []byte("# Ship\n\nDeploy to prod carefully."), 0644)
	r := Load(Options{WorkDir: dir})
	sk, ok := r.Get("ship")
	if !ok {
		t.Fatal("missing ship command skill")
	}
	if !sk.UserInvocable {
		t.Fatal("should be invocable")
	}
}

func TestDisabled(t *testing.T) {
	dir := t.TempDir()
	sd := filepath.Join(dir, ".codeforge", "skills", "wip")
	_ = os.MkdirAll(sd, 0755)
	_ = os.WriteFile(filepath.Join(sd, "SKILL.md"), []byte("---\nname: wip\ndescription: WIP\n---\nbody"), 0644)
	r := Load(Options{WorkDir: dir, Disabled: []string{"wip"}})
	sk, ok := r.Get("wip")
	if !ok || !sk.Disabled {
		t.Fatal("expected disabled")
	}
	if strings.Contains(r.CatalogForPrompt(), "wip") {
		t.Fatal("disabled should not appear in catalog")
	}
}

func TestNormalizeName(t *testing.T) {
	if normalizeName("Review_PR") != "review-pr" {
		t.Fatal(normalizeName("Review_PR"))
	}
}
