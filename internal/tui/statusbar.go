package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/codeforge/tui/internal/theme"
)

// StatusBarModel is the Grok-style footer strip.
type StatusBarModel struct {
	width      int
	Provider   string
	ModelName  string
	Mode       string // PROMPT / SCROLL / …
	AgentMode  string // PLAN / ACT
	Branch     string
	Workdir    string
	Cost       float64
	Tokens     int
	Streaming  bool
	Spark      []float64
	GitHubUser string
	GitHubRepo string
	GitHubOK   bool
	BudgetMax  float64
	BudgetWarn bool
	BudgetStop bool
	ThemeName  string
	// Phase 7
	TodoBadge  string // e.g. "2/5"
	BgTasks    int    // running background tasks
	// Phase G4
	Sandbox string // e.g. "SBX:ws" or empty
}

func NewStatusBarModel() StatusBarModel {
	return StatusBarModel{
		Provider:  "gemini",
		Mode:      "PROMPT",
		AgentMode: "PLAN",
		Branch:    "main",
		ThemeName: "groknight",
	}
}

func (s *StatusBarModel) SetSize(w int) { s.width = w }

func (s *StatusBarModel) PushSpark(v float64) {
	s.Spark = append(s.Spark, v)
	if len(s.Spark) > 32 {
		s.Spark = s.Spark[len(s.Spark)-32:]
	}
}

func (s StatusBarModel) Init() tea.Cmd { return nil }

func (s StatusBarModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return s, nil
}

func (s StatusBarModel) View() string { return s.ViewFooter() }

// ViewTop is a slim brand strip (optional; often empty in Grok-like layout).
func (s StatusBarModel) ViewTop() string {
	// Grok puts almost everything in the footer — keep top minimal / empty for space
	return ""
}

// ViewFooter Grok-style bottom info bar.
func (s StatusBarModel) ViewFooter() string {
	t := theme.Current()
	mode := theme.ModeBadge(s.Mode)
	agent := theme.ModeBadge(s.AgentMode)

	model := s.Provider
	if s.ModelName != "" {
		model = s.Provider + " · " + shortModel(s.ModelName)
	}
	if s.Streaming {
		model = "⠋ " + model
	}
	model = lipgloss.NewStyle().Foreground(t.AccentAssistant).Render(model)

	git := lipgloss.NewStyle().Foreground(t.TextMuted).Render("git:" + s.Branch)
	if s.GitHubOK && s.GitHubUser != "" {
		git = lipgloss.NewStyle().Foreground(t.TextSecondary).Render("gh:@" + s.GitHubUser + " · " + s.Branch)
	}

	costPart := fmt.Sprintf("$%.4f", s.Cost)
	if s.BudgetMax > 0 {
		costPart = fmt.Sprintf("$%.4f/$%.2f", s.Cost, s.BudgetMax)
		if s.BudgetStop {
			costPart = "⛔" + costPart
		} else if s.BudgetWarn {
			costPart = "⚠" + costPart
		}
	}
	cost := lipgloss.NewStyle().Foreground(t.TextSecondary).Render(costPart)
	spark := theme.Sparkline(s.Spark)
	clock := lipgloss.NewStyle().Foreground(t.TextMuted).Render(time.Now().Format("15:04"))
	themeN := lipgloss.NewStyle().Foreground(t.TextMuted).Render(theme.Name())
	todo := ""
	if s.TodoBadge != "" {
		todo = lipgloss.NewStyle().Foreground(t.AccentPlan).Render("☑ "+s.TodoBadge) + "  "
	}
	bg := ""
	if s.BgTasks > 0 {
		bg = lipgloss.NewStyle().Foreground(t.Warning).Render(fmt.Sprintf("⟳%d", s.BgTasks)) + "  "
	}
	sbx := ""
	if s.Sandbox != "" {
		sbx = lipgloss.NewStyle().Foreground(t.Warning).Render(s.Sandbox) + "  "
	}

	left := lipgloss.JoinHorizontal(lipgloss.Left, mode, " ", agent, "  ", model, "  ", git)
	right := lipgloss.JoinHorizontal(lipgloss.Right, todo, bg, sbx, spark, "  ", cost, "  ", themeN, "  ", clock)

	pad := s.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if pad < 1 {
		pad = 1
	}
	line := left + strings.Repeat(" ", pad) + right
	return theme.StatusBarStyle(s.width).Padding(0, 1).Render(line)
}

// ViewBottom keeps API used by model (hints row).
func (s StatusBarModel) ViewBottom() string {
	t := theme.Current()
	hints := "tab focus  @ file  / commands  ctrl+k palette  shift+tab build/design/yolo  ctrl+b panels  ? help"
	if theme.CompactMode() {
		hints = "tab · @ · / · ⌘k · S-tab · ?"
	}
	return lipgloss.NewStyle().
		Foreground(t.TextMuted).
		Background(t.BgBase).
		Width(s.width).
		Padding(0, 1).
		Render(hints)
}

func shortModel(id string) string {
	parts := strings.Split(id, "-")
	if len(id) > 22 {
		if len(parts) >= 3 {
			return parts[1] + "-" + parts[2]
		}
		return id[:18] + "…"
	}
	return id
}
