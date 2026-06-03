package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/noamsto/resolved/internal/model"
)

func TestEndToEndScanMixedTiers(t *testing.T) {
	dir := t.TempDir()
	src := "package main\n" +
		"// FIXME https://github.com/o/r/issues/1\n" + // stale (closed + keyword)
		"// docs https://github.com/o/r/issues/2\n" + // closed, no keyword
		"// tracking https://github.com/o/r/issues/3\n" + // open
		"func main() {}\n"
	writeTestFile(t, dir, "main.go", src)

	fetcher := stubFetcher{statuses: map[string]model.Status{
		"o/r#1": {State: "closed", Title: "one"},
		"o/r#2": {State: "closed", Title: "two"},
		"o/r#3": {State: "open", Title: "three"},
	}}

	buf := new(bytes.Buffer)
	code, err := runScan(scanConfig{
		dir: dir, args: []string{dir}, keywords: defaultKeywords, failOn: "stale",
		json: true, fetcher: fetcher, out: buf, noCache: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	out := buf.String()
	for _, want := range []string{`"stale"`, `"closed"`, `"open"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in output:\n%s", want, out)
		}
	}
}
