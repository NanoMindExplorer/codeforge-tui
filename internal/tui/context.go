package tui

import (
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/codeforge/tui/internal/theme"
)

// FileEntry is one row in the context pane.
type FileEntry struct {
	Name     string
	GitGlyph string
	Touched  bool // AI recently read/wrote
}

type ContextModel struct {
	width   int
	height  int
	workdir string
	files   []FileEntry
	tools   []string
	filter  string
	cursor  int
}

func NewContextModel(workdir string) ContextModel {
	c := ContextModel{
		workdir: workdir,
		tools:   []string{"read_file", "write_file", "list_dir", "grep_search", "run_command"},
	}
	c.RefreshFiles(nil)
	return c
}

func (c ContextModel) Init() tea.Cmd { return nil }

func (c *ContextModel) SetSize(w, h int) {
	c.width = w
	c.height = h
}

// RefreshFiles rebuilds the listing. gitStatus maps relpath → status char.
func (c *ContextModel) RefreshFiles(gitStatus map[string]string) {
	entries, err := os.ReadDir(c.workdir)
	if err != nil {
		return
	}
	// Preserve touched flags
	touched := map[string]bool{}
	for _, f := range c.files {
		if f.Touched {
			touched[f.Name] = true
		}
	}
	var files []FileEntry
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if e.IsDir() {
			switch name {
			case "node_modules", "vendor", "dist", "build":
				continue
			}
			name = name + "/"
		}
		gs := ""
		if gitStatus != nil {
			if s, ok := gitStatus[strings.TrimSuffix(name, "/")]; ok {
				gs = theme.GitStatusGlyph(s)
			}
		}
		files = append(files, FileEntry{
			Name:     name,
			GitGlyph: gs,
			Touched:  touched[name],
		})
		if len(files) >= 80 {
			break
		}
	}
	c.files = files
}

func (c *ContextModel) MarkTouched(path string) {
	base := filepath.Base(path)
	// also try relative
	rel := path
	if r, err := filepath.Rel(c.workdir, path); err == nil {
		rel = r
	}
	for i, f := range c.files {
		n := strings.TrimSuffix(f.Name, "/")
		if n == base || n == rel || f.Name == rel || strings.HasSuffix(rel, n) {
			c.files[i].Touched = true
			return
		}
	}
	// add if missing
	c.files = append([]FileEntry{{Name: rel, Touched: true}}, c.files...)
}

func (c *ContextModel) AddFile(path string) {
	c.MarkTouched(path)
}

func (c ContextModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ContextUpdateMsg:
		if len(msg.Files) > 0 {
			for _, f := range msg.Files {
				c.MarkTouched(f)
			}
		}
		if msg.Refresh {
			c.RefreshFiles(msg.GitStatus)
		}
	}
	return c, nil
}

func (c ContextModel) View() string {
	t := theme.Current()
	if c.width < 10 {
		c.width = 10
	}
	if c.height < 5 {
		c.height = 5
	}
	var sb strings.Builder
	sb.WriteString(theme.StyleHeader().Render("Files") + "\n")
	sb.WriteString(lipgloss.NewStyle().Foreground(t.BorderDim).Render(strings.Repeat("─", max(0, c.width-4))) + "\n")

	shown := 0
	maxFiles := c.height - 10
	if maxFiles < 3 {
		maxFiles = 3
	}
	for _, f := range c.files {
		if shown >= maxFiles {
			sb.WriteString(lipgloss.NewStyle().Foreground(t.TextMuted).Render("  …") + "\n")
			break
		}
		icon := theme.FileIcon(f.Name)
		name := f.Name
		if len(name) > c.width-8 {
			name = name[:c.width-9] + "…"
		}
		line := icon + " " + name
		if f.GitGlyph != "" && f.GitGlyph != " " {
			line = f.GitGlyph + " " + line
		}
		style := lipgloss.NewStyle().Foreground(t.TextSecondary)
		if f.Touched {
			style = lipgloss.NewStyle().Foreground(t.AccentAgent).Bold(true)
		}
		sb.WriteString(style.Render(" "+line) + "\n")
		shown++
	}
	if len(c.files) == 0 {
		sb.WriteString(lipgloss.NewStyle().Foreground(t.TextMuted).Render("  (empty)") + "\n")
	}

	sb.WriteString("\n")
	sb.WriteString(theme.StyleHeader().Render("Tools") + "\n")
	for _, tool := range c.tools {
		icon := theme.ToolIcon(tool)
		sb.WriteString(lipgloss.NewStyle().Foreground(t.TextMuted).Render("  "+icon+" "+tool) + "\n")
	}

	return lipgloss.NewStyle().
		Width(c.width).
		Height(c.height).
		Foreground(t.TextSecondary).
		Render(sb.String())
}
