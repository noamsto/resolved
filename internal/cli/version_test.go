package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	buf := new(bytes.Buffer)
	if err := run([]string{"version"}, buf, buf); err != nil {
		t.Fatalf("execute: %v", err)
	}
	// Assert against the package var, not a literal: a release build stamps it
	// via ldflags (e.g. nix build), so the output is not always "dev".
	if got := strings.TrimSpace(buf.String()); got != version {
		t.Fatalf("version output = %q, want %q", got, version)
	}
}
