package patterns

import (
	"testing"

	"github.com/noamsto/resolved/internal/model"
)

func TestExtractURL(t *testing.T) {
	got := Extract("see https://github.com/noamsto/resolved/pull/7 for fix", "", "")
	if len(got) != 1 {
		t.Fatalf("got %d matches, want 1", len(got))
	}
	m := got[0]
	if m.Kind != model.KindURL || m.Owner != "noamsto" || m.Repo != "resolved" || m.Number != 7 || m.Type != model.TypePR {
		t.Fatalf("bad match: %+v", m)
	}
}

func TestExtractShortForm(t *testing.T) {
	got := Extract("fixed by owner/repo#123", "x", "y")
	if len(got) != 1 || got[0].Kind != model.KindShortForm || got[0].Owner != "owner" || got[0].Number != 123 {
		t.Fatalf("bad short form: %+v", got)
	}
}

func TestExtractBareUsesOrigin(t *testing.T) {
	got := Extract("TODO(#42): drop this", "noamsto", "resolved")
	if len(got) != 1 || got[0].Kind != model.KindBare || got[0].Owner != "noamsto" || got[0].Number != 42 {
		t.Fatalf("bad bare: %+v", got)
	}
}

func TestExtractBareWithoutOriginSkipped(t *testing.T) {
	if got := Extract("TODO(#42)", "", ""); len(got) != 0 {
		t.Fatalf("expected 0 bare matches without origin, got %+v", got)
	}
}

func TestExtractDoesNotDoubleCount(t *testing.T) {
	// A full URL must not also match as short form or bare.
	got := Extract("https://github.com/o/r/issues/5", "o", "r")
	if len(got) != 1 {
		t.Fatalf("expected 1 match, got %d: %+v", len(got), got)
	}
}

func TestExtractBareRejectsNoise(t *testing.T) {
	cases := []string{
		`e.g. "#,##0.00", "yyyy-mm-dd", "0.00%"`, // format patterns (real-world false positive)
		"color #00ff00 and #123abc",              // hex-ish tokens
		"#0 zero and #007 leading-zero",          // invalid issue numbers
		"foo#123 glued and ##456 doubled",        // not standalone tokens
	}
	for _, text := range cases {
		if got := Extract(text, "o", "r"); len(got) != 0 {
			t.Errorf("Extract(%q) = %+v, want no matches", text, got)
		}
	}
}

func TestExtractBareStandaloneForms(t *testing.T) {
	for _, text := range []string{"TODO(#42): drop", "see #42 for details", "[#42] tracked"} {
		got := Extract(text, "o", "r")
		if len(got) != 1 || got[0].Number != 42 {
			t.Errorf("Extract(%q) = %+v, want exactly one #42", text, got)
		}
	}
}

func TestExtractBareRejectsSectionHeaders(t *testing.T) {
	cases := []string{
		"// #7 — generic dispatch error on plan-path is best-effort; no panic.",
		"# #3 section in a shell comment",
		"/* #2 block section */",
		"-- #4 sql section",
	}
	for _, text := range cases {
		if got := Extract(text, "o", "r"); len(got) != 0 {
			t.Errorf("Extract(%q) = %+v, want none (section-header style)", text, got)
		}
	}
	// words before the number => still a ref; and a raw "#5" (check command input) still parses
	for _, text := range []string{"// see #42 for details", "#5"} {
		if got := Extract(text, "o", "r"); len(got) != 1 {
			t.Errorf("Extract(%q) = %+v, want exactly one match", text, got)
		}
	}
}

func TestDetectKeyword(t *testing.T) {
	if kw := DetectKeyword("// TODO: thing", []string{"TODO", "FIXME"}); kw != "TODO" {
		t.Fatalf("got %q", kw)
	}
	if kw := DetectKeyword("// just a note", []string{"TODO"}); kw != "" {
		t.Fatalf("expected no keyword, got %q", kw)
	}
	if kw := DetectKeyword("// TODONT", []string{"TODO"}); kw != "" {
		t.Fatalf("word-boundary failed, got %q", kw)
	}
}
