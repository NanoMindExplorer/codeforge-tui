package tui

import (
	"github.com/codeforge/tui/internal/bgtask"
	"github.com/codeforge/tui/internal/personas"
	"github.com/codeforge/tui/internal/sandbox"
	"github.com/codeforge/tui/internal/skills"
	"github.com/codeforge/tui/internal/todos"
)

// AppServices groups process-wide collaborators the TUI reads/writes.
// Q2.4: new TUI code should prefer m.svc over package globals so tests can inject
// isolated instances without racing other packages.
//
// Defaults come from DefaultAppServices(); New() wires them. Callers may replace
// fields before first Update for unit tests.
type AppServices struct {
	Todos    *todos.List
	BgTasks  *bgtask.Manager
	Sandbox  func() *sandbox.Engine
	Skills   func() *skills.Registry
	Personas func() *personas.Registry
}

// DefaultAppServices points at the historical process-wide singletons.
func DefaultAppServices() AppServices {
	return AppServices{
		Todos:    todos.Global,
		BgTasks:  bgtask.Global,
		Sandbox:  sandbox.Global,
		Skills:   skills.Global,
		Personas: personas.Global,
	}
}

// todosList returns the active todo list (never nil if DefaultAppServices used).
func (m *Model) todosList() *todos.List {
	if m.svc.Todos != nil {
		return m.svc.Todos
	}
	return todos.Global
}

func (m *Model) bgTasks() *bgtask.Manager {
	if m.svc.BgTasks != nil {
		return m.svc.BgTasks
	}
	return bgtask.Global
}

func (m *Model) sandboxEng() *sandbox.Engine {
	if m.svc.Sandbox != nil {
		return m.svc.Sandbox()
	}
	return sandbox.Global()
}

func (m *Model) skillsReg() *skills.Registry {
	if m.svc.Skills != nil {
		return m.svc.Skills()
	}
	return skills.Global()
}

func (m *Model) personasReg() *personas.Registry {
	if m.svc.Personas != nil {
		return m.svc.Personas()
	}
	return personas.Global()
}
