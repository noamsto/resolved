package detect

import "testing"

func TestCommentsGo(t *testing.T) {
	src := []byte("package main\n// TODO https://github.com/o/r/issues/1\nfunc main() {}\n")
	comments, err := Comments("x.go", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 1 {
		t.Fatalf("got %d comments, want 1: %+v", len(comments), comments)
	}
	if comments[0].Line != 2 {
		t.Fatalf("line = %d, want 2", comments[0].Line)
	}
	if comments[0].Text != "// TODO https://github.com/o/r/issues/1" {
		t.Fatalf("text = %q", comments[0].Text)
	}
}

func TestCommentsIgnoresStringLiteral(t *testing.T) {
	// A URL inside a string literal must NOT be returned as a comment.
	src := []byte(`package main
func main() { _ = "https://github.com/o/r/issues/9" }
`)
	comments, err := Comments("x.go", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 0 {
		t.Fatalf("expected 0 comments, got %+v", comments)
	}
}

func TestCommentsUnknownExtension(t *testing.T) {
	comments, err := Comments("x.unknownext", []byte("// hi"))
	if err != nil {
		t.Fatal(err)
	}
	if comments != nil {
		t.Fatalf("expected nil for unknown extension, got %+v", comments)
	}
}

func TestCommentsNixLine(t *testing.T) {
	src := []byte("{ pkgs }:\n# TODO https://github.com/o/r/issues/1\npkgs.hello\n")
	comments, err := Comments("default.nix", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 1 {
		t.Fatalf("got %d comments, want 1: %+v", len(comments), comments)
	}
	if comments[0].Line != 2 {
		t.Fatalf("line = %d, want 2", comments[0].Line)
	}
	if comments[0].Text != "# TODO https://github.com/o/r/issues/1" {
		t.Fatalf("text = %q", comments[0].Text)
	}
}

func TestCommentsNixBlock(t *testing.T) {
	src := []byte("{ }:\n/* see https://github.com/o/r/issues/2 */\nnull\n")
	comments, err := Comments("x.nix", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 1 {
		t.Fatalf("got %d comments, want 1: %+v", len(comments), comments)
	}
	if comments[0].Text != "/* see https://github.com/o/r/issues/2 */" {
		t.Fatalf("text = %q", comments[0].Text)
	}
}

func TestCommentsNixIgnoresStrings(t *testing.T) {
	// A ref inside a "" string or a '' string is code, not a comment.
	src := []byte("{\n  homepage = \"https://github.com/o/r/issues/8\";\n  script = ''\n    # https://github.com/o/r/issues/9\n  '';\n}\n")
	comments, err := Comments("x.nix", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 0 {
		t.Fatalf("expected 0 comments, got %+v", comments)
	}
}

func TestCommentsYAML(t *testing.T) {
	src := []byte("steps:\n  - run: build # https://github.com/o/r/issues/3\n")
	comments, err := Comments("ci.yaml", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 1 {
		t.Fatalf("got %d comments, want 1: %+v", len(comments), comments)
	}
	if comments[0].Text != "# https://github.com/o/r/issues/3" {
		t.Fatalf("text = %q", comments[0].Text)
	}
}
