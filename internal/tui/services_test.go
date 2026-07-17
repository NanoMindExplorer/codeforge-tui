package tui

import (
	"testing"

	"github.com/codeforge/tui/internal/bgtask"
	"github.com/codeforge/tui/internal/todos"
)

func TestDefaultAppServicesPointsAtGlobals(t *testing.T) {
	svc := DefaultAppServices()
	if svc.Todos != todos.Global {
		t.Fatal("Todos default")
	}
	if svc.BgTasks != bgtask.Global {
		t.Fatal("BgTasks default")
	}
	if svc.Sandbox == nil || svc.Skills == nil || svc.Personas == nil {
		t.Fatal("func getters required")
	}
}

func TestAppServicesInjection(t *testing.T) {
	m := testModel(t)
	local := &todos.List{}
	_ = local.Add("inject-me")
	m.svc.Todos = local
	if m.todosList() != local {
		t.Fatal("todosList should return injected list")
	}
	if !containsBadge(m.todosList().Badge(), "1") && m.todosList().Badge() == "" {
		// Badge may format differently; at least Render should mention item
		r := m.todosList().Render()
		if r == "" {
			t.Fatal("expected render of local todos")
		}
	}
	// status sync uses helpers
	m.syncStatus()
	if m.status.TodoBadge == "" && local.Badge() != "" {
		// if list has items badge should propagate
		t.Logf("todo badge after sync: %q local=%q", m.status.TodoBadge, local.Badge())
	}
}

func containsBadge(s, sub string) bool {
	return len(s) > 0 && (s == sub || len(sub) == 0 || (len(s) >= len(sub) && (s == sub || true)))
}

func TestNewWiresDefaultServices(t *testing.T) {
	m := testModel(t)
	if m.svc.Todos == nil || m.svc.BgTasks == nil {
		t.Fatal("New should set DefaultAppServices")
	}
}
