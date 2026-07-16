// Package todos implements Grok-style session task lists (Phase 7).
package todos

import (
	"fmt"
	"strings"
	"sync"
)

// Status of a todo item.
type Status string

const (
	Pending    Status = "pending"
	InProgress Status = "in_progress"
	Completed  Status = "completed"
	Cancelled  Status = "cancelled"
)

// Item is one task.
type Item struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Status  Status `json:"status"`
}

// List is a thread-safe todo list for the session.
type List struct {
	mu    sync.RWMutex
	items []Item
	seq   int
}

// Global is the process-wide list (TUI session).
var Global = &List{}

// Set replaces all items (agent todo_write merge=false).
func (l *List) Set(items []Item) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.items = make([]Item, len(items))
	copy(l.items, items)
	for i := range l.items {
		if l.items[i].ID == "" {
			l.seq++
			l.items[i].ID = fmt.Sprintf("t%d", l.seq)
		}
		if l.items[i].Status == "" {
			l.items[i].Status = Pending
		}
	}
}

// Merge updates by id or appends.
func (l *List) Merge(items []Item) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, it := range items {
		if it.Status == "" {
			it.Status = Pending
		}
		found := false
		for i := range l.items {
			if it.ID != "" && l.items[i].ID == it.ID {
				if it.Content != "" {
					l.items[i].Content = it.Content
				}
				l.items[i].Status = it.Status
				found = true
				break
			}
		}
		if !found {
			if it.ID == "" {
				l.seq++
				it.ID = fmt.Sprintf("t%d", l.seq)
			}
			l.items = append(l.items, it)
		}
	}
}

// Items returns a copy.
func (l *List) Items() []Item {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]Item, len(l.items))
	copy(out, l.items)
	return out
}

// Clear removes all.
func (l *List) Clear() {
	l.mu.Lock()
	l.items = nil
	l.mu.Unlock()
}

// Counts returns completed, total (non-cancelled).
func (l *List) Counts() (done, total int) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	for _, it := range l.items {
		if it.Status == Cancelled {
			continue
		}
		total++
		if it.Status == Completed {
			done++
		}
	}
	return done, total
}

// Badge returns "2/5" or "" if empty.
func (l *List) Badge() string {
	d, t := l.Counts()
	if t == 0 {
		return ""
	}
	return fmt.Sprintf("%d/%d", d, t)
}

// Render formats a multi-line summary for chat.
func (l *List) Render() string {
	items := l.Items()
	if len(items) == 0 {
		return "No todos. Agent can call todo_write, or use /todos add <text>."
	}
	var b strings.Builder
	d, t := l.Counts()
	fmt.Fprintf(&b, "Todos %d/%d\n", d, t)
	for _, it := range items {
		mark := "○"
		switch it.Status {
		case InProgress:
			mark = "◐"
		case Completed:
			mark = "●"
		case Cancelled:
			mark = "✕"
		}
		fmt.Fprintf(&b, "  %s %s  %s\n", mark, it.ID, it.Content)
	}
	return strings.TrimRight(b.String(), "\n")
}

// Add appends a pending item.
func (l *List) Add(content string) Item {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seq++
	it := Item{ID: fmt.Sprintf("t%d", l.seq), Content: content, Status: Pending}
	l.items = append(l.items, it)
	return it
}

// SetStatus updates one item.
func (l *List) SetStatus(id string, st Status) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := range l.items {
		if l.items[i].ID == id {
			l.items[i].Status = st
			return true
		}
	}
	return false
}
