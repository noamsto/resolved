package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/noamsto/resolved/internal/model"
)

// collapseHome shortens an absolute path under the user's home directory to a
// leading "~" for display. The real path is unchanged elsewhere.
func collapseHome(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+string(os.PathSeparator)) {
		return "~" + p[len(home):]
	}
	return p
}

// shortLoc renders the last two components of the root-relative path
// (parent_dir/file) — enough to disambiguate common basenames like handler.go
// without spending row width on the full path. Deriving it from displayPath
// keeps it consistent with the detail pane, so a file at the scan root shows
// just its name (it has no parent), not the repo directory name.
func (m Model) shortLoc(p string) string {
	dir, file := filepath.Split(m.displayPath(p))
	parent := filepath.Base(filepath.Clean(dir))
	if parent == "." || parent == string(os.PathSeparator) {
		return file
	}
	return parent + "/" + file
}

// displayPath renders p relative to the scan root when it's under the root;
// otherwise it falls back to the ~-collapsed absolute path. The real path is
// unchanged elsewhere (editor/snippet use it).
func (m Model) displayPath(p string) string {
	if root := m.deps.Root; root != "" {
		if rel, err := filepath.Rel(root, p); err == nil &&
			rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return rel
		}
	}
	return collapseHome(p)
}

// renderAll composes header + (list | detail) + footer into the full screen.
func (m Model) renderAll() string {
	width := m.width
	if width <= 0 {
		width = 80
	}

	header := m.renderHeader(width)
	footer := m.renderFooter(width)

	listW := width / 2 // even split: the rows now carry state + title, so the
	if listW < 24 {    // list earns as much width as the source-preview pane
		listW = 24
	}
	detailW := width - listW // Width includes the border, so the panes tile exactly
	if detailW < 20 {
		detailW = 20
	}

	ph := m.listHeight()
	paneH := ph + 2                                 // Height includes the border; ph is the inner content height
	frame := m.styles.pane.GetHorizontalFrameSize() // border + padding (l+r)
	listInner := listW - frame
	if listInner < 6 {
		listInner = 6
	}
	detailInner := detailW - frame
	if detailInner < 6 {
		detailInner = 6
	}
	// MaxHeight hard-clips: Height is only a minimum, and content that wraps
	// would otherwise grow the pane and push the footer off-screen.
	body := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.styles.pane.Width(listW).Height(paneH).MaxHeight(paneH).Render(m.renderList(listInner)),
		m.styles.pane.Width(detailW).Height(paneH).MaxHeight(paneH).Render(m.renderDetail(detailInner, ph)),
	)

	return strings.Join([]string{header, body, footer}, "\n")
}

func (m Model) renderHeader(width int) string {
	s := m.summary()
	left := "resolved"
	if m.deps.Root != "" {
		left += "  " + collapseHome(m.deps.Root)
	}
	right := fmt.Sprintf("%d refs · %d stale · %d closed · %d open · %d gone",
		len(m.findings), s.stale, s.closed, s.open, s.gone)
	if s.unknown > 0 {
		right += fmt.Sprintf(" · %d unknown", s.unknown)
	}
	right += " · sort: " + m.mode.label()

	inner := width - m.styles.header.GetHorizontalFrameSize()
	gap := inner - lipgloss.Width(left) - lipgloss.Width(right)
	line := left + "  " + right
	if gap >= 1 {
		line = left + strings.Repeat(" ", gap) + right
	}
	return m.styles.header.Width(width).Render(ansi.Truncate(line, inner, "…"))
}

func (m Model) renderFooter(width int) string {
	help := "j/k move · s sort · ⏎ open · e edit · y/Y yank · r refresh · q quit"
	switch {
	case m.loading:
		help = m.spinner() + " " + m.loadProgress() + "   " + help
	case m.status != "":
		help = m.status + "   " + help
	}
	inner := width - m.styles.footer.GetHorizontalFrameSize()
	return m.styles.footer.Width(width).Render(ansi.Truncate(help, inner, "…"))
}

// locColWidth is the width of the file/line column: the longest entry across
// all findings, capped so the ref column still fits. This left-packs the layout
// (refs sit right after the longest filename) rather than stretching to the
// pane's right edge.
func (m Model) locColWidth(width int) int {
	maxLoc, maxRef := 0, 0
	for _, f := range m.findings {
		var loc string
		if m.mode == modeFile {
			loc = fmt.Sprintf(":%d", f.Line)
		} else {
			loc = fmt.Sprintf("%s:%d", m.shortLoc(f.File), f.Line)
		}
		if w := lipgloss.Width(loc); w > maxLoc {
			maxLoc = w
		}
		if w := lipgloss.Width(fmt.Sprintf("%s/%s#%d", f.Owner, f.Repo, f.Number)); w > maxRef {
			maxRef = w
		}
	}
	const markerW, iconW, gap = 2, 2, 1
	budget := width - markerW - iconW - gap - maxRef
	if budget < 8 {
		budget = 8
	}
	if maxLoc < 1 {
		maxLoc = 1
	}
	if maxLoc < budget {
		return maxLoc
	}
	return budget
}

// spinner returns the current braille frame.
func (m Model) spinner() string {
	return spinFrames[m.spinFrame%len(spinFrames)]
}

// loadProgress describes the current load phase for the footer.
func (m Model) loadProgress() string {
	if m.resolveTotal == 0 {
		return "scanning…"
	}
	return fmt.Sprintf("resolving %d/%d issues…", m.resolveDone, m.resolveTotal)
}

