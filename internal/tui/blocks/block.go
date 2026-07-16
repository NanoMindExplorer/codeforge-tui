// Package blocks implements Grok-style scrollback: foldable conversation blocks
// with selection, follow-tail, sticky user headers, and optional scrollbar.
// Phase 1 of GROK_PARITY_ROADMAP.
package blocks

import (
	"fmt"
	"strings"
	"sync/atomic"
)

// Kind classifies a scrollback block.
type Kind int

const (
	KindUser Kind = iota
	KindAssistant
	KindSystem
	KindToolCall
	KindToolResult
	KindDiff
	KindThinking
)

// Block is one atomic unit in the scrollback (Grok pager entry).
type Block struct {
	ID        string
	TurnID    string // groups user + following assistant/tools in one turn
	Kind      Kind
	Title     string // tool name, "you", "assistant", …
	Body      string // raw text (markdown for assistant)
	Collapsed bool
	Foldable  bool
	Streaming bool // still receiving chunks
	Meta      string // e.g. "+12 -3" for diffs
}

// Store holds ordered blocks and scroll/selection state.
type Store struct {
	blocks   []Block
	selected int // index into blocks; -1 = none
	// scroll
	offset int  // first visible *line* in the flattened render
	follow bool // auto stick to bottom (Grok follow-tail)
	// layout
	width  int
	height int // viewport height in rows
	// turn tracking
	turnSeq uint64
	turnID  string
	// streaming assistant block index (-1 if none)
	streamAsst int
	// id generator
	idSeq uint64
	// vim-like selection when scrollback focused
	showSelection bool
}

// NewStore creates an empty follow-tail store.
func NewStore() *Store {
	return &Store{
		selected:   -1,
		follow:     true,
		streamAsst: -1,
	}
}

func (s *Store) nextID(prefix string) string {
	n := atomic.AddUint64(&s.idSeq, 1)
	return fmt.Sprintf("%s-%d", prefix, n)
}

// BeginTurn starts a new user turn id.
func (s *Store) BeginTurn() string {
	s.turnSeq++
	s.turnID = fmt.Sprintf("turn-%d", s.turnSeq)
	return s.turnID
}

// CurrentTurn returns the active turn id (may be empty).
func (s *Store) CurrentTurn() string { return s.turnID }

// Len returns block count.
func (s *Store) Len() int { return len(s.blocks) }

// Blocks returns a copy of blocks (for tests).
func (s *Store) Blocks() []Block {
	out := make([]Block, len(s.blocks))
	copy(out, s.blocks)
	return out
}

// SelectedIndex returns selected block index or -1.
func (s *Store) SelectedIndex() int { return s.selected }

// Following reports follow-tail state.
func (s *Store) Following() bool { return s.follow }

// SetSize sets render width and viewport height.
func (s *Store) SetSize(w, h int) {
	if w < 20 {
		w = 20
	}
	if h < 3 {
		h = 3
	}
	s.width = w
	s.height = h
	s.clampOffset()
}

// SetShowSelection enables highlight of selected block (scrollback focused).
func (s *Store) SetShowSelection(on bool) {
	s.showSelection = on
	if on && s.selected < 0 && len(s.blocks) > 0 {
		s.selected = len(s.blocks) - 1
	}
}

// --- Mutations ---

// AddUser appends a user prompt block and begins a turn.
func (s *Store) AddUser(text string) {
	tid := s.BeginTurn()
	s.streamAsst = -1
	s.append(Block{
		ID: s.nextID("user"), TurnID: tid, Kind: KindUser,
		Title: "you", Body: text, Foldable: true, Collapsed: false,
	})
}

// AddSystem appends a non-foldable (or lightly foldable) system line group.
func (s *Store) AddSystem(text string) {
	s.streamAsst = -1
	s.append(Block{
		ID: s.nextID("sys"), TurnID: s.turnID, Kind: KindSystem,
		Title: "system", Body: text, Foldable: true, Collapsed: false,
	})
}

// StartAssistant opens a streaming assistant block (or reuses open one).
func (s *Store) StartAssistant() {
	if s.streamAsst >= 0 && s.streamAsst < len(s.blocks) {
		return
	}
	s.append(Block{
		ID: s.nextID("asst"), TurnID: s.turnID, Kind: KindAssistant,
		Title: "assistant", Body: "", Foldable: true, Collapsed: false, Streaming: true,
	})
	s.streamAsst = len(s.blocks) - 1
}

// AppendAssistantChunk adds text to the streaming assistant block.
func (s *Store) AppendAssistantChunk(text string) {
	if text == "" {
		return
	}
	s.StartAssistant()
	b := &s.blocks[s.streamAsst]
	b.Body += text
	if s.follow {
		s.scrollToEnd()
	}
}

// SealAssistant finishes the streaming assistant block.
func (s *Store) SealAssistant() {
	if s.streamAsst >= 0 && s.streamAsst < len(s.blocks) {
		s.blocks[s.streamAsst].Streaming = false
		// drop empty assistant
		if strings.TrimSpace(s.blocks[s.streamAsst].Body) == "" {
			s.blocks = append(s.blocks[:s.streamAsst], s.blocks[s.streamAsst+1:]...)
		}
	}
	s.streamAsst = -1
}

