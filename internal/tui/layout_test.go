package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/noamsto/resolved/internal/model"
)

// layoutModel builds a model with one finding pointing into a real 60-line
// source file (so the preview always has lines to fill) plus a filler finding.
func layoutModel(t *testing.T, title string, w, h int) Model {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "demo.go")
	var b strings.Builder
	b.WriteString("package d\n")
	for i := 0; i < 60; i++ {
		b.WriteString("var x = 1 // filler line\n")
	}
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	f := model.Finding{
		Reference: model.Reference{File: p, Line: 30, Owner: "o", Repo: "r", Number: 1, Type: model.TypeIssue},
		Status:    model.Status{State: "closed", Title: title},
		Tier:      model.TierStale,
	}
	m := New([]model.Finding{f, mkF("a.go", 1, model.TierOpen, time.Time{})}, Deps{}, Mocha())
	nm, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return nm.(Model)
}

// The rendered view must always be exactly the terminal height: a detail pane
// with wrapping content (long titles, long paths) must not grow and push the
// footer off-screen.
func TestViewFitsTerminalHeight(t *testing.T) {
	for _, tc := range []struct {
		name  string
		title string
		w, h  int
	}{
		{"short title", "x", 100, 24},
		{"long title", strings.Repeat("a very long issue title ", 8), 100, 24},
		{"long title narrow", strings.Repeat("word ", 40), 80, 24},
		{"tiny height", "x", 100, 12},
		{"tall", "x", 120, 50},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := layoutModel(t, tc.title, tc.w, tc.h)
			out := m.View().Content
			if got := lipgloss.Height(out); got != tc.h {
				t.Fatalf("rendered %d lines, want exactly %d:\n%s", got, tc.h, strip(out))
			}
			last := strings.Split(strip(out), "\n")
			if !strings.Contains(last[len(last)-1], "q quit") {
				t.Fatalf("footer must be the last line, got %q", last[len(last)-1])
			}
		})
	}
}

// A long status message must not wrap the footer onto a second line.
func TestFooterStatusDoesNotWrap(t *testing.T) {
	m := layoutModel(t, "x", 80, 24)
	m.status = "opened https://github.com/some-owner/some-repo/issues/123456 in the browser"
	if got := lipgloss.Height(m.View().Content); got != 24 {
		t.Fatalf("rendered %d lines with long status, want 24", got)
	}
}
