package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGlobSearchBasic(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "b.ts"), []byte("export {}"), 0644)
	sub := filepath.Join(dir, "pkg")
	_ = os.MkdirAll(sub, 0755)
	_ = os.WriteFile(filepath.Join(sub, "c.go"), []byte("package pkg"), 0644)
	// ignored dir
	_ = os.MkdirAll(filepath.Join(dir, "node_modules"), 0755)
	_ = os.WriteFile(filepath.Join(dir, "node_modules", "x.go"), []byte("x"), 0644)

	g := &GlobSearch{WorkDir: dir}
	res := g.Execute(json.RawMessage(`{"pattern":"**/*.go"}`))
	if !res.Success {
		t.Fatal(res.Error)
	}
	if !strings.Contains(res.Output, "a.go") || !strings.Contains(res.Output, "pkg/c.go") {
		t.Fatal(res.Output)
	}
	if strings.Contains(res.Output, "node_modules") {
		t.Fatal("should ignore node_modules:", res.Output)
	}
}

func TestGlobSearchSimplePattern(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# hi"), 0644)
	g := &GlobSearch{WorkDir: dir}
	res := g.Execute(json.RawMessage(`{"pattern":"*.md"}`))
	if !res.Success || !strings.Contains(res.Output, "readme.md") {
		t.Fatal(res)
	}
}

func TestMatchStarStar(t *testing.T) {
	if !matchStarStar("**/*.go", "internal/tool/glob.go") {
		t.Fatal("expected match")
	}
	if matchStarStar("**/*.ts", "internal/tool/glob.go") {
		t.Fatal("unexpected match")
	}
	if !matchStarStar("src/**/*.tsx", "src/components/App.tsx") {
		t.Fatal("prefix ** match")
	}
}

func TestGlobRegistered(t *testing.T) {
	r := NewRegistry(t.TempDir())
	for _, name := range []string{"glob_file_search", "glob", "find_files"} {
		if _, ok := r.Get(name); !ok {
			t.Fatalf("missing %s", name)
		}
	}
}
