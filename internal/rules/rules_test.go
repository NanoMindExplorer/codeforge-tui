package rules

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAGENTS(t *testing.T) {
	dir := t.TempDir()
	content := "# Rules\n- Always use tabs\n"
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	b := Load(dir)
	if b.Text == "" || b.Primary == "" {
		t.Fatalf("%+v", b)
	}
	sys := Inject("base prompt", b)
	if !contains(sys, "Always use tabs") {
		t.Fatal(sys)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || index(s, sub) >= 0)
}

func index(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