// AddToolCall records a tool invocation (foldable; body = args preview).
func (s *Store) AddToolCall(name, input string) {
	s.SealAssistant()
	s.append(Block{
		ID: s.nextID("tool"), TurnID: s.turnID, Kind: KindToolCall,
		Title: name, Body: input, Foldable: true, Collapsed: false,
	})
}

// AddToolResult records tool output (foldable; often collapsed when long).
func (s *Store) AddToolResult(name, output string, ok bool) {
	title := "✓ " + name
	if !ok {
		title = "✗ " + name
	}
	collapsed := len(output) > 200
	s.append(Block{
		ID: s.nextID("tres"), TurnID: s.turnID, Kind: KindToolResult,
		Title: title, Body: output, Foldable: true, Collapsed: collapsed,
	})
}

// AddToolProgress appends a short progress system-style block (or updates last progress).
func (s *Store) AddToolProgress(text string) {
	if text == "" {
		return
	}
	// attach to last tool call body if present
	if n := len(s.blocks); n > 0 && s.blocks[n-1].Kind == KindToolCall {
		s.blocks[n-1].Body += "\n⋯ " + text
		if s.follow {
			s.scrollToEnd()
		}
		return
	}
	s.AddSystem("⋯ " + text)
}

// AddDiff appends a foldable diff block (inline under tool writes).
func (s *Store) AddDiff(title, diffText, meta string) {
	if meta == "" {
		meta = DiffMeta(diffText)
	}
	// default collapse very large diffs
	collapsed := strings.Count(diffText, "\n") > 40
	s.append(Block{
		ID: s.nextID("diff"), TurnID: s.turnID, Kind: KindDiff,
		Title: title, Body: diffText, Meta: meta, Foldable: true, Collapsed: collapsed,
	})
}

// AddThinking opens a reasoning block (synthetic "planning…" or provider text).
func (s *Store) AddThinking(text string) {
	// if last is streaming thinking, append
	if n := len(s.blocks); n > 0 && s.blocks[n-1].Kind == KindThinking && s.blocks[n-1].Streaming {
		if text != "" {
			s.blocks[n-1].Body += text
		}
		if s.follow {
			s.scrollToEnd()
		}
		return
	}
	s.append(Block{
		ID: s.nextID("think"), TurnID: s.turnID, Kind: KindThinking,
		Title: "thinking", Body: text, Foldable: true, Collapsed: false, Streaming: true,
	})
}

// SealThinking marks thinking block complete (and collapses if long).
func (s *Store) SealThinking() {
	for i := len(s.blocks) - 1; i >= 0; i-- {
		if s.blocks[i].Kind == KindThinking && s.blocks[i].Streaming {
			s.blocks[i].Streaming = false
			if len(s.blocks[i].Body) > 200 {
				s.blocks[i].Collapsed = true
			}
			return
		}
	}
}

// Selected returns the selected block or nil.
func (s *Store) Selected() *Block {
	if s.selected < 0 || s.selected >= len(s.blocks) {
		return nil
	}
	b := s.blocks[s.selected]
	return &b
}

// SelectedBody returns body of selected block for copy/viewer.
func (s *Store) SelectedBody() string {
	b := s.Selected()
	if b == nil {
		return ""
	}
	return b.Body
}

// SelectedMeta returns a one-line metadata summary for copy.
func (s *Store) SelectedMeta() string {
	b := s.Selected()
	if b == nil {
		return ""
	}
	return fmt.Sprintf("kind=%s id=%s turn=%s title=%q meta=%q bytes=%d",
		kindName(b.Kind), b.ID, b.TurnID, b.Title, b.Meta, len(b.Body))
}

func kindName(k Kind) string {
	switch k {
	case KindUser:
		return "user"
	case KindAssistant:
		return "assistant"
	case KindSystem:
		return "system"
	case KindToolCall:
		return "tool_call"
	case KindToolResult:
		return "tool_result"
	case KindDiff:
		return "diff"
	case KindThinking:
		return "thinking"
	default:
		return "?"
	}
}

// DiffMeta computes +N/-M from a unified diff.
func DiffMeta(diffText string) string {
	add, del := 0, 0
	for _, line := range strings.Split(diffText, "\n") {
		if len(line) == 0 {
			continue
		}
		if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "@@") {
			continue
		}
		switch line[0] {
		case '+':
			add++
		case '-':
			del++
		}
	}
	if add == 0 && del == 0 {
		return ""
	}
	return fmt.Sprintf("+%d -%d", add, del)
}

func (s *Store) append(b Block) {
	s.blocks = append(s.blocks, b)
	if s.follow {
		s.selected = len(s.blocks) - 1
		s.scrollToEnd()
	}
}

// Clear resets all blocks.
func (s *Store) Clear() {
	s.blocks = nil
	s.selected = -1
	s.offset = 0
	s.follow = true
	s.streamAsst = -1
	s.turnID = ""
}

// --- Navigation ---