func (m Model) renderList(width int) string {
	rows := m.displayRows()
	if len(rows) == 0 {
		if m.loading {
			return m.spinner() + " " + m.loadProgress()
		}
		return "no references found"
	}
	vh := m.listHeight()
	end := m.listOffset + vh
	if end > len(rows) {
		end = len(rows)
	}
	locW := m.locColWidth(width)
	var b strings.Builder
	for ri := m.listOffset; ri < end; ri++ {
		r := rows[ri]
		if r.header {
			b.WriteString(m.styles.fileHeader.Render("▸ " + trimMid(m.displayPath(r.text), width-2)))
			b.WriteString("\n")
			continue
		}
		b.WriteString(m.renderFindingRow(m.findings[r.idx], r.idx == m.cursor, locW, width))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) renderFindingRow(f model.Finding, selected bool, locW, width int) string {
	icon, _ := tierMeta(f.Tier)
	ref := fmt.Sprintf("%s/%s#%d", f.Owner, f.Repo, f.Number)

	marker := "  "
	if selected {
		marker = "❯ "
	}
	lineStr := fmt.Sprintf(":%d", f.Line)

	var loc string
	if m.mode == modeFile {
		loc = lineStr // the file group header already carries the full path
	} else {
		nameBudget := locW - lipgloss.Width(lineStr)
		if nameBudget < 3 {
			nameBudget = 3
		}
		loc = trimMid(m.shortLoc(f.File), nameBudget) + lineStr
	}
	locCell := lipgloss.NewStyle().Width(locW).Render(loc)

	// State + title surface the fetched issue info in the row. %-7s pads to
	// "unknown" so the ref column stays aligned; an empty state renders "…"
	// while it's still being resolved.
	state := f.State
	if state == "" {
		state = "…"
	}
	row := marker + icon + " " + locCell + " " + fmt.Sprintf("%-7s", state) + " " + ref
	if f.Title != "" {
		row += " — " + f.Title
	}
	// Truncate instead of letting a long title wrap the row past the pane width.
	row = ansi.Truncate(row, width, "…")
	if selected {
		return m.styles.selectedRowFor(f.Tier).Width(width).Render(row)
	}
	return lipgloss.NewStyle().Foreground(m.styles.tierColor(f.Tier)).Render(row)
}

func (m Model) renderDetail(width, height int) string {
	f, ok := m.current()
	if !ok {
		return "no references"
	}
	kw := f.Keyword
	if kw == "" {
		kw = "—"
	}
	lines := []string{
		m.styles.tierBadge(f.Tier) + "  " + fmt.Sprintf("%s/%s#%d", f.Owner, f.Repo, f.Number),
		lipgloss.NewStyle().Bold(true).Render(f.Title),
		"",
		m.styles.detailKey.Render("state    ") + f.State,
		m.styles.detailKey.Render("file     ") + fmt.Sprintf("%s:%d", m.displayPath(f.File), f.Line),
		m.styles.detailKey.Render("url      ") + issueURL(f),
		m.styles.detailKey.Render("keyword  ") + kw,
		m.styles.detailKey.Render("kind     ") + fmt.Sprintf("%s · %s", f.Kind, f.Confidence),
		"",
	}
	// Truncate instead of letting lipgloss wrap: a wrapped line grows the pane
	// past its Height and pushes the footer off-screen.
	for i, ln := range lines {
		lines[i] = ansi.Truncate(ln, width, "…")
	}
	if avail := height - len(lines); avail >= 1 {
		lines = append(lines, m.renderPreview(f, width, avail)...)
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

// renderPreview renders a source window around the reference line with a
// line-number gutter; the ref line is marked. Uses up to avail lines.
func (m Model) renderPreview(f model.Finding, width, avail int) []string {
	if avail < 1 {
		return nil
	}
	src, ok := m.sources.lines(f.File)
	if !ok {
		return []string{m.styles.snippet.Render(sourceUnavailable)}
	}
	start := f.Line - 2
	if start < 1 {
		start = 1
	}
	end := start + avail - 1
	if end > len(src) {
		end = len(src)
		start = end - avail + 1
		if start < 1 {
			start = 1
		}
	}
	code := strings.Join(src[start-1:end], "\n")
	colored := highlight(code, f.File, m.theme.Chroma)
	cl := strings.Split(colored, "\n")
	gutterW := len(fmt.Sprintf("%d", end))
	out := make([]string, 0, len(cl))
	for i, ln := range cl {
		n := start + i
		marker := " "
		g := m.styles.detailKey.Render(fmt.Sprintf("%*d", gutterW, n))
		if n == f.Line {
			marker = "▶"
			g = lipgloss.NewStyle().Bold(true).Foreground(m.theme.Accent).Render(fmt.Sprintf("%*d", gutterW, n))
		}
		out = append(out, ansi.Truncate(fmt.Sprintf("%s %s │ %s", marker, g, ln), width, "…"))
	}
	return out
}

type tierCounts struct{ stale, closed, gone, open, unknown int }

func (m Model) summary() tierCounts {
	var c tierCounts
	for _, f := range m.findings {
		switch f.Tier {
		case model.TierStale:
			c.stale++
		case model.TierClosed:
			c.closed++
		case model.TierGone:
			c.gone++
		case model.TierOpen:
			c.open++
		default:
			c.unknown++
		}
	}
	return c
}

// trimMid shortens s to max runes, keeping the tail (filenames read better
// truncated on the left).
func trimMid(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return "…" + s[len(s)-max+1:]
}
