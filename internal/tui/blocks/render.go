package blocks

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/codeforge/tui/internal/pager"
	"github.com/codeforge/tui/internal/theme"
	"github.com/codeforge/tui/internal/ui/markdown"
	"github.com/muesli/reflow/wordwrap"
)

// View renders the visible viewport including sticky header + scrollbar.
// Phase 9: only materializes lines in the viewport (O(viewport) not O(history)).
func (s *Store) View() string {
	if s.width < 10 || s.height < 1 {
		return ""
	}
	t := theme.Current()
	// sticky takes 1 row when present (pager.toml sticky_headers)
	sticky := ""
	if pager.Global().StickyHeaders() {
		sticky = s.StickyUserTitle()
	}
	vpH := s.height
	if sticky != "" {
		vpH--
	}
	if vpH < 1 {
		vpH = 1
	}

	total := s.totalLines()
	if s.follow {
		if total > vpH {
			s.offset = total - vpH
		} else {
			s.offset = 0
		}
	}
	s.clampOffsetWithH(vpH, total)

	vis := s.viewportLines(s.offset, vpH)
	// pad
	for len(vis) < vpH {
		vis = append(vis, "")
	}

	// scrollbar on right of content (pager.toml scrollbar.enabled)
	var body string
	if pager.Global().ScrollbarEnabled() {
		body = s.withScrollbar(vis, total, vpH)
	} else {
		body = strings.Join(vis, "\n")
	}

	var out strings.Builder
	if sticky != "" {
		st := lipgloss.NewStyle().
			Foreground(t.AccentUser).
			Background(t.BgElevated).
			Width(s.width).
			Render("┃ you · " + sticky)
		out.WriteString(st)
		out.WriteByte('\n')
	}
	out.WriteString(body)

	// follow indicator
	if !s.follow && total > vpH {
		hint := lipgloss.NewStyle().Foreground(t.TextMuted).Render(" ↓ follow off · G to resume")
		_ = hint
	}
	return out.String()
}

func (s *Store) clampOffsetWithH(h, total int) {
	maxOff := total - h
	if maxOff < 0 {
		maxOff = 0
	}
	if s.offset > maxOff {
		s.offset = maxOff
	}
	if s.offset < 0 {
		s.offset = 0
	}
}

func (s *Store) withScrollbar(lines []string, total, vpH int) string {
	t := theme.Current()
	contentW := s.width - 1
	if contentW < 8 {
		contentW = s.width
		var b strings.Builder
		for i, ln := range lines {
			if i > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(ln)
		}
		return b.String()
	}
	// thumb position
	thumbAt := 0
	if total > vpH && vpH > 0 {
		thumbAt = s.offset * (vpH - 1) / (total - vpH)
		if thumbAt < 0 {
			thumbAt = 0
		}
		if thumbAt >= vpH {
			thumbAt = vpH - 1
		}
	}
	var b strings.Builder
	for i, ln := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		// pad/truncate visual — lipgloss width
		plain := ln
		if lipgloss.Width(plain) > contentW {
			// rough cut
			r := []rune(stripANSIApprox(plain))
			if len(r) > contentW-1 {
				plain = string(r[:contentW-1]) + "…"
			}
		}
		ch := "│"
		style := lipgloss.NewStyle().Foreground(t.BorderDim)
		if total > vpH && i == thumbAt {
			ch = "█"
			style = lipgloss.NewStyle().Foreground(t.AccentUser)
		} else if total > vpH {
			ch = "│"
		} else {
			ch = " "
		}
		b.WriteString(plain)
		// pad to contentW
		pad := contentW - lipgloss.Width(plain)
		if pad > 0 {
			b.WriteString(strings.Repeat(" ", pad))
		}
		b.WriteString(style.Render(ch))
	}
	return b.String()
}

// stripANSIApprox is a minimal width helper for truncation (not full ANSI parse).
func stripANSIApprox(s string) string {
	// content already mostly styled per-line; for safety return s
	return s
}

func (s *Store) rebuildLayout() {
	n := len(s.blocks)
	s.heights = make([]int, n)
	s.lineStarts = make([]int, n)
	total := 0
	for i := 0; i < n; i++ {
		s.lineStarts[i] = total
		h := len(s.renderBlockLines(i))
		s.heights[i] = h
		total += h
	}
	s.cachedTotal = total
	s.layoutDirty = false
}

func (s *Store) ensureLayout() {
	if s.layoutDirty || len(s.heights) != len(s.blocks) {
		s.rebuildLayout()
	}
}

func (s *Store) totalLines() int {
	s.ensureLayout()
	return s.cachedTotal
}

