package tui

import (
	"strings"
	"testing"

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
