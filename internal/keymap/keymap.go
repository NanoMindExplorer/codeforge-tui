// Package keymap centralizes all key bindings and help text.
package keymap

import "github.com/charmbracelet/bubbles/key"

// Map holds all CodeForge key bindings.
type Map struct {
	Insert       key.Binding
	ActInsert    key.Binding
	Command      key.Binding
	Slash        key.Binding
	Quit         key.Binding
	Help         key.Binding
	Palette      key.Binding
	ToggleMode   key.Binding // Plan/Act
	PaneChat     key.Binding
	PaneDiff     key.Binding
	PaneContext  key.Binding
	NextPane     key.Binding
	PrevPane     key.Binding
	ScrollDown   key.Binding
	ScrollUp     key.Binding
	ScrollTop    key.Binding
	ScrollBottom key.Binding
	Esc          key.Binding
	FileMention  key.Binding
}

// Default returns the default key map.
func Default() Map {
	return Map{
		Insert: key.NewBinding(
			key.WithKeys("i"),
			key.WithHelp("i", "insert mode"),
		),
		ActInsert: key.NewBinding(
			key.WithKeys("I"),
			key.WithHelp("I", "insert /act"),
		),
		Command: key.NewBinding(
			key.WithKeys(":"),
			key.WithHelp(":", "command"),
		),
		Slash: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "slash cmd"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Palette: key.NewBinding(
			key.WithKeys("ctrl+k"),
			key.WithHelp("⌘K", "palette"),
		),
		ToggleMode: key.NewBinding(
			key.WithKeys("P"), // Shift+P
			key.WithHelp("Shift+P", "plan/act"),
		),
		PaneChat: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1", "chat pane"),
		),
		PaneDiff: key.NewBinding(
			key.WithKeys("2"),
			key.WithHelp("2", "diff pane"),
		),
		PaneContext: key.NewBinding(
			key.WithKeys("3"),
			key.WithHelp("3", "context pane"),
		),
		NextPane: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("Tab", "next pane"),
		),
		PrevPane: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("S-Tab", "prev pane"),
		),
		ScrollDown: key.NewBinding(
			key.WithKeys("j", "down"),
			key.WithHelp("j", "scroll down"),
		),
		ScrollUp: key.NewBinding(
			key.WithKeys("k", "up"),
			key.WithHelp("k", "scroll up"),
		),
		ScrollTop: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("g", "top"),
		),
		ScrollBottom: key.NewBinding(
			key.WithKeys("G"),
			key.WithHelp("G", "bottom"),
		),
		Esc: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("Esc", "normal mode"),
		),
		FileMention: key.NewBinding(
			key.WithKeys("@"),
			key.WithHelp("@", "mention file"),
		),
	}
}

// ShortHelp returns bottom-bar hint text.
func (m Map) ShortHelp() string {
	return "i:chat  I:/act  ⌘K:palette  @:file  Tab:pane  Shift+P:plan/act  q:quit"
}

// FullHelp returns multi-line help for /help.
func FullHelp() string {
	return `MODES
  i          INSERT mode (ketik pesan atau /command)
  I          INSERT mode dengan /act
  /          INSERT mode dengan /
  Esc        NORMAL mode
  :          Command line
  Ctrl+K     Command palette (fuzzy)

NAVIGATION
  1 / 2 / 3  Chat / Diff / Context
  Tab        Pane berikutnya
  j / k      Scroll
  g / G      Atas / bawah

WORKFLOW
  Shift+P    Toggle Plan ↔ Act mode
  @          Mention file ke prompt
  /sessions  Resume sesi tersimpan
  /undo      Batalkan write terakhir
  /mode      Tampilkan / ganti Plan|Act

AGENT
  /act <task>   Agent tool-calling
  /read /ls /grep /run /explain /fix

GIT & LAINNYA
  /status /commit /provider /model /cost /clear /quit`
}
