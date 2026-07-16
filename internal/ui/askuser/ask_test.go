package askuser

import "testing"

func TestAskUserPick(t *testing.T) {
	m := New()
	m.Open("Which approach?", []string{"A", "B", "C"})
	if !m.Active {
		t.Fatal("expected active")
	}
	if !m.Pick(2) {
		t.Fatal("pick failed")
	}
	if m.Answer != "B" || m.Active || !m.Done {
		t.Fatalf("%+v", m)
	}
}

func TestAskUserDismiss(t *testing.T) {
	m := New()
	m.Open("Q?", nil)
	m.Dismiss()
	if m.Active || m.Answer != "" {
		t.Fatal(m)
	}
}

func TestAskUserView(t *testing.T) {
	m := New()
	m.Open("Pick one", []string{"yes", "no"})
	if m.View() == "" {
		t.Fatal("empty view")
	}
}
