package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codeforge/tui/internal/config"
	"github.com/codeforge/tui/internal/provider"
	"github.com/codeforge/tui/internal/theme"
	"github.com/codeforge/tui/internal/tool"
)

// Smoke test: construct model, send window size + keys, ensure View does not panic
// and contains key brand strings.
func TestSmokeRender(t *testing.T) {
	theme.Set(theme.Aurora())
	theme.SetMotion(false)

	reg := provider.NewRegistry()
	_ = reg.Register(provider.NewGeminiProvider("test-key", "gemini-2.5-flash"))
	tools := tool.NewRegistry(t.TempDir())
	cfg := config.Default()

	m := New(cfg, reg, tools, nil, t.TempDir())
	// size
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = nm.(Model)
	view := m.View()
	if !strings.Contains(view, "CodeForge") {
		t.Fatalf("view missing brand:\n%s", truncateView(view))
	}

	// press i (insert)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m = nm.(Model)
	// help
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = nm.(Model)
	// spinner tick should not panic
	nm, _ = m.Update(SpinnerTickMsg{})
	m = nm.(Model)

	// compact layout
	nm, _ = m.Update(tea.WindowSizeMsg{Width: 70, Height: 30})
	m = nm.(Model)
	_ = m.View()

	// back to NORMAL then toggle plan/act with Shift+P
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(Model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("P")})
	m = nm.(Model)
	if m.agentMode != tool.ModeAct {
		t.Fatalf("expected ACT after Shift+P, got mode=%v agent=%v", m.mode, m.agentMode)
	}
}

func TestModelSwitchSlash(t *testing.T) {
	theme.SetMotion(false)
	reg := provider.NewRegistry()
	_ = reg.Register(provider.NewGeminiProvider("k", "gemini-2.5-flash"))
	tools := tool.NewRegistry(t.TempDir())
	m := New(config.Default(), reg, tools, nil, t.TempDir())
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	_ = m.executeSlashCommand("/model gemini-2.5-pro")
	cur, _ := reg.Current()
	if cur.Model() != "gemini-2.5-pro" {
		t.Fatalf("model not switched: %s", cur.Model())
	}
}

func TestContextUpdateMsgWires(t *testing.T) {
	theme.SetMotion(false)
	reg := provider.NewRegistry()
	_ = reg.Register(provider.NewGeminiProvider("k", "gemini-2.5-flash"))
	dir := t.TempDir()
	tools := tool.NewRegistry(dir)
	m := New(config.Default(), reg, tools, nil, dir)
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(Model)
	nm, _ = m.Update(ContextUpdateMsg{Refresh: true})
	m = nm.(Model)
	// files list should be non-nil (may be empty dir)
	_ = m.context.View()
}

func truncateView(s string) string {
	if len(s) > 200 {
		return s[:200]
	}
	return s
}

func TestCalculateCostNotAlwaysZero(t *testing.T) {
	// provider.CostForModel is the source of truth now
	p := provider.NewGeminiProvider("k", "gemini-2.5-pro")
	c := provider.CostForModel(p, "gemini-2.5-pro", 100_000, 100_000)
	if c == 0 {
		t.Fatal("gemini pro cost should not be zero")
	}
	_ = time.Now()
}
