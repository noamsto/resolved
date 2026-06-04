package tui

import (
	"os"
	"strings"
)

const sourceUnavailable = "(source unavailable)"

// sourceCache reads and caches file contents (split into lines) so the detail
// pane can show a reference's source line without re-reading on every render.
type sourceCache struct {
	files map[string][]string
	read  map[string]bool
}

func newSourceCache() *sourceCache {
	return &sourceCache{files: map[string][]string{}, read: map[string]bool{}}
}

// lines returns the cached file content split into lines, ok=false if unreadable.
func (c *sourceCache) lines(path string) ([]string, bool) {
	if !c.read[path] {
		c.read[path] = true
		if data, err := os.ReadFile(path); err == nil {
			c.files[path] = strings.Split(string(data), "\n")
		}
	}
	l, ok := c.files[path]
	return l, ok
}

// snippet returns the trimmed source line (1-based) at path, or
// "(source unavailable)" if the file can't be read or the line is out of range.
func (c *sourceCache) snippet(path string, line int) string {
	lines, ok := c.lines(path)
	if !ok || line < 1 || line > len(lines) {
		return sourceUnavailable
	}
	return strings.TrimSpace(lines[line-1])
}
