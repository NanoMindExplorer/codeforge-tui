package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseProfile(t *testing.T) {
	cases := map[string]Profile{
		"off": Off, "workspace": Workspace, "read-only": ReadOnly,
		"readonly": ReadOnly, "strict": Strict, "devbox": Devbox,
	}
	for in, want := range cases {
		p, ok := ParseProfile(in)
		if !ok || p != want {
			t.Fatalf("%s → %s ok=%v", in, p, ok)
		}
	}
	if _, ok := ParseProfile("nope"); ok {
		t.Fatal("expected fail")
	}
}

func TestCheckWriteWorkspace(t *testing.T) {
	dir := t.TempDir()
	e := Ensure(Workspace, dir)
	inside := filepath.Join(dir, "a.txt")
	if err := e.CheckWrite(inside); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(os.TempDir(), "codeforge-sbx-outside-"+t.Name())
	// /tmp is allowed
	if err := e.CheckWrite(outside); err != nil {
		t.Fatal("tmp should be writable:", err)
	}
	// parent of workdir should not
	parentFile := filepath.Join(filepath.Dir(dir), "escape.txt")
	if err := e.CheckWrite(parentFile); err == nil {
		// if parent is under /tmp it may pass — skip if so
		if !isTemp(parentFile) {
			t.Fatal("expected write deny for parent")
		}
	}
}

func TestCheckWriteReadOnly(t *testing.T) {
	dir := t.TempDir()
	e := Ensure(ReadOnly, dir)
	if err := e.CheckWrite(filepath.Join(dir, "x.go")); err == nil {
		t.Fatal("project write should fail in read-only")
	}
	cf := e.CodeforgeHome
	_ = os.MkdirAll(cf, 0755)
	if err := e.CheckWrite(filepath.Join(cf, "note.txt")); err != nil {
		t.Fatal(err)
	}
}

func TestCheckReadStrict(t *testing.T) {
	dir := t.TempDir()
	e := Ensure(Strict, dir)
	if err := e.CheckRead(filepath.Join(dir, "main.go")); err != nil {
		t.Fatal(err)
	}
	// something clearly outside
	home, _ := os.UserHomeDir()
	foreign := filepath.Join(home, ".ssh", "id_rsa")
	// may or may not exist; policy should deny if not under allowed roots
	if under(foreign, dir) || under(foreign, e.CodeforgeHome) {
		t.Skip("odd layout")
	}
	if err := e.CheckRead(foreign); err == nil {
		// .ssh might be under home which is not allowed in strict
		t.Fatal("expected strict read deny for ~/.ssh")
	}
}

func TestDenyGlob(t *testing.T) {
	dir := t.TempDir()
	e := Ensure(Workspace, dir)
	e.Deny = []string{"**/.env", "**/*.pem"}
	if err := e.CheckWrite(filepath.Join(dir, ".env")); err == nil {
		t.Fatal("deny .env")
	}
	if err := e.CheckRead(filepath.Join(dir, "secret.pem")); err == nil {
		t.Fatal("deny pem")
	}
	if err := e.CheckWrite(filepath.Join(dir, "ok.go")); err != nil {
		t.Fatal(err)
	}
}

func TestCommandSoft(t *testing.T) {
	dir := t.TempDir()
	e := Ensure(Workspace, dir)
	// force soft
	e.Backend = BackendSoft
	cmd, err := e.Command(context.Background(), "echo hi")
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		// unshare -n may fail in restricted envs — retry without network restrict
		e.RestrictNetwork = false
		cmd, err = e.Command(context.Background(), "echo hi")
		if err != nil {
			t.Fatal(err)
		}
		out, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err, string(out))
		}
	}
	if !strings.Contains(string(out), "hi") {
		t.Fatal(string(out))
	}
}

func TestSummaryLabel(t *testing.T) {
	e := Ensure(Off, t.TempDir())
	if e.Label() != "" {
		t.Fatal(e.Label())
	}
	e = Ensure(Strict, t.TempDir())
	if e.Label() != "SBX:strict" {
		t.Fatal(e.Label())
	}
	if !strings.Contains(e.Summary(), "strict") {
		t.Fatal(e.Summary())
	}
}

func TestResolvePreferExplicit(t *testing.T) {
	t.Setenv("CODEFORGE_SANDBOX", "workspace")
	p := ResolvePreferExplicit(true, "strict", "off")
	if p != Strict {
		t.Fatal(p)
	}
	p = ResolvePreferExplicit(false, "", "read-only")
	// env wins over config when flag not set
	if p != Workspace {
		t.Fatal(p)
	}
}
