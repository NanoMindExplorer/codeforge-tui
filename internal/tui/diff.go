package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/codeforge/tui/internal/ui/diffview"
)

// DiffModel is a rich multi-file diff pane (orchestrator over diffview).
type DiffModel struct {
	width   int
	height  int
	files   []diffview.FileDiff
	idx     int
	pending bool
	raw     string
}

func NewDiffModel() DiffModel {
	return DiffModel{}
}

func (d DiffModel) Init() tea.Cmd { return nil }

func (d *DiffModel) SetSize(w, h int) {
	d.width = w
	d.height = h
}

func (d *DiffModel) SetContent(content string) {
	d.raw = content
	d.files = diffview.Parse(content)
	d.idx = 0
}

func (d *DiffModel) SetPending(p bool) { d.pending = p }

func (d *DiffModel) NextFile() {
	if len(d.files) == 0 {
		return
	}
	d.idx = (d.idx + 1) % len(d.files)
}

func (d *DiffModel) PrevFile() {
	if len(d.files) == 0 {
		return
	}
	d.idx = (d.idx - 1 + len(d.files)) % len(d.files)
}

func (d DiffModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case DiffUpdateMsg:
		d.raw = msg.Content
		d.files = diffview.Parse(msg.Content)
		d.pending = msg.Pending
		d.idx = 0
	case tea.KeyMsg:
		switch msg.String() {
		case "n", "right":
			d.NextFile()
		case "p", "left":
			d.PrevFile()
		}
	}
	return d, nil
}

func (d DiffModel) View() string {
	return diffview.RenderMulti(d.files, d.idx, d.width, d.height, d.pending)
}