func (s *Store) blockHeight(i int) int {
	s.ensureLayout()
	if i < 0 || i >= len(s.heights) {
		return 0
	}
	return s.heights[i]
}

func (s *Store) blockLineSpan(i int) (start, end int) {
	s.ensureLayout()
	if i < 0 || i >= len(s.lineStarts) {
		return 0, 0
	}
	start = s.lineStarts[i]
	end = start + s.heights[i]
	return
}

// viewportLines returns only the lines covering [offset, offset+vpH).
func (s *Store) viewportLines(offset, vpH int) []string {
	s.ensureLayout()
	if vpH <= 0 || s.cachedTotal == 0 {
		return nil
	}
	end := offset + vpH
	if end > s.cachedTotal {
		end = s.cachedTotal
	}
	if offset < 0 {
		offset = 0
	}
	if offset >= end {
		return nil
	}
	// find first block that intersects
	var out []string
	for i := range s.blocks {
		bs, be := s.lineStarts[i], s.lineStarts[i]+s.heights[i]
		if be <= offset {
			continue
		}
		if bs >= end {
			break
		}
		lines := s.renderBlockLines(i)
		// slice intersection
		for li, ln := range lines {
			abs := bs + li
			if abs >= offset && abs < end {
				out = append(out, ln)
			}
		}
	}
	return out
}

// flattenLines still available for tests/debug (full materialization).
func (s *Store) flattenLines() []string {
	var all []string
	for i := range s.blocks {
		all = append(all, s.renderBlockLines(i)...)
	}
	return all
}

func (s *Store) renderBlockLines(i int) []string {
	if i < 0 || i >= len(s.blocks) {
		return nil
	}
	b := s.blocks[i]
	t := theme.Current()
	selected := s.showSelection && i == s.selected
	innerW := s.width - 4 // accent + scrollbar + pad
	if innerW < 12 {
		innerW = 12
	}

	accent := t.AccentSystem
	switch b.Kind {
	case KindUser:
		accent = t.AccentUser
	case KindAssistant, KindThinking:
		accent = t.AccentAssistant
	case KindToolCall, KindToolResult, KindDiff:
		accent = t.AccentTool
	}
	pfx := theme.BlockPrefix(accent)
	if selected {
		pfx = lipgloss.NewStyle().Foreground(t.AccentFocus).Bold(true).Render("┃ ")
	}

	var lines []string
	header := s.blockHeader(b)
	if selected {
		header = lipgloss.NewStyle().Background(t.BgElevated).Foreground(t.AccentFocus).Render(header)
	} else {
		header = lipgloss.NewStyle().Foreground(headerColor(b, t)).Render(header)
	}
	lines = append(lines, pfx+header)

	if b.Collapsed && b.Foldable {
		// collapsed: one summary line only (already header)
		if b.Kind == KindToolResult || b.Kind == KindDiff {
			sum := strings.TrimSpace(strings.ReplaceAll(b.Body, "\n", " "))
			if len(sum) > 50 {
				sum = sum[:50] + "…"
			}
			if sum != "" {
				lines = append(lines, pfx+lipgloss.NewStyle().Foreground(t.TextMuted).Render("  "+sum))
			}
		}
		lines = append(lines, "")
		return lines
	}

	// expanded body
	bodyLines := s.bodyLines(b, innerW)
	for _, ln := range bodyLines {
		if b.Kind == KindUser {
			ln = lipgloss.NewStyle().Background(t.BgLight).Foreground(t.TextPrimary).Width(innerW).Render(ln)
		}
		lines = append(lines, pfx+ln)
	}
	lines = append(lines, "") // gap after block
	return lines
}

func headerColor(b Block, t theme.Tokens) lipgloss.Color {
	switch b.Kind {
	case KindUser:
		return t.AccentUser
	case KindAssistant:
		return t.AccentAssistant
	case KindToolCall:
		return t.AccentTool
	case KindToolResult:
		if strings.HasPrefix(b.Title, "✗") {
			return t.Danger
		}
		return t.Success
	case KindDiff:
		return t.AccentTool
	case KindThinking:
		return t.AccentThinking
	default:
		return t.TextMuted
	}
}

