package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/noamsto/resolved/internal/cache"
	"github.com/noamsto/resolved/internal/model"
)

// fakeFetcher returns canned statuses and records how many refs it was asked for.
type fakeFetcher struct {
	statuses map[string]model.Status
	asked    int
}

func (f *fakeFetcher) Fetch(_ context.Context, refs []model.Reference) (map[string]model.Status, error) {
	f.asked += len(refs)
	out := map[string]model.Status{}
	for _, r := range refs {
		out[r.Key()] = f.statuses[r.Key()]
	}
	return out, nil
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRunClassifiesStale(t *testing.T) {
	dir := t.TempDir()
	f := writeFile(t, dir, "a.go",
		"package main\n// TODO https://github.com/o/r/issues/1\nfunc main(){}\n")

	fetcher := &fakeFetcher{statuses: map[string]model.Status{
		"o/r#1": {State: "closed", Title: "bug"},
	}}

	res, err := Run(context.Background(), Options{
		Targets:  []string{f},
		Keywords: []string{"TODO"},
		Cache:    cache.New(t.TempDir()),
		GitHub:   fetcher,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) != 1 {
		t.Fatalf("got %d findings, want 1", len(res.Findings))
	}
	if res.Findings[0].Tier != model.TierStale {
		t.Fatalf("tier = %v, want stale", res.Findings[0].Tier)
	}
	if res.Summary.Stale != 1 {
		t.Fatalf("summary.Stale = %d, want 1", res.Summary.Stale)
	}
}

func TestRunCountsGone(t *testing.T) {
	dir := t.TempDir()
	f := writeFile(t, dir, "a.go",
		"package main\n// see https://github.com/o/r/issues/9\nfunc main(){}\n")
	fetcher := &fakeFetcher{statuses: map[string]model.Status{
		"o/r#9": {State: "gone"},
	}}
	res, err := Run(context.Background(), Options{
		Targets: []string{f}, Keywords: []string{"TODO"},
		Cache: cache.New(t.TempDir()), GitHub: fetcher,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.Gone != 1 {
		t.Fatalf("Summary.Gone = %d, want 1", res.Summary.Gone)
	}
	if res.Summary.Unknown != 0 {
		t.Fatalf("Summary.Unknown = %d, want 0 (gone must not count as unknown)", res.Summary.Unknown)
	}
}

func TestRunSuppressesGoneBareRefs(t *testing.T) {
	dir := t.TempDir()
	f := writeFile(t, dir, "a.go",
		"package main\n// see #9 which never existed\nfunc main(){}\n")
	fetcher := &fakeFetcher{statuses: map[string]model.Status{
		"o/r#9": {State: "gone"},
	}}
	res, err := Run(context.Background(), Options{
		Targets: []string{f}, Keywords: []string{"TODO"},
		Owner: "o", Repo: "r",
		Cache: cache.New(t.TempDir()), GitHub: fetcher,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) != 0 {
		t.Fatalf("bare ref resolving to gone should be suppressed, got %+v", res.Findings)
	}
	if res.Summary.Refs != 0 || res.Summary.Gone != 0 {
		t.Fatalf("summary should not count suppressed bare refs: %+v", res.Summary)
	}
}

func TestRunDedupesAndUsesCache(t *testing.T) {
	dir := t.TempDir()
	f := writeFile(t, dir, "a.go",
		"package main\n// see #1 and again #1\nfunc main(){}\n")

	c := cache.New(t.TempDir())
	c.Put("o/r#1", model.Status{State: "open"})

	fetcher := &fakeFetcher{statuses: map[string]model.Status{}}

	res, err := Run(context.Background(), Options{
		Targets:  []string{f},
		Keywords: []string{"TODO"},
		Owner:    "o", Repo: "r",
		Cache:  c,
		GitHub: fetcher,
	})
	if err != nil {
		t.Fatal(err)
	}
	if fetcher.asked != 0 {
		t.Fatalf("fetcher asked for %d refs, want 0 (all cached)", fetcher.asked)
	}
	if len(res.Findings) != 2 {
		t.Fatalf("got %d findings, want 2", len(res.Findings))
	}
}

func TestScanReturnsUnclassifiedFindings(t *testing.T) {
	dir := t.TempDir()
	f := writeFile(t, dir, "a.go",
		"package main\n// TODO https://github.com/o/r/issues/1\nfunc main(){}\n")

	findings, summary, err := Scan(Options{Targets: []string{f}, Keywords: []string{"TODO"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1", len(findings))
	}
	if findings[0].Tier != model.TierUnknown {
		t.Fatalf("tier = %v, want unknown (no status fetched yet)", findings[0].Tier)
	}
	if findings[0].State != "" {
		t.Fatalf("state = %q, want empty before resolve", findings[0].State)
	}
	if summary.Scanned != 1 || summary.Refs != 1 {
		t.Fatalf("summary = %+v, want scanned=1 refs=1", summary)
	}
}

func TestRunCountsUnsupportedFilesAsSkipped(t *testing.T) {
	dir := t.TempDir()
	goFile := writeFile(t, dir, "a.go",
		"package main\n// TODO https://github.com/o/r/issues/1\nfunc main(){}\n")
	unknownFile := writeFile(t, dir, "b.zzqq",
		"// TODO https://github.com/o/r/issues/2\nclass C {}\n")

	fetcher := &fakeFetcher{statuses: map[string]model.Status{
		"o/r#1": {State: "open", Title: "x"},
	}}
	res, err := Run(context.Background(), Options{
		Targets:  []string{goFile, unknownFile},
		Keywords: []string{"TODO"},
		Cache:    cache.New(t.TempDir()),
		GitHub:   fetcher,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.Scanned != 1 {
		t.Fatalf("Scanned = %d, want 1 (only the parsed .go file)", res.Summary.Scanned)
	}
	if res.Summary.Skipped != 1 {
		t.Fatalf("Skipped = %d, want 1 (the unsupported .zzqq file)", res.Summary.Skipped)
	}
}
