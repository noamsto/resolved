package detect

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// Comment is a single comment node's text and 1-based position.
type Comment struct {
	Text string
	Line int
	Col  int
}

// Comments parses src for path's language and returns all comment nodes.
// Returns (nil, nil) for unsupported extensions so callers can skip silently.
func Comments(path string, src []byte) ([]Comment, error) {
	lang := languageFor(path)
	if lang == nil {
		return nil, nil
	}

	parser := sitter.NewParser()
	parser.SetLanguage(lang)
	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	var out []Comment
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		// Grammars name comment nodes "comment", "line_comment",
		// "block_comment", etc. Matching the substring covers them all.
		if strings.Contains(n.Type(), "comment") {
			start := n.StartPoint()
			out = append(out, Comment{
				Text: n.Content(src),
				Line: int(start.Row) + 1,
				Col:  int(start.Column) + 1,
			})
			return
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i))
		}
	}
	walk(tree.RootNode())

	return out, nil
}