// SelectNext moves selection down.
func (s *Store) SelectNext() {
	if len(s.blocks) == 0 {
		return
	}
	s.showSelection = true
	if s.selected < 0 {
		s.selected = 0
	} else if s.selected < len(s.blocks)-1 {
		s.selected++
	}
	s.ensureVisible(s.selected)
	s.follow = false
}

// SelectPrev moves selection up.
func (s *Store) SelectPrev() {
	if len(s.blocks) == 0 {
		return
	}
	s.showSelection = true
	if s.selected < 0 {
		s.selected = len(s.blocks) - 1
	} else if s.selected > 0 {
		s.selected--
	}
	s.ensureVisible(s.selected)
	s.follow = false
}

// ToggleCollapse toggles the selected (or last) foldable block.
func (s *Store) ToggleCollapse() {
	idx := s.selected
	if idx < 0 || idx >= len(s.blocks) {
		idx = len(s.blocks) - 1
	}
	if idx < 0 {
		return
	}
	if s.blocks[idx].Foldable {
		s.blocks[idx].Collapsed = !s.blocks[idx].Collapsed
	}
}

// ExpandAll expands every foldable block.
func (s *Store) ExpandAll() {
	for i := range s.blocks {
		if s.blocks[i].Foldable {
			s.blocks[i].Collapsed = false
		}
	}
}

// CollapseAll collapses every foldable block.
func (s *Store) CollapseAll() {
	for i := range s.blocks {
		if s.blocks[i].Foldable {
			s.blocks[i].Collapsed = true
		}
	}
}

// ToggleExpandAll expands if any collapsed, else collapses all.
func (s *Store) ToggleExpandAll() {
	anyCollapsed := false
	for _, b := range s.blocks {
		if b.Foldable && b.Collapsed {
			anyCollapsed = true
			break
		}
	}
	if anyCollapsed {
		s.ExpandAll()
	} else {
		s.CollapseAll()
	}
}

// LineDown scrolls one line without changing selection (Grok Ctrl+J).
func (s *Store) LineDown() {
	s.follow = false
	s.offset++
	s.clampOffset()
}

// LineUp scrolls one line up.
func (s *Store) LineUp() {
	s.follow = false
	if s.offset > 0 {
		s.offset--
	}
}

// PageDown scrolls half/full page.
func (s *Store) PageDown(half bool) {
	s.follow = false
	step := s.height
	if half {
		step = s.height / 2
	}
	if step < 1 {
		step = 1
	}
	s.offset += step
	s.clampOffset()
}

// PageUp scrolls up.
func (s *Store) PageUp(half bool) {
	s.follow = false
	step := s.height
	if half {
		step = s.height / 2
	}
	if step < 1 {
		step = 1
	}
	s.offset -= step
	if s.offset < 0 {
		s.offset = 0
	}
}

// GotoTop jumps to start.
func (s *Store) GotoTop() {
	s.follow = false
	s.offset = 0
	if len(s.blocks) > 0 {
		s.selected = 0
	}
}

// GotoBottom enables follow-tail (Grok G).
func (s *Store) GotoBottom() {
	s.follow = true
	if len(s.blocks) > 0 {
		s.selected = len(s.blocks) - 1
	}
	s.scrollToEnd()
}

func (s *Store) scrollToEnd() {
	total := s.totalLines()
	if total <= s.height {
		s.offset = 0
		return
	}
	s.offset = total - s.height
}

func (s *Store) clampOffset() {
	total := s.totalLines()
	maxOff := total - s.height
	if maxOff < 0 {
		maxOff = 0
	}
	if s.offset > maxOff {
		s.offset = maxOff
	}
	if s.offset < 0 {
		s.offset = 0
	}
	if s.follow {
		s.scrollToEnd()
	}
}

func (s *Store) ensureVisible(blockIdx int) {
	if blockIdx < 0 || blockIdx >= len(s.blocks) {
		return
	}
	start, _ := s.blockLineSpan(blockIdx)
	// if block starts above viewport, scroll up
	if start < s.offset {
		s.offset = start
	}
	// if below, scroll so block end fits
	_, end := s.blockLineSpan(blockIdx)
	if end > s.offset+s.height {
		s.offset = end - s.height
	}
	s.clampOffset()
}

// StickyUserTitle returns the nearest user prompt above the viewport, if any.
func (s *Store) StickyUserTitle() string {
	if s.follow || len(s.blocks) == 0 {
		return ""
	}
	// find first visible block
	line := 0
	firstVis := -1
	for i := range s.blocks {
		n := s.blockHeight(i)
		if line+n > s.offset {
			firstVis = i
			break
		}
		line += n
	}
	if firstVis <= 0 {
		return ""
	}
	// walk back for user block that scrolled off
	for i := firstVis - 1; i >= 0; i-- {
		if s.blocks[i].Kind == KindUser {
			body := strings.TrimSpace(s.blocks[i].Body)
			body = strings.ReplaceAll(body, "\n", " ")
			if len(body) > 60 {
				body = body[:60] + "…"
			}
			return body
		}
	}
	return ""
}
