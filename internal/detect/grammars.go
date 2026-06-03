package detect

import (
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/bash"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/ruby"
	"github.com/smacker/go-tree-sitter/rust"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

// langByExt maps a lowercased file extension to its tree-sitter language.
var langByExt = map[string]*sitter.Language{
	".go":   golang.GetLanguage(),
	".py":   python.GetLanguage(),
	".js":   javascript.GetLanguage(),
	".jsx":  javascript.GetLanguage(),
	".ts":   typescript.GetLanguage(),
	".tsx":  typescript.GetLanguage(),
	".rs":   rust.GetLanguage(),
	".rb":   ruby.GetLanguage(),
	".java": java.GetLanguage(),
	".c":    cpp.GetLanguage(),
	".h":    cpp.GetLanguage(),
	".cc":   cpp.GetLanguage(),
	".cpp":  cpp.GetLanguage(),
	".hpp":  cpp.GetLanguage(),
	".sh":   bash.GetLanguage(),
	".bash": bash.GetLanguage(),
}

func languageFor(path string) *sitter.Language {
	return langByExt[strings.ToLower(filepath.Ext(path))]
}
