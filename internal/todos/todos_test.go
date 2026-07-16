package todos

import "testing"

func TestListBadge(t *testing.T) {
	l := &List{}
	if l.Badge() != "" {
		t.Fatal("empty")
	}
	l.Add("a")
	l.Add("b")
	l.SetStatus("t1", Completed)
	if l.Badge() != "1/2" {
		t.Fatal(l.Badge())
	}
}

func TestMerge(t *testing.T) {
	l := &List{}
	l.Set([]Item{{ID: "x", Content: "one", Status: Pending}})
	l.Merge([]Item{{ID: "x", Status: Completed}})
	items := l.Items()
	if items[0].Status != Completed || items[0].Content != "one" {
		t.Fatal(items)
	}
}
