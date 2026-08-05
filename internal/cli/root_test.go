package cli

import (
	"io"
	"slices"
	"testing"

	"github.com/alecthomas/kong"
)

// newParser builds the real grammar. Kong validates struct tags when the parser
// is constructed, so a malformed tag fails here instead of at runtime.
func newParser(t *testing.T) (*kong.Kong, *CLI) {
	t.Helper()
	var cli CLI
	parser, err := kong.New(&cli,
		kong.Name("resolved"),
		kong.Writers(io.Discard, io.Discard),
	)
	if err != nil {
		t.Fatalf("grammar: %v", err)
	}
	return parser, &cli
}

func parseInto(t *testing.T, args ...string) *CLI {
	t.Helper()
	parser, cli := newParser(t)
	if _, err := parser.Parse(args); err != nil {
		t.Fatalf("parse %v: %v", args, err)
	}
	return cli
}

func TestScanFlagsParse(t *testing.T) {
	c := parseInto(t, "scan", "a.go", "b.go",
		"--fail-on", "any", "--json", "--no-color",
		"--staged", "--diff", "HEAD~1",
		"--exclude", "*.md,*.txt",
		"--keywords", "TODO,FIXME",
		"--no-cache", "--bare",
	).Scan

	if !slices.Equal(c.Paths, []string{"a.go", "b.go"}) {
		t.Errorf("paths = %v", c.Paths)
	}
	if c.FailOn != "any" || !c.JSON || !c.NoColor {
		t.Errorf("output flags = %+v", c)
	}
	// Slice flags split on commas, so one --exclude can carry several globs.
	if !slices.Equal(c.Targets.Exclude, []string{"*.md", "*.txt"}) {
		t.Errorf("exclude = %v", c.Targets.Exclude)
	}
	if !slices.Equal(c.Targets.Keywords, []string{"TODO", "FIXME"}) {
		t.Errorf("keywords = %v", c.Targets.Keywords)
	}
	if !c.Targets.Staged || c.Targets.Diff != "HEAD~1" || !c.Targets.NoCache || !c.Targets.Bare {
		t.Errorf("target flags = %+v", c.Targets)
	}
}

func TestScanDefaults(t *testing.T) {
	c := parseInto(t, "scan").Scan
	if c.FailOn != "stale" {
		t.Errorf("--fail-on default = %q, want stale", c.FailOn)
	}
	if len(c.Paths) != 0 {
		t.Errorf("paths = %v, want none", c.Paths)
	}
	if got := c.Targets.config(t.TempDir(), nil).keywords; !slices.Equal(got, defaultKeywords) {
		t.Errorf("keywords = %v, want %v", got, defaultKeywords)
	}
}

func TestExploreFlagsParse(t *testing.T) {
	c := parseInto(t, "explore", "--theme", "latte", "--no-popup", "--staged").Explore
	if c.Theme != "latte" || !c.NoPopup || !c.Targets.Staged {
		t.Errorf("explore flags = %+v", c)
	}
	if d := parseInto(t, "explore").Explore; d.Theme != "mocha" {
		t.Errorf("--theme default = %q, want mocha", d.Theme)
	}
}

func TestCheckTakesExactlyOneRef(t *testing.T) {
	if c := parseInto(t, "check", "#5").Check; c.Ref != "#5" {
		t.Errorf("ref = %q", c.Ref)
	}
}

func TestParseErrors(t *testing.T) {
	for _, args := range [][]string{
		{"check"},
		{"check", "#1", "#2"},
		{"frobnicate"},
		{"scan", "--nonexistent-flag"},
	} {
		parser, _ := newParser(t)
		if _, err := parser.Parse(args); err == nil {
			t.Errorf("%v: expected a parse error", args)
		}
	}
}
