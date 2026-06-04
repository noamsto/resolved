package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/noamsto/resolved/internal/model"
)

func TestCollapseHome(t *testing.T) {
	t.Setenv("HOME", "/home/test")
	cases := map[string]string{
		"/home/test/Data/x.go": "~/Data/x.go",
		"/home/test":           "~",
		"/etc/passwd":          "/etc/passwd",
		"relative/x.go":        "relative/x.go",
	}
	for in, want := range cases {
		if got := collapseHome(in); got != want {
			t.Errorf("collapseHome(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDetailShowsTildePath(t *testing.T) {
	t.Setenv("HOME", "/home/test")
	f := mkF("/home/test/proj/main.go", 5, model.TierStale, time.Time{})
	m := New([]model.Finding{f}, Deps{}, Mocha())
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = nm.(Model)
	out := strip(m.View().Content)
	if !strings.Contains(out, "~/proj/main.go") {
		t.Fatalf("detail should show ~-collapsed path:\n%s", out)
	}
	if strings.Contains(out, "/home/test/proj/main.go") {
		t.Fatalf("home path should be collapsed, not shown in full:\n%s", out)
	}
}

func TestFooterAdvertisesSortKey(t *testing.T) {
	m := New(fixture(), Deps{}, Mocha())
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = nm.(Model)
	if !strings.Contains(strip(m.View().Content), "s sort") {
		t.Fatalf("footer should advertise the s sort key:\n%s", strip(m.View().Content))
	}
}
