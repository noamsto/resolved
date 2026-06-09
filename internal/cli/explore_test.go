package cli

import (
	"slices"
	"testing"

	"github.com/noamsto/resolved/internal/model"
	tui "github.com/noamsto/resolved/internal/tui"
)

func TestExploreFindings(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.go",
		"package main\n// TODO https://github.com/o/r/issues/1\nfunc main(){}\n")

	fetcher := stubFetcher{statuses: map[string]model.Status{"o/r#1": {State: "closed", Title: "bug"}}}

	findings, err := exploreFindings(scanConfig{
		dir: dir, args: []string{dir}, keywords: []string{"TODO"},
		fetcher: fetcher, noCache: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Tier != model.TierStale {
		t.Fatalf("unexpected findings: %+v", findings)
	}
}

func TestTmuxPopupArgs(t *testing.T) {
	got := tmuxPopupArgs("/usr/bin/resolved", "/work", []string{"explore", "--theme", "latte"})
	want := []string{
		"display-popup", "-E", "-e", "RESOLVED_IN_POPUP=1", "-w", "90%", "-h", "90%", "-d", "/work", "--",
		"/usr/bin/resolved", "explore", "--theme", "latte",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("popup args mismatch:\n got: %v\nwant: %v", got, want)
	}
}

func TestTmuxPopupArgsPreservesDoubleDash(t *testing.T) {
	// The recursion guard is injected with `-e` (an env var, not a CLI flag of
	// ours), so a user's `--` — which would turn a trailing flag into a
	// positional arg — must pass through clean.
	got := tmuxPopupArgs("/usr/bin/resolved", "/work", []string{"explore", "--", "weird-path"})
	want := []string{
		"display-popup", "-E", "-e", "RESOLVED_IN_POPUP=1", "-w", "90%", "-h", "90%", "-d", "/work", "--",
		"/usr/bin/resolved", "explore", "--", "weird-path",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("popup args mismatch:\n got: %v\nwant: %v", got, want)
	}
}

func TestShouldPopup(t *testing.T) {
	t.Setenv(popupGuardEnv, "")
	t.Setenv("TMUX", "")
	if shouldPopup(false) {
		t.Fatal("no TMUX: should not popup")
	}
	t.Setenv("TMUX", "/tmp/tmux-1000/default,5930,5")
	if !shouldPopup(false) {
		t.Fatal("TMUX set: should popup")
	}
	if shouldPopup(true) {
		t.Fatal("--no-popup must override TMUX")
	}
	t.Setenv(popupGuardEnv, "1")
	if shouldPopup(false) {
		t.Fatal("already in popup: must not re-launch")
	}
}

func TestThemeByNameInvalid(t *testing.T) {
	if _, err := tui.ThemeByName("nope"); err == nil {
		t.Fatal("expected error for invalid theme")
	}
	if _, err := tui.ThemeByName("mocha"); err != nil {
		t.Fatalf("mocha should resolve: %v", err)
	}
}
