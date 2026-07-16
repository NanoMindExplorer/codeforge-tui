package personas

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadYAMLPersona(t *testing.T) {
	dir := t.TempDir()
	pd := filepath.Join(dir, ".codeforge", "personas")
	_ = os.MkdirAll(pd, 0755)
	_ = os.WriteFile(filepath.Join(pd, "strict.yaml"), []byte(`
name: strict
description: Strict style
instructions: |
  Always verify with tests.
  No untested changes.
`), 0644)

	r := Load(Options{WorkDir: dir})
	p, ok := r.Get("strict")
	if !ok {
		t.Fatal("missing strict")
	}
	if !strings.Contains(p.Resolved, "verify with tests") {
		t.Fatal(p.Resolved)
	}
	// bundled still present
	if _, ok := r.Get("researcher"); !ok {
		t.Fatal("bundled researcher")
	}
}

func TestConfigOverrides(t *testing.T) {
	r := Load(Options{
		WorkDir: t.TempDir(),
		ConfigPersonas: map[string]Persona{
			"researcher": {
				Instructions: "CONFIG OVERRIDE RESEARCHER",
				Description:  "from config",
			},
		},
	})
	p, ok := r.Get("researcher")
	if !ok {
		t.Fatal("missing")
	}
	if !strings.Contains(p.Resolved, "CONFIG OVERRIDE") {
		t.Fatal(p.Resolved)
	}
	rem := p.SystemReminder()
	if !strings.Contains(rem, "<system-reminder>") {
		t.Fatal(rem)
	}
}

func TestParseTOML(t *testing.T) {
	raw := `
name = "toml-p"
description = "from toml"
instructions = '''
Line one
Line two
'''
`
	p, err := parseTOML(raw, "/tmp/x.toml")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "toml-p" {
		t.Fatal(p.Name)
	}
	if !strings.Contains(p.Instructions, "Line one") {
		t.Fatal(p.Instructions)
	}
}
