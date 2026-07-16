package hooks

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPreToolUseDeny(t *testing.T) {
	dir := t.TempDir()
	hookDir := filepath.Join(dir, ".codeforge", "hooks")
	_ = os.MkdirAll(hookDir, 0755)
	// exit 2 = deny
	content := `{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "run_command",
        "hooks": [
          { "type": "command", "command": "exit 2" }
        ]
      }
    ]
  }
}`
	_ = os.WriteFile(filepath.Join(hookDir, "deny.json"), []byte(content), 0644)
	r := Load(dir)
	if r.Count() < 1 {
		t.Fatal("no hooks")
	}
	deny, reason := r.PreToolUse(context.Background(), "run_command", `{"command":"echo hi"}`)
	if !deny {
		t.Fatalf("expected deny, reason=%s", reason)
	}
}

func TestPreToolUseAllow(t *testing.T) {
	dir := t.TempDir()
	hookDir := filepath.Join(dir, ".codeforge", "hooks")
	_ = os.MkdirAll(hookDir, 0755)
	content := `{
  "hooks": {
    "PreToolUse": [
      {
        "hooks": [
          { "type": "command", "command": "exit 0" }
        ]
      }
    ]
  }
}`
	_ = os.WriteFile(filepath.Join(hookDir, "ok.json"), []byte(content), 0644)
	r := Load(dir)
	deny, _ := r.PreToolUse(context.Background(), "read_file", `{}`)
	if deny {
		t.Fatal("should allow")
	}
}
