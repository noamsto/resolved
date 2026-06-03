package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/noamsto/resolved/internal/model"
)

type stubFetcher struct{ statuses map[string]model.Status }

func (s stubFetcher) Fetch(_ context.Context, refs []model.Reference) (map[string]model.Status, error) {
	out := map[string]model.Status{}
	for _, r := range refs {
		out[r.Key()] = s.statuses[r.Key()]
	}
	return out, nil
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunScanJSONAndExitCode(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.go",
		"package main\n// TODO https://github.com/o/r/issues/1\nfunc main(){}\n")

	fetcher := stubFetcher{statuses: map[string]model.Status{"o/r#1": {State: "closed", Title: "bug"}}}

	buf := new(bytes.Buffer)
	code, err := runScan(scanConfig{
		dir:      dir,
		keywords: []string{"TODO"},
		failOn:   "stale",
		json:     true,
		fetcher:  fetcher,
		out:      buf,
	})
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stale found)", code)
	}
	if !strings.Contains(buf.String(), `"tier": "stale"`) {
		t.Fatalf("expected stale finding in JSON:\n%s", buf.String())
	}
}
