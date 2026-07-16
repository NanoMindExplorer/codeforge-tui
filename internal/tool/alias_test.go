package tool

import (
	"encoding/json"
	"testing"
)

func TestGrokAliasesRegistered(t *testing.T) {
	r := NewRegistry(t.TempDir())
	aliases := []string{"grep", "run_terminal_command", "web_fetch", "list_directory", "edit_file"}
	for _, a := range aliases {
		tt, ok := r.Get(a)
		if !ok {
			t.Fatalf("missing alias %s", a)
		}
		if tt.Name() != a {
			t.Fatal(tt.Name())
		}
	}
	// execute alias
	g, _ := r.Get("grep")
	res := g.Execute(json.RawMessage(`{"pattern":"package","path":"."}`))
	// may fail if no matches in empty dir — should not be "unknown tool"
	if res.Error == "unknown tool" {
		t.Fatal(res)
	}
}

func TestWebSearchSchema(t *testing.T) {
	w := &WebSearch{}
	if w.Name() != "web_search" {
		t.Fatal(w.Name())
	}
	_ = w.Schema()
}

func TestMemoryWriteSearch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	mw := &MemoryWrite{}
	res := mw.Execute(json.RawMessage(`{"text":"use go modules always","tags":"go"}`))
	if !res.Success {
		t.Fatal(res.Error)
	}
	ms := &MemorySearch{}
	res = ms.Execute(json.RawMessage(`{"query":"go modules"}`))
	if !res.Success || res.Output == "" {
		t.Fatal(res)
	}
}
