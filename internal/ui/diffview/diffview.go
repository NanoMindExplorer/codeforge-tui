// Package diffview renders rich unified diffs with gutters and badges.
package diffview

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/codeforge/tui/internal/theme"
)

// FileDiff is one file's change set for multi-file tabs.
type FileDiff struct {
	Path    string
	Content string // unified diff text
	Add     int
	Del     int
}

// Parse splits multi-file combined diffs (each starts with @@ path @@).
func Parse(combined string) []FileDiff {
	if strings.TrimSpace(combined) == "" {
		return nil
	}
	var files []FileDiff
	var cur *FileDiff
	for _, line := range strings.Split(combined, "\n") {
		if strings.HasPrefix(line, "@@ ") && strings.HasSuffix(strings.TrimSpace(line), "@@") {
			// Header: @@ filename @@
			name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "@@ "), "@@"))
			files = append(files, FileDiff{Path: name})
			cur = &files[len(files)-1]
			continue
		}
		if cur == nil {
			files = append(files, FileDiff{Path: "changes"})
			cur = &files[len(files)-1]
		}
		cur.Content += line + "\n"
		switch {
		case strings.HasPrefix(line, "+ "):
			cur.Add++
		case strings.HasPrefix(line, "- "):
			cur.Del++
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			cur.Add++
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			cur.Del++
		}
	}
	return files
}

// CountStats returns +N -M from a unified diff string.
func CountStats(content string) (add, del int) {
	for _, line := range strings.Split(content, "\n") {
		switch {
		case strings.HasPrefix(line, "+ ") || (strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") && !strings.HasPrefix(line, "@@")):
			if strings.HasPrefix(line, "@@") {
				continue
			}
			add++
		case strings.HasPrefix(line, "- ") || (strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") && !strings.HasPrefix(line, "@@")):
			if strings.HasPrefix(line, "@@") {
				continue
			}
			del++
		}
	}
	return
}

// Render paints a file diff into a fixed box.
func Render(fd FileDiff, width, height int, pending bool) string {
	t := theme.Current()
	if width < 12 {
		width = 12
	}
	if height < 4 {
		height = 4
	}

	add, del := fd.Add, fd.Del
	if add == 0 && del == 0 {
		add, del = CountStats(fd.Content)
	}

	// Header
	name := filepath.Base(fd.Path)
	if name == "" || name == "." {
		name = fd.Path
	}
	badge := lipgloss.NewStyle().Foreground(t.Success).Render(fmt.Sprintf("+%d", add)) +
		" " +
		lipgloss.NewStyle().Foreground(t.Danger).Render(fmt.Sprintf("-%d", del))
	title := theme.StyleHeader().Render("Diff") + "  " + badge
	if pending {
		title += "  " + lipgloss.NewStyle().Foreground(t.Warning).Render("⏳ PENDING")
	}

	var lines []string
	lines = append(lines, title)
	lines = append(lines, lipgloss.NewStyle().Foreground(t.TextMuted).Render(name))
	lines = append(lines, lipgloss.NewStyle().Foreground(t.BorderDim).Render(strings.Repeat("─", max(0, width-4))))

	oldLn, newLn := 0, 0
	bodyBudget := height - 4
	if bodyBudget < 1 {
		bodyBudget = 1
	}
	bodyLines := 0
	for _, raw := range strings.Split(fd.Content, "\n") {
		if bodyLines >= bodyBudget {
			lines = append(lines, lipgloss.NewStyle().Foreground(t.TextMuted).Render("… (truncated)"))
			break
		}
		if raw == "" && bodyLines == 0 {
			continue
		}
		var styled string
		switch {
		case strings.HasPrefix(raw, "+ ") || (strings.HasPrefix(raw, "+") && !strings.HasPrefix(raw, "+++")):
			newLn++
			gutter := lipgloss.NewStyle().Foreground(t.TextDisabled).Render(fmt.Sprintf("%4d ", newLn))
			body := lipgloss.NewStyle().
				Foreground(t.DiffAddFg).
				Background(t.DiffAddBg).
				Render(trimPrefix(raw, "+"))
			styled = gutter + body
		case strings.HasPrefix(raw, "- ") || (strings.HasPrefix(raw, "-") && !strings.HasPrefix(raw, "---")):
			oldLn++
			gutter := lipgloss.NewStyle().Foreground(t.TextDisabled).Render(fmt.Sprintf("%4d ", oldLn))
			body := lipgloss.NewStyle().
				Foreground(t.DiffDelFg).
				Background(t.DiffDelBg).
				Render(trimPrefix(raw, "-"))
			styled = gutter + body
		case strings.HasPrefix(raw, "@@"):
			styled = lipgloss.NewStyle().Foreground(t.AccentAI).Render(raw)
		default:
			// context
			oldLn++
			newLn++
			gutter := lipgloss.NewStyle().Foreground(t.TextDisabled).Render(fmt.Sprintf("%4d ", oldLn))
			body := lipgloss.NewStyle().Foreground(t.DiffCtxFg).Render(strings.TrimPrefix(raw, "  "))
			styled = gutter + body
		}
		// Truncate visual width
		if lipgloss.Width(styled) > width-2 {
			// simple cut of plain part
			plain := raw
			if len(plain) > width-8 {
				plain = plain[:width-8] + "…"
			}
			styled = plain
		}
		lines = append(lines, styled)
		bodyLines++
	}

	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Foreground(t.TextSecondary).
		Render(strings.Join(lines, "\n"))
}

// RenderMulti shows tab header [i/n] filename when multiple files.
func RenderMulti(files []FileDiff, idx, width, height int, pending bool) string {
	if len(files) == 0 {
		t := theme.Current()
		msg := "No changes yet.\n\nWhen the AI edits files,\ndiffs will appear here."
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Foreground(t.TextMuted).
			Render("Diff\n" + strings.Repeat("─", max(0, width-4)) + "\n\n" + msg)
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(files) {
		idx = len(files) - 1
	}
	fd := files[idx]
	if len(files) > 1 {
		tab := fmt.Sprintf("[%d/%d] %s", idx+1, len(files), filepath.Base(fd.Path))
		// prepend tab into content area by reducing height
		inner := Render(fd, width, height-1, pending)
		return lipgloss.NewStyle().Foreground(theme.Current().AccentAgent).Render(tab) + "\n" + inner
	}
	return Render(fd, width, height, pending)
}

func trimPrefix(s, p string) string {
	s = strings.TrimPrefix(s, p)
	return strings.TrimPrefix(s, " ")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
