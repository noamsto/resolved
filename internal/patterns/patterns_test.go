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
