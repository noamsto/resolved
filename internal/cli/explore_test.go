package cli

import (
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

func TestThemeByNameInvalid(t *testing.T) {
	if _, err := tui.ThemeByName("nope"); err == nil {
		t.Fatal("expected error for invalid theme")
	}
	if _, err := tui.ThemeByName("mocha"); err != nil {
		t.Fatalf("mocha should resolve: %v", err)
	}
}
