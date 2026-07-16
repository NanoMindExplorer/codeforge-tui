package index

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildAndSearch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.go"), []byte("package main\nfunc DoThing() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Hello\nDoThing docs\n"), 0644); err != nil {
		t.Fatal(err)
	}
	idx, err := Build(dir)
	if err != nil {
		t.Fatal(err)
	}
	f, s := idx.Stats()
	if f < 2 {
		t.Fatalf("files=%d", f)
	}
	if s < 1 {
		t.Fatalf("symbols=%d", s)
	}
	hits := idx.Search("DoThing", 5)
	if len(hits) == 0 {
		t.Fatal("no hits")
	}
	if hits[0].Path != "hello.go" && hits[0].Path != "README.md" {
		// either is fine; prefer go usually
		t.Log(hits[0].Path)
	}
}
