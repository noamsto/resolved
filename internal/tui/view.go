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
	body := lipgloss.JoinHorizontal(
		lipgloss.Top,
		paneStyle.Width(listW).Height(ph).Render(m.renderList(listW)),
		paneStyle.Width(detailW).Height(ph).Render(m.renderDetail(detailW)),
	)

	return strings.Join([]string{header, body, footer}, "\n")
}

func (m Model) renderHeader(width int) string {
	s := m.summary()
	line := fmt.Sprintf("resolved · %d refs · %d stale · %d closed · %d open",
		len(m.findings), s.stale, s.closed, s.open)
	return headerStyle.Width(width).Render(line)
}

func (m Model) renderFooter(width int) string {
	help := "j/k move · ⏎ open · e edit · r refresh · q quit"
	if m.status != "" {
		help = m.status + "   " + help
	}
	return footerStyle.Width(width).Render(help)
}

func (m Model) renderList(width int) string {
	if len(m.findings) == 0 {
		return "no references found"
	}
	vh := m.listHeight()
	end := m.listOffset + vh
	if end > len(m.findings) {
		end = len(m.findings)
	}
	var b strings.Builder
	for i := m.listOffset; i < end; i++ {
		f := m.findings[i]
		icon, _, c := tierMeta(f.Tier)
		row := fmt.Sprintf("%s %s:%d  %s#%d",
			icon, trimMid(f.File, 18), f.Line, f.Owner+"/"+f.Repo, f.Number)
		if i == m.cursor {
			b.WriteString(selectedRowStyle.Width(width).Render("❯ " + row))
		} else {
			b.WriteString(lipgloss.NewStyle().Foreground(c).Render("  " + row))
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
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
		tierBadge(f.Tier) + "  " + fmt.Sprintf("%s/%s#%d", f.Owner, f.Repo, f.Number),
		lipgloss.NewStyle().Bold(true).Render(f.Title),
		"",
		detailKeyStyle.Render("state    ") + f.State,
		detailKeyStyle.Render("file     ") + fmt.Sprintf("%s:%d", f.File, f.Line),
		detailKeyStyle.Render("url      ") + issueURL(f),
		detailKeyStyle.Render("keyword  ") + kw,
		detailKeyStyle.Render("kind     ") + fmt.Sprintf("%s · %s", f.Kind, f.Confidence),
		"",
		snippetStyle.Render(snippet),
	}
	return strings.Join(lines, "\n")
}

type tierCounts struct{ stale, closed, open int }

func (m Model) summary() tierCounts {
	var c tierCounts
	for _, f := range m.findings {
		switch f.Tier {
		case model.TierStale:
			c.stale++
		case model.TierClosed:
			c.closed++
		case model.TierOpen:
			c.open++
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
