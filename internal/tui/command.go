package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/codeforge/tui/internal/theme"
)

type CommandModel struct {
	width      int
	height     int
	active     bool
	input      string
	Done       bool
	FinalValue string
}

func NewCommandModel() CommandModel { return CommandModel{} }

func (c *CommandModel) SetSize(w, h int) {
	c.width = w
	c.height = h
}

func (c CommandModel) Init() tea.Cmd { return nil }

func (c *CommandModel) Activate() {
	c.active = true
	c.input = ""
	c.Done = false
}

func (c *CommandModel) ActivateWithPrefix(prefix string) {
	c.active = true
	c.input = prefix
	c.Done = false
}

func (c CommandModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if !c.active {
		return c, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			c.Done = true
			c.FinalValue = c.input
			c.active = false
			return c, nil
		case "esc":
			c.Done = true
			c.FinalValue = ""
			c.active = false
			return c, nil
		case "backspace":
			if len(c.input) > 0 {
				r := []rune(c.input)
				c.input = string(r[:len(r)-1])
			}
			return c, nil
		case "tab":
			if completed := autocomplete("/" + strings.TrimPrefix(c.input, "/")); completed != "" {
				c.input = strings.TrimPrefix(completed, "/")
				if strings.HasPrefix(c.input, "/") {
					// keep as is
				}
				// autocomplete returns "/cmd " — strip leading /
				c.input = strings.TrimSpace(strings.TrimPrefix(completed, "/")) + " "
			}
			return c, nil
		default:
			if msg.Type == tea.KeyRunes {
				c.input += string(msg.Runes)
			}
			return c, nil
		}
	}
	return c, nil
}

func (c CommandModel) View() string {
	if !c.active {
		return ""
	}
	t := theme.Current()
	style := lipgloss.NewStyle().
		Background(t.BgOverlay).
		Foreground(t.AccentAI).
		Width(c.width).
		Padding(0, 1)

	prompt := ":"
	if strings.HasPrefix(c.input, "/") {
		prompt = ""
	}
	content := prompt + c.input + "▌"

	hint := ""
	partial := strings.TrimPrefix(c.input, "/")
	if partial != "" {
		var matches []string
		for _, cmd := range slashCommands {
			name := strings.TrimPrefix(cmd, "/")
			if strings.HasPrefix(name, partial) {
				matches = append(matches, cmd)
			}
		}
		if len(matches) > 0 {
			hint = "\n" + lipgloss.NewStyle().Foreground(t.TextMuted).Render(strings.Join(matches, "  "))
		}
	}
	_ = fmt.Sprintf
	return style.Render(content + hint)
}
