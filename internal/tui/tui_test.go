package tui

import (
	"fmt"
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

func TestEnterOpensIssueURL(t *testing.T) {
	var opened string
	m := New(fixture(), Deps{
		OpenURL: func(url string) error { opened = url; return nil },
	})
	// cursor at 0 -> stale finding o/r#1 (issue)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	if opened != "https://github.com/o/r/issues/1" {
		t.Fatalf("opened = %q", opened)
	}
	if m.status == "" {
		t.Fatal("expected a status message after opening")
	}
}

func TestEditInvokesEditorCmd(t *testing.T) {
	var gotFile string
	var gotLine int
	m := New(fixture(), Deps{
		EditorCmd: func(file string, line int) tea.Cmd {
			gotFile, gotLine = file, line
			return nil
		},
	})
	// cursor at 0 -> a.go:2 after sorting (stale finding)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	_ = nm.(Model)
	if gotFile != "a.go" || gotLine != 2 {
		t.Fatalf("editor invoked with %s:%d, want a.go:2", gotFile, gotLine)
	}
}

func TestEditorDoneSetsErrorStatus(t *testing.T) {
	m := New(fixture(), Deps{})
	nm, _ := m.Update(editorDoneMsg{err: errTest})
	m = nm.(Model)
	if !strings.Contains(m.status, "editor") {
		t.Fatalf("status = %q, want editor error", m.status)
	}
}

var errTest = fmt.Errorf("boom")

func TestRefreshReplacesFindings(t *testing.T) {
	fresh := []model.Finding{
		{Reference: model.Reference{File: "b.go", Line: 1, Owner: "o", Repo: "r", Number: 9}, Tier: model.TierOpen},
	}
	m := New(fixture(), Deps{
		Rescan: func() ([]model.Finding, error) { return fresh, nil },
	})

	// pressing r returns a command that performs the rescan
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(Model)
	if cmd == nil {
		t.Fatal("r should return a rescan command")
	}
	msg := cmd()
	done, ok := msg.(rescanDoneMsg)
	if !ok {
		t.Fatalf("rescan command produced %T, want rescanDoneMsg", msg)
	}

	// feeding the done message back replaces the findings
	nm, _ = m.Update(done)
	m = nm.(Model)
	if len(m.findings) != 1 || m.findings[0].Number != 9 {
		t.Fatalf("findings not replaced: %+v", m.findings)
	}
	if !strings.Contains(m.status, "refreshed") {
		t.Fatalf("status = %q, want refreshed", m.status)
	}
}

func TestRefreshErrorSetsStatus(t *testing.T) {
	m := New(fixture(), Deps{})
	nm, _ := m.Update(rescanDoneMsg{err: errTest})
	m = nm.(Model)
	if !strings.Contains(m.status, "refresh failed") {
		t.Fatalf("status = %q", m.status)
	}
}
