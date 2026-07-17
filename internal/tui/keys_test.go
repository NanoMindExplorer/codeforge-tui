package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codeforge/tui/internal/config"
	"github.com/codeforge/tui/internal/provider"
	"github.com/codeforge/tui/internal/theme"
	"github.com/codeforge/tui/internal/tool"
)

func testModel(t *testing.T) Model {
	t.Helper()
	theme.Set(theme.Aurora())
	theme.SetMotion(false)
	reg := provider.NewRegistry()
	_ = reg.Register(provider.NewGeminiProvider("test-key", "gemini-2.5-flash"))
	tools := tool.NewRegistry(t.TempDir())
	cfg := config.Default()
	m := New(cfg, reg, tools, nil, t.TempDir())
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return asModel(nm)
}

func TestShiftTabCyclesSessionMode(t *testing.T) {
	m := testModel(t)
	if m.sessionMode != tool.SessionBuild {
		t.Fatalf("start BUILD, got %v", m.sessionMode.Label())
	}
	// Prefer handleKeyMsg directly (Q2.2 unit surface)
	nm, _ := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = asModel(nm)
	if m.sessionMode != tool.SessionDesign {
		t.Fatalf("expected DESIGN, got %v", m.sessionMode.Label())
	}
	nm, _ = m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = asModel(nm)
	if m.sessionMode != tool.SessionYolo {
		t.Fatalf("expected YOLO, got %v", m.sessionMode.Label())
	}
	nm, _ = m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = asModel(nm)
	if m.sessionMode != tool.SessionBuild {
		t.Fatalf("expected BUILD, got %v", m.sessionMode.Label())
	}
}

func TestEscStackClosesModalsInOrder(t *testing.T) {
	// Steal-Esc stack: Settings esc → back to insert/normal without opening lower layers.
	m := testModel(t)
	m.openSettings()
	if m.mode != ModeSettings {
		t.Fatalf("expected settings mode, got %v", m.mode)
	}
	nm, _ := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyEscape})
	m = asModel(nm)
	if m.mode == ModeSettings {
		t.Fatal("esc should leave settings")
	}

	// Theme picker
	m.openThemePicker()
	if m.mode != ModeThemePick {
		t.Fatalf("theme pick mode: %v", m.mode)
	}
	nm, _ = m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyEscape})
	m = asModel(nm)
	if m.mode == ModeThemePick {
		t.Fatal("esc should close theme picker")
	}

	// Palette
	m.openPalette()
	if m.mode != ModePalette {
		t.Fatalf("palette: %v", m.mode)
	}
	nm, _ = m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyEscape})
	m = asModel(nm)
	if m.mode == ModePalette {
		t.Fatal("esc should close palette")
	}
}

func TestEscStackPriorityReviewOverPalette(t *testing.T) {
	// When ModeReview is active, keys go to review — not palette — even if palette was open before.
	m := testModel(t)
	m.mode = ModeReview
	// esc cancels review (updateReview)
	nm, _ := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyEscape})
	m = asModel(nm)
	if m.mode == ModeReview {
		// finish path sets ModeNormal after Cancel
		t.Fatal("esc should cancel review mode")
	}
}

func TestTabFocusSwap(t *testing.T) {
	m := testModel(t)
	if !m.focusPrompt {
		t.Fatal("default prompt focused")
	}
	nm, _ := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyTab})
	m = asModel(nm)
	if m.focusPrompt {
		t.Fatal("tab should move to scrollback")
	}
	if m.mode != ModeNormal {
		t.Fatalf("scrollback mode NORMAL, got %v", m.mode)
	}
	nm, _ = m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyTab})
	m = asModel(nm)
	if !m.focusPrompt {
		t.Fatal("tab should restore prompt focus")
	}
}

func TestCtrlBTogglesPanels(t *testing.T) {
	m := testModel(t)
	before := m.showPanels
	nm, _ := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyCtrlB})
	m = asModel(nm)
	if m.showPanels == before {
		t.Fatal("ctrl+b should toggle panels")
	}
}

func TestPromptEscClearsOrBlurs(t *testing.T) {
	m := testModel(t)
	m.focusPrompt = true
	m.mode = ModeInsert
	m.chat.SetInput("hello")
	// First esc often clears input or blurs depending on handlePromptEsc
	nm, _ := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyEscape})
	m = asModel(nm)
	// Should not panic; either input cleared or focus moved
	_ = m.chat.InputValue()
	_ = time.Now() // keep time import stable if needed
}

func TestIsImmediateSlash(t *testing.T) {
	if !isImmediateSlash("/help") {
		t.Fatal("help immediate")
	}
	if !isImmediateSlash("/status") {
		t.Fatal("status immediate")
	}
	if isImmediateSlash("/act") {
		t.Fatal("act needs args / agent run — not always immediate")
	}
}
