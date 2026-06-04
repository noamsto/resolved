package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/noamsto/resolved/internal/model"
)

func fixture() []model.Finding {
	return []model.Finding{
		{Reference: model.Reference{File: "z.go", Line: 9, Owner: "o", Repo: "r", Number: 3, Type: model.TypeIssue}, Status: model.Status{Title: "open one"}, Tier: model.TierOpen},
		{Reference: model.Reference{File: "a.go", Line: 2, Owner: "o", Repo: "r", Number: 1, Type: model.TypeIssue}, Status: model.Status{Title: "stale one"}, Tier: model.TierStale},
		{Reference: model.Reference{File: "m.go", Line: 5, Owner: "o", Repo: "r", Number: 2, Type: model.TypePR}, Status: model.Status{Title: "closed one"}, Tier: model.TierClosed},
	}
}

func TestNewSortsStaleFirst(t *testing.T) {
	m := New(fixture(), Deps{})
	if m.findings[0].Tier != model.TierStale {
		t.Fatalf("first finding tier = %v, want stale", m.findings[0].Tier)
	}
	if m.findings[1].Tier != model.TierClosed {
		t.Fatalf("second finding tier = %v, want closed", m.findings[1].Tier)
	}
}

func TestViewShowsLocationsAndCursor(t *testing.T) {
	m := New(fixture(), Deps{})
	out := m.View()
	if !strings.Contains(out, "a.go:2") {
		t.Fatalf("view missing a.go:2:\n%s", out)
	}
	if !strings.Contains(out, "> ") {
		t.Fatalf("view missing cursor marker:\n%s", out)
	}
}

func TestIssueURL(t *testing.T) {
	issue := model.Finding{Reference: model.Reference{Owner: "o", Repo: "r", Number: 7, Type: model.TypeIssue}}
	if got := issueURL(issue); got != "https://github.com/o/r/issues/7" {
		t.Fatalf("issue url = %q", got)
	}
	pr := model.Finding{Reference: model.Reference{Owner: "o", Repo: "r", Number: 8, Type: model.TypePR}}
	if got := issueURL(pr); got != "https://github.com/o/r/pull/8" {
		t.Fatalf("pr url = %q", got)
	}
}

func TestUpdateNavigation(t *testing.T) {
	m := New(fixture(), Deps{})

	down := tea.KeyMsg{Type: tea.KeyDown}
	nm, _ := m.Update(down)
	m = nm.(Model)
	if m.cursor != 1 {
		t.Fatalf("after down, cursor = %d, want 1", m.cursor)
	}

	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = nm.(Model)
	if m.cursor != 2 {
		t.Fatalf("after j, cursor = %d, want 2", m.cursor)
	}

	// cannot go past the last item (3 findings -> max index 2)
	nm, _ = m.Update(down)
	m = nm.(Model)
	if m.cursor != 2 {
		t.Fatalf("cursor overran end: %d", m.cursor)
	}

	// up / k move back, clamped at 0
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = nm.(Model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = nm.(Model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = nm.(Model)
	if m.cursor != 0 {
		t.Fatalf("cursor underran start: %d", m.cursor)
	}
}

func TestUpdateQuit(t *testing.T) {
	m := New(fixture(), Deps{})
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = nm.(Model)
	if !m.quitting {
		t.Fatal("q should set quitting")
	}
	if cmd == nil {
		t.Fatal("q should return a quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q command should produce tea.QuitMsg")
	}
}
