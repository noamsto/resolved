package detect

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// Comment is a single comment node's text and 1-based position.
type Comment struct {
	Text string
	Line int
	Col  int
}

// Supported reports whether a lexer matches path, i.e. whether Comments can
// extract anything from it.
func Supported(path string) bool {
	return lexers.Match(path) != nil
}

// Comments tokenizes src with the lexer matched to path and returns every
// comment token's text with its 1-based position. Returns (nil, nil) when no
// lexer matches path, so callers can skip silently. String/code tokens are
// never returned, so a reference inside a literal is not mistaken for a comment.
func Comments(path string, src []byte) ([]Comment, error) {
	lexer := lexers.Match(path)
	if lexer == nil {
		return nil, nil
	}
	it, err := chroma.Coalesce(lexer).Tokenise(nil, string(src))
	if err != nil {
		return nil, err
	}

	var out []Comment
	line, col := 1, 0 // col is a 0-based byte offset within the current line
	for _, tok := range it.Tokens() {
		if tok.Type.InCategory(chroma.Comment) {
			out = append(out, Comment{
				Text: strings.TrimRight(tok.Value, "\r\n"),
				Line: line,
				Col:  col + 1,
			})
		}
		for i := 0; i < len(tok.Value); i++ {
			if tok.Value[i] == '\n' {
				line++
				col = 0
			} else {
				col++
			}
		}
	}
	return out, nil
}
