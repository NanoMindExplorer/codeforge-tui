package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStagedWriterPlanModeDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	sw := NewStagedWriter(dir)
	sw.SetMode(ModePlan)

	target := "hello.txt"
	in, _ := json.Marshal(map[string]string{"path": target, "content": "hello world"})
	res := sw.Execute(in)
	if !res.Success {
		t.Fatalf("expected success, got %v", res.Error)
	}
	if !sw.HasPending() {
		t.Fatal("expected pending patch")
	}
	// file must not exist yet
	if _, err := os.Stat(filepath.Join(dir, target)); !os.IsNotExist(err) {
		t.Fatal("file should not be written in Plan mode")
	}
	if res.Diff == "" {
		t.Fatal("expected diff in result")
	}
}

func TestStagedWriterApplyAccepted(t *testing.T) {
	dir := t.TempDir()
	sw := NewStagedWriter(dir)
	sw.SetMode(ModePlan)

	in, _ := json.Marshal(map[string]string{"path": "a.go", "content": "package a\n"})
	_ = sw.Execute(in)
	sw.AcceptAll()
	applied, diff, err := sw.ApplyAccepted()
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 1 {
		t.Fatalf("applied=%d", len(applied))
	}
	data, err := os.ReadFile(filepath.Join(dir, "a.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "package a\n" {
		t.Fatalf("content=%q", data)
	}
	if diff == "" {
		t.Fatal("expected combined diff")
	}
}

func TestStagedWriterActModeWrites(t *testing.T) {
	dir := t.TempDir()
	sw := NewStagedWriter(dir)
	sw.SetMode(ModeAct)
	in, _ := json.Marshal(map[string]string{"path": "b.txt", "content": "act"})
	res := sw.Execute(in)
	if !res.Success {
		t.Fatal(res.Error)
	}
	if sw.HasPending() {
		t.Fatal("act mode should not stage")
	}
	data, err := os.ReadFile(filepath.Join(dir, "b.txt"))
	if err != nil || string(data) != "act" {
		t.Fatalf("write failed: %v %q", err, data)
	}
}

func TestPathSandbox(t *testing.T) {
	dir := t.TempDir()
	sw := NewStagedWriter(dir)
	sw.SetMode(ModeAct)
	in, _ := json.Marshal(map[string]string{"path": "../escape.txt", "content": "x"})
	res := sw.Execute(in)
	if res.Success {
		t.Fatal("should reject path escape")
	}
}
