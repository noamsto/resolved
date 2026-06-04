package tui

import (
	"fmt"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/noamsto/resolved/internal/model"
)

// renderAll composes header + (list | detail) + footer into the full screen.
func (m Model) renderAll() string {
	width := m.width
	if width <= 0 {
		width = 80
	}

	header := m.renderHeader(width)
	footer := m.renderFooter(width)

	listW := width * 2 / 5
	if listW < 24 {
		listW = 24
	}
	detailW := width - listW - 4
	if detailW < 20 {
		detailW = 20
	}

	ph := m.listHeight()
	frame := m.styles.pane.GetHorizontalFrameSize() // border + padding (l+r)
	listInner := listW - frame
	if listInner < 6 {
		listInner = 6
	}
	detailInner := detailW - frame
	if detailInner < 6 {
		detailInner = 6
	}
	body := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.styles.pane.Width(listW).Height(ph).Render(m.renderList(listInner)),
		m.styles.pane.Width(detailW).Height(ph).Render(m.renderDetail(detailInner)),
	)

	return strings.Join([]string{header, body, footer}, "\n")
}

func (m Model) renderHeader(width int) string {
	s := m.summary()
	line := fmt.Sprintf("resolved · %d refs · %d stale · %d closed · %d open · %d gone",
		len(m.findings), s.stale, s.closed, s.open, s.gone)
	if s.unknown > 0 {
		line += fmt.Sprintf(" · %d unknown", s.unknown)
	}
	line += "  · sort: " + m.mode.label()
	return m.styles.header.Width(width).Render(line)
}

func (m Model) renderFooter(width int) string {
	help := "j/k move · ⏎ open · e edit · r refresh · q quit"
	if m.status != "" {
		help = m.status + "   " + help
	}
	return m.styles.footer.Width(width).Render(help)
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
			loc = fmt.Sprintf("%s:%d", f.File, f.Line)
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

func (m Model) renderList(width int) string {
	rows := m.displayRows()
	if len(rows) == 0 {
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
			b.WriteString(m.styles.fileHeader.Render("▸ " + trimMid(r.text, width-2)))
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
		loc = lineStr
	} else {
		nameBudget := locW - lipgloss.Width(lineStr)
		if nameBudget < 3 {
			nameBudget = 3
		}
		loc = trimMid(f.File, nameBudget) + lineStr
	}
	locCell := lipgloss.NewStyle().Width(locW).Render(loc)

	row := marker + icon + " " + locCell + " " + ref
	if selected {
		return m.styles.selectedRowFor(f.Tier).Width(width).Render(row)
	}
	return lipgloss.NewStyle().Foreground(m.styles.tierColor(f.Tier)).Render(row)
}

func (m Model) renderDetail(width int) string {
	f, ok := m.current()
	if !ok {
		return "no references"
	}
	snippet := m.sources.snippet(f.File, f.Line)
	kw := f.Keyword
	if kw == "" {
		kw = "—"
	}
	lines := []string{
		m.styles.tierBadge(f.Tier) + "  " + fmt.Sprintf("%s/%s#%d", f.Owner, f.Repo, f.Number),
		lipgloss.NewStyle().Bold(true).Render(f.Title),
		"",
		m.styles.detailKey.Render("state    ") + f.State,
		m.styles.detailKey.Render("file     ") + fmt.Sprintf("%s:%d", f.File, f.Line),
		m.styles.detailKey.Render("url      ") + issueURL(f),
		m.styles.detailKey.Render("keyword  ") + kw,
		m.styles.detailKey.Render("kind     ") + fmt.Sprintf("%s · %s", f.Kind, f.Confidence),
		"",
		m.styles.snippet.Render(snippet),
	}
	return strings.Join(lines, "\n")
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
