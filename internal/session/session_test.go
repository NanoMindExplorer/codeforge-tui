package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/codeforge/tui/internal/provider"
)

func TestSaveAndLoad(t *testing.T) {
	// redirect home
	home := t.TempDir()
	t.Setenv("HOME", home)

	s := New("gemini", "gemini-2.5-flash", "/tmp/proj")
	s.Messages = []provider.Message{
		{Role: provider.RoleUser, Content: "hello world test session"},
		{Role: provider.RoleAssistant, Content: "hi"},
	}
	s.TotalCost = 0.01
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	// verify file exists under ~/.codeforge/sessions
	dir := filepath.Join(home, ".codeforge", "sessions")
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		t.Fatalf("no session files: %v", err)
	}
	loaded, err := Load(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Messages) != 2 {
		t.Fatalf("messages=%d", len(loaded.Messages))
	}
	if loaded.Preview == "" {
		t.Fatal("expected preview")
	}
}

func TestList(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for i := 0; i < 3; i++ {
		s := New("claude", "sonnet", ".")
		s.Messages = []provider.Message{{Role: provider.RoleUser, Content: "msg"}}
		if err := s.Save(); err != nil {
			t.Fatal(err)
		}
	}
	list, err := List(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 1 {
		t.Fatal("expected sessions")
	}
}
