package review

import (
	"strings"
	"testing"

	"github.com/codeforge/tui/internal/theme"
	"github.com/codeforge/tui/internal/tool"
)

// Q5.4: BUILD review overlay accept/reject/toggle unit coverage.
func TestReviewOverlayAcceptReject(t *testing.T) {
	theme.Set(theme.Aurora())
	theme.SetMotion(false)
	m := New()
	m.Width = 80
	m.Height = 24
	m.Open([]tool.PendingPatch{
		{RelPath: "a.go", Diff: "@@ -1 +1 @@\n-old\n+new\n", Accepted: false},
		{RelPath: "b.go", Diff: "@@ -1 +1 @@\n-x\n+y\n", Accepted: false},
	})
	if !m.Active || len(m.Patches) != 2 {
		t.Fatal("open")
	}
	// Open forces all accepted by default
	if !m.Patches[0].Accepted || !m.Patches[1].Accepted {
		t.Fatal("default accepted")
	}
	m.Toggle()
	if m.Patches[0].Accepted {
		t.Fatal("toggled off")
	}
	m.Move(1)
	m.RejectAll()
	if m.Patches[0].Accepted || m.Patches[1].Accepted {
		t.Fatal("reject all")
	}
	m.AcceptAll()
	acc := m.Accepted()
	if len(acc) != 2 {
		t.Fatal(len(acc))
	}
	view := m.View()
	if !strings.Contains(view, "a.go") && !strings.Contains(strings.ToLower(view), "review") {
		// view should render something meaningful
		if view == "" {
			t.Fatal("empty view")
		}
	}
	m.Apply()
	if !m.Done || m.Action != "apply" || m.Active {
		t.Fatalf("apply: done=%v action=%s active=%v", m.Done, m.Action, m.Active)
	}

	// reopen and cancel
	m.Open([]tool.PendingPatch{{RelPath: "c.go", Diff: "d", Accepted: true}})
	m.Cancel()
	if m.Action != "cancel" {
		t.Fatal(m.Action)
	}
}

func TestReviewRejectAction(t *testing.T) {
	m := New()
	m.Open([]tool.PendingPatch{{RelPath: "x", Diff: "d"}})
	m.Reject()
	if m.Action != "reject" || !m.Done {
		t.Fatal(m.Action)
	}
}
