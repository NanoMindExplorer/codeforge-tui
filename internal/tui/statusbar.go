package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/codeforge/tui/internal/keymap"
	"github.com/codeforge/tui/internal/theme"
)

type StatusBarModel struct {
	width     int
	Provider  string
	ModelName string
	Mode      string // NORMAL / INSERT / COMMAND
	AgentMode string // PLAN / ACT
	Branch    string
	Workdir   string
	Cost      float64
	Tokens    int
	Streaming bool
	Spark     []float64 // token rate samples
}

func NewStatusBarModel() StatusBarModel {
	return StatusBarModel{
		Provider:  "gemini",
		Mode:      "NORMAL",
		AgentMode: "PLAN",
		Branch:    "main",
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

func (s StatusBarModel) View() string { return s.ViewTop() }

func (s StatusBarModel) ViewTop() string {
	t := theme.Current()
	brand := lipgloss.NewStyle().Bold(true).Foreground(t.AccentAI).Padding(0, 1).Render("⚡ CodeForge")
	mode := theme.ModeBadge(s.Mode)
	agent := theme.ModeBadge(s.AgentMode)

	aiStatus := s.Provider
	if s.ModelName != "" {
		aiStatus = s.Provider + " · " + shortModel(s.ModelName)
	}
	if s.Streaming {
		aiStatus = "⠋ " + aiStatus
	}

	spark := theme.Sparkline(s.Spark)
	info := fmt.Sprintf(" %s  git:%s  %s  $%.4f ",
		aiStatus, s.Branch, spark, s.Cost)

	helpHint := lipgloss.NewStyle().Foreground(t.TextMuted).Render("?=help  ⌘K  q")

	middleWidth := s.width - lipgloss.Width(brand) - lipgloss.Width(mode) - lipgloss.Width(agent) - lipgloss.Width(helpHint) - 3
	if middleWidth < 0 {
		middleWidth = 0
	}
	middleInfo := lipgloss.NewStyle().Foreground(t.TextSecondary).Width(middleWidth).Render(info)

	return theme.StatusBarStyle(s.width).Render(
		lipgloss.JoinHorizontal(lipgloss.Left, brand, mode, " ", agent, middleInfo, helpHint),
	)
}

func (s StatusBarModel) ViewBottom() string {
	t := theme.Current()
	left := keymap.Default().ShortHelp()
	right := time.Now().Format("15:04")
	pad := s.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if pad < 1 {
		pad = 1
	}
	return lipgloss.NewStyle().
		Background(t.BgElevated).
		Foreground(t.TextMuted).
		Width(s.width).
		Padding(0, 1).
		Render(left + strings.Repeat(" ", pad) + right)
}

func shortModel(id string) string {
	// claude-sonnet-4-20250514 → sonnet-4
	parts := strings.Split(id, "-")
	if len(parts) >= 2 {
		// take last meaningful chunks
		if len(id) > 24 {
			if len(parts) >= 3 {
				return parts[1] + "-" + parts[2]
			}
			return id[:20] + "…"
		}
	}
	return id
}
