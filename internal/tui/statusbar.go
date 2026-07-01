package tui

import (
    "fmt"
    "strings"
    "time"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
)

type StatusBarModel struct {
    width     int
    Provider  string
    Mode      string
    Branch    string
    Workdir   string
    Cost      float64
    Tokens    int
    Streaming bool // true saat AI sedang streaming atau agent berjalan
}

func NewStatusBarModel() StatusBarModel {
    return StatusBarModel{
        Provider: "claude",
        Mode:     "NORMAL",
        Branch:   "main",
    }
}

func (s *StatusBarModel) SetSize(w int) {
    s.width = w
}

func (s StatusBarModel) Init() tea.Cmd { return nil }

func (s StatusBarModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    return s, nil
}

func (s StatusBarModel) View() string {
    return s.ViewTop()
}

func (s StatusBarModel) ViewTop() string {
    brandStyle := lipgloss.NewStyle().
        Bold(true).
        Foreground(lipgloss.Color("#06B6D4")).
        Padding(0, 1)

    modeStyle := lipgloss.NewStyle().
        Bold(true).
        Background(modeColor(s.Mode)).
        Foreground(lipgloss.Color("#FFFFFF")).
        Padding(0, 1)

    infoStyle := lipgloss.NewStyle().
        Foreground(lipgloss.Color("#94A3B8")).
        Padding(0, 1)

    brand := brandStyle.Render("⚡ CodeForge")
    mode := modeStyle.Render(s.Mode)

    aiStatus := s.Provider
    if s.Streaming {
        aiStatus = "⠋ " + s.Provider + " …"
    }

    info := fmt.Sprintf("  %s  git:%s  tok:%d  $%.4f  ",
        aiStatus, s.Branch, s.Tokens, s.Cost)

    helpHint := infoStyle.Render("?=help  /=cmd  q=quit")

    middleWidth := s.width - lipgloss.Width(brand) - lipgloss.Width(mode) - lipgloss.Width(helpHint) - 2
    if middleWidth < 0 {
        middleWidth = 0
    }
    middleInfo := infoStyle.Width(middleWidth).Render(info)

    return lipgloss.NewStyle().
        Background(lipgloss.Color("#1E293B")).
        Width(s.width).
        Render(lipgloss.JoinHorizontal(lipgloss.Left, brand, mode, middleInfo, helpHint))
}

func (s StatusBarModel) ViewBottom() string {
    style := lipgloss.NewStyle().
        Background(lipgloss.Color("#1E293B")).
        Foreground(lipgloss.Color("#94A3B8")).
        Width(s.width).
        Padding(0, 1)

    left := "i:chat  I:/act  /:cmd  1-3:pane  Tab:next  j/k:scroll  q:quit"
    right := time.Now().Format("15:04")

    padding := s.width - len(left) - len(right) - 2
    if padding < 0 {
        padding = 0
    }
    return style.Render(left + strings.Repeat(" ", padding) + right)
}

func modeColor(mode string) lipgloss.Color {
    switch mode {
    case "NORMAL":
        return lipgloss.Color("#10B981")
    case "INSERT":
        return lipgloss.Color("#F59E0B")
    case "COMMAND":
        return lipgloss.Color("#318AB7")
    }
    return lipgloss.Color("#64748B")
}
