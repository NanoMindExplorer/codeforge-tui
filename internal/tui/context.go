package tui

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
)

type ContextModel struct {
    width   int
    height  int
    workdir string
    files   []string
    tools   []string
}

func NewContextModel(workdir string) ContextModel {
    return ContextModel{
        workdir: workdir,
        files:   []string{},
        tools:   []string{"read_file", "write_file", "list_dir", "grep_search", "run_command"},
    }
}

func (c ContextModel) Init() tea.Cmd { return nil }

func (c *ContextModel) SetSize(w, h int) {
    c.width = w
    c.height = h
}

func (c ContextModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case ContextUpdateMsg:
        c.files = msg.Files
    }
    return c, nil
}

func (c ContextModel) View() string {
    if c.width < 10 {
        c.width = 10
    }
    if c.height < 5 {
        c.height = 5
    }
    var sb strings.Builder
    sb.WriteString("Context\n")
    sb.WriteString(strings.Repeat("-", max(0, c.width-4)) + "\n\n")

    sb.WriteString("Workdir:\n")
    shortPath := c.workdir
    if len(shortPath) > c.width-6 {
        shortPath = "..." + shortPath[len(shortPath)-(c.width-9):]
    }
    sb.WriteString("  " + shortPath + "\n\n")

    sb.WriteString("Files in context:\n")
    if len(c.files) == 0 {
        sb.WriteString("  (none)\n")
    } else {
        for _, f := range c.files {
            if len(f) > c.width-6 {
                f = "..." + f[len(f)-(c.width-9):]
            }
            sb.WriteString("  " + f + "\n")
        }
    }
    sb.WriteString("\n")

    sb.WriteString("Recent files:\n")
    entries, err := os.ReadDir(c.workdir)
    if err == nil {
        count := 0
        for _, e := range entries {
            if count >= 5 {
                break
            }
            if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
                name := e.Name()
                if len(name) > c.width-6 {
                    name = name[:c.width-9] + "..."
                }
                sb.WriteString("  " + name + "\n")
                count++
            }
        }
        if count == 0 {
            sb.WriteString("  (empty)\n")
        }
    } else {
        sb.WriteString("  (cannot read)\n")
    }
    sb.WriteString("\n")

    sb.WriteString("Tools ready:\n")
    for _, t := range c.tools {
        sb.WriteString(fmt.Sprintf("  + %s\n", t))
    }

    style := lipgloss.NewStyle().
        Width(c.width).
        Height(c.height).
        Foreground(lipgloss.Color("#94A3B8"))

    return style.Render(sb.String())
}

func (c *ContextModel) AddFile(path string) {
    abs, err := filepath.Abs(path)
    if err != nil {
        abs = path
    }
    for _, f := range c.files {
        if f == abs {
            return
        }
    }
    c.files = append(c.files, abs)
}