func (s *Store) blockHeader(b Block) string {
	pg := pager.Global()
	fold := ""
	if b.Foldable {
		if b.Collapsed {
			ind := pg.ExpandableChar()
			if ind == "" {
				ind = "›"
			}
			fold = ind + " "
		} else {
			fold = "⌄ "
		}
	}
	switch b.Kind {
	case KindUser:
		return fold + "you"
	case KindDiff:
		m := b.Meta
		if m != "" {
			m = " " + m
		}
		return fold + "▤ " + b.Title + m
	case KindThinking:
		if !pg.ShowThinking() {
			return fold + "💭"
		}
		stream := ""
		if b.Streaming {
			stream = " …"
		}
		label := "thinking"
		if !pg.ThinkingHeader() {
			label = ""
		}
		if label == "" {
			return fold + "💭" + stream
		}
		return fold + "💭 " + label + stream
	case KindAssistant:
		stream := ""
		if b.Streaming {
			stream = " …"
		}
		return fold + "assistant" + stream
	case KindSystem:
		return fold + "system"
	case KindToolCall:
		icon := pg.ToolBulletChar()
		if icon == "" {
			icon = theme.ToolIcon(b.Title)
		}
		return fold + icon + " " + b.Title
	case KindToolResult:
		return fold + b.Title
	default:
		return fold + b.Title
	}
}

func (s *Store) bodyLines(b Block, width int) []string {
	body := b.Body
	if body == "" {
		return nil
	}
	pg := pager.Global()
	// hide thinking body entirely when disabled
	if b.Kind == KindThinking && !pg.ShowThinking() {
		return nil
	}
	// cap thinking width
	if b.Kind == KindThinking {
		mw := pg.MaxThoughtsWidth()
		if mw > 0 && width > mw {
			width = mw
		}
	}
	var lines []string
	switch b.Kind {
	case KindAssistant, KindThinking:
		out := markdown.Render(body, width)
		lines = strings.Split(out, "\n")
		// collapsed/truncated thinking lines when not streaming
		if b.Kind == KindThinking && !b.Streaming && b.Collapsed {
			n := pg.ThinkingTruncateLines()
			if n > 0 && len(lines) > n {
				lines = append(lines[:n], "…")
			}
		}
	case KindToolCall:
		// args preview
		preview := strings.TrimSpace(body)
		if len(preview) > 400 {
			preview = preview[:400] + "…"
		}
		wrapped := wordwrap.String(preview, width)
		for _, ln := range strings.Split(wrapped, "\n") {
			lines = append(lines, lipgloss.NewStyle().Foreground(theme.Current().TextMuted).Render(ln))
		}
	case KindToolResult:
		preview := body
		if len(preview) > 2000 {
			preview = preview[:2000] + "\n…"
		}
		wrapped := wordwrap.String(preview, width)
		lines = strings.Split(wrapped, "\n")
	case KindDiff:
		lines = renderDiffBody(body, width)
	case KindUser:
		wrapped := wordwrap.String(body, width)
		lines = strings.Split(wrapped, "\n")
	default:
		wrapped := wordwrap.String(body, width)
		lines = strings.Split(wrapped, "\n")
	}
	// Phase 9: cap painted lines; full body still in Block for viewer/copy
	if len(lines) > MaxBodyLines {
		t := theme.Current()
		trunc := lipgloss.NewStyle().Foreground(t.TextMuted).Italic(true).
			Render(fmt.Sprintf("… +%d lines (Enter to expand · y to copy)", len(lines)-MaxBodyLines+1))
		lines = append(lines[:MaxBodyLines-1], trunc)
	}
	return lines
}

func renderDiffBody(diffText string, width int) []string {
	t := theme.Current()
	var lines []string
	for _, raw := range strings.Split(diffText, "\n") {
		if len(lines) > 80 {
			lines = append(lines, lipgloss.NewStyle().Foreground(t.TextMuted).Render("…"))
			break
		}
		var styled string
		switch {
		case strings.HasPrefix(raw, "+") && !strings.HasPrefix(raw, "+++"):
			styled = lipgloss.NewStyle().Foreground(t.DiffAddFg).Background(t.DiffAddBg).Width(width).Render(raw)
		case strings.HasPrefix(raw, "-") && !strings.HasPrefix(raw, "---"):
			styled = lipgloss.NewStyle().Foreground(t.DiffDelFg).Background(t.DiffDelBg).Width(width).Render(raw)
		case strings.HasPrefix(raw, "@@"):
			styled = lipgloss.NewStyle().Foreground(t.AccentAssistant).Render(raw)
		default:
			styled = lipgloss.NewStyle().Foreground(t.DiffCtxFg).Render(raw)
		}
		lines = append(lines, styled)
	}
	return lines
}

// DebugString returns a plain dump for tests (no ANSI concerns for structure).
func (s *Store) DebugString() string {
	var b strings.Builder
	for i, bl := range s.blocks {
		c := " "
		if bl.Collapsed {
			c = "›"
		}
		fmt.Fprintf(&b, "%d %s %s %q fold=%v\n", i, c, kindName(bl.Kind), bl.Title, bl.Foldable)
	}
	return b.String()
}

// kindName for DebugString — defined in block.go
