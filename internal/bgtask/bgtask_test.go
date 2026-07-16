package bgtask

import (
	"testing"
	"time"
)

func TestStartAndCancel(t *testing.T) {
	m := New()
	task, err := m.Start(t.TempDir(), "sleep 30")
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != Running {
		t.Fatal(task.Status)
	}
	if err := m.Cancel(task.ID); err != nil {
		t.Fatal(err)
	}
	// wait for cancel
	for i := 0; i < 50; i++ {
		got, _ := m.Get(task.ID)
		if got.Status == Cancelled || got.Status == Failed || got.Status == Succeeded {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	got, _ := m.Get(task.ID)
	t.Fatalf("still %s", got.Status)
}
