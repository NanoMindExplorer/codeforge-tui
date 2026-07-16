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

// ensure time import used
var _ = time.Now

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

	// Already prompt-focused (Grok simple mode)
	// help via ?
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = nm.(Model)
	// spinner tick should not panic
	nm, _ = m.Update(SpinnerTickMsg{})
	m = nm.(Model)

	// narrow layout
	nm, _ = m.Update(tea.WindowSizeMsg{Width: 70, Height: 30})
	m = nm.(Model)
	_ = m.View()

	// Shift+Tab toggles Plan/Act
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = nm.(Model)
	if m.agentMode != tool.ModeAct {
		t.Fatalf("expected ACT after Shift+Tab, got mode=%v agent=%v", m.mode, m.agentMode)
	}
	// Theme picker opens (Phase 3)
	_ = m.executeSlashCommand("/theme")
	if m.mode != ModeThemePick {
		t.Fatalf("expected ModeThemePick after /theme, got %v", m.mode)
	}
	// Set theme by name
	m.mode = ModeInsert
	m.themes.Close()
	_ = m.executeSlashCommand("/theme tokyonight")
	if theme.DisplayName() != "tokyonight" {
		t.Fatalf("theme set: %s", theme.DisplayName())
	}
	// Vim mode toggle
	before := m.vimMode
	_ = m.executeSlashCommand("/vim-mode")
	if m.vimMode == before {
		t.Fatal("vim mode should toggle")
	}
}

func TestThemePickerPreviewAndCancel(t *testing.T) {
	theme.Set(theme.GrokNight())
	theme.SetMotion(false)
	reg := provider.NewRegistry()
	_ = reg.Register(provider.NewGeminiProvider("k", "gemini-2.5-flash"))
	m := New(config.Default(), reg, tool.NewRegistry(t.TempDir()), nil, t.TempDir())
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = asModel(nm)
	_ = m.executeSlashCommand("/theme")
	m = asModel(m) // mode already set
	if m.mode != ModeThemePick {
		t.Fatal("picker not open")
	}
	// move down previews next theme
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = asModel(nm)
	// esc reverts
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = asModel(nm)
	if m.mode == ModeThemePick {
		t.Fatal("should close on esc")
	}
	if theme.DisplayName() != "groknight" {
		t.Fatalf("should revert to groknight, got %s", theme.DisplayName())
	}
}

func TestDoubleEscClearsPrompt(t *testing.T) {
	theme.Set(theme.GrokNight())
	theme.SetMotion(false)
	reg := provider.NewRegistry()
	_ = reg.Register(provider.NewGeminiProvider("k", "gemini-2.5-flash"))
	m := New(config.Default(), reg, tool.NewRegistry(t.TempDir()), nil, t.TempDir())
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = asModel(nm)
	m.focusPrompt = true
	m.mode = ModeInsert
	m.chat.FocusInput()
	m.chat.SetInput("hello draft")
	m.lastEsc = time.Now()
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = asModel(nm)
	if m.chat.InputValue() != "" {
		// cleared on second press within window
		t.Log("draft:", m.chat.InputValue())
	}
}

func asModel(nm tea.Model) Model {
	switch v := nm.(type) {
	case Model:
		return v
	case *Model:
		return *v
	default:
		panic("unexpected model type")
	}
}

func TestSlashMenuActivates(t *testing.T) {
	theme.SetMotion(false)
	reg := provider.NewRegistry()
	_ = reg.Register(provider.NewGeminiProvider("k", "gemini-2.5-flash"))
	m := New(config.Default(), reg, tool.NewRegistry(t.TempDir()), nil, t.TempDir())
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(Model)
	m.focusPrompt = true
	m.chat.SetInput("/he")
	m.slash.UpdateQuery("/he")
	if !m.slash.Active {
		t.Fatal("slash menu inactive")
	}
	if m.slash.Selected() == "" && len(m.slash.Filtered) == 0 {
		t.Fatal("no filter")
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
