package tui

import (
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// highlight renders code with ANSI syntax colors for the file's language and
// the theme's chroma style. Falls back to plain text on any failure.
func highlight(code, filename, styleName string) string {
	lexer := lexers.Match(filepath.Base(filename))
	if lexer == nil {
		return code
	}
	style := styles.Get(styleName)
	if style == nil {
		style = styles.Fallback
	}
	formatter := formatters.Get("terminal16m")
	if formatter == nil {
		return code
	}
	it, err := chroma.Coalesce(lexer).Tokenise(nil, code)
	if err != nil {
		return code
	}
	var b strings.Builder
	if err := formatter.Format(&b, style, it); err != nil {
		return code
	}
	return b.String()
}
