package tui

import (
	"image/color"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/noamsto/resolved/internal/model"
)

// Palette (ANSI 256 codes; lipgloss degrades to no-color on non-color terminals).
var (
	colStale   = lipgloss.Color("203") // red
	colClosed  = lipgloss.Color("245") // gray
	colOpen    = lipgloss.Color("78")  // green
	colGone    = lipgloss.Color("171") // magenta
	colUnknown = lipgloss.Color("240") // faint
	colAccent  = lipgloss.Color("63")  // borders / header
	colMuted   = lipgloss.Color("245")
)

var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(colAccent).Padding(0, 1)
	footerStyle = lipgloss.NewStyle().Foreground(colMuted).Padding(0, 1)
	paneStyle   = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(colAccent).
			Padding(0, 1)
	selectedRowStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(colAccent)
	detailKeyStyle   = lipgloss.NewStyle().Foreground(colMuted)
	snippetStyle     = lipgloss.NewStyle().Foreground(colMuted).Italic(true)
)

// tierMeta maps a tier to its badge icon, label, and color.
func tierMeta(t model.Tier) (icon, label string, c color.Color) {
	switch t {
	case model.TierStale:
		return "◆", "STALE", colStale
	case model.TierClosed:
		return "●", "closed", colClosed
	case model.TierGone:
		return "✗", "gone", colGone
	case model.TierOpen:
		return "○", "open", colOpen
	default:
		return "?", "unknown", colUnknown
	}
}

// tierBadge renders a colored "<icon> <LABEL>" badge for a tier.
func tierBadge(t model.Tier) string {
	icon, label, c := tierMeta(t)
	return lipgloss.NewStyle().Foreground(c).Render(icon + " " + label)
}
