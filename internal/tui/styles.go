package tui

import (
	"image/color"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/noamsto/resolved/internal/model"
)

// Styles is the set of lipgloss styles derived from a Theme.
type Styles struct {
	theme       Theme
	header      lipgloss.Style
	footer      lipgloss.Style
	pane        lipgloss.Style
	selectedRow lipgloss.Style
	detailKey   lipgloss.Style
	snippet     lipgloss.Style
	fileHeader  lipgloss.Style
}

func newStyles(t Theme) Styles {
	return Styles{
		theme:       t,
		header:      lipgloss.NewStyle().Bold(true).Foreground(t.Accent).Padding(0, 1),
		footer:      lipgloss.NewStyle().Foreground(t.Muted).Padding(0, 1),
		pane:        lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(t.Accent).Padding(0, 1),
		selectedRow: lipgloss.NewStyle().Bold(true).Foreground(t.SelFg).Background(t.SelBg),
		detailKey:   lipgloss.NewStyle().Foreground(t.Muted),
		snippet:     lipgloss.NewStyle().Foreground(t.Snippet).Italic(true),
		fileHeader:  lipgloss.NewStyle().Bold(true).Foreground(t.Accent),
	}
}

// tierMeta returns the badge icon and label for a tier (color comes from Styles).
func tierMeta(t model.Tier) (icon, label string) {
	switch t {
	case model.TierStale:
		return "◆", "STALE"
	case model.TierClosed:
		return "●", "closed"
	case model.TierGone:
		return "✗", "gone"
	case model.TierOpen:
		return "○", "open"
	default:
		return "?", "unknown"
	}
}

// tierColor maps a tier to its themed color.
func (s Styles) tierColor(t model.Tier) color.Color {
	switch t {
	case model.TierStale:
		return s.theme.Stale
	case model.TierClosed:
		return s.theme.Closed
	case model.TierGone:
		return s.theme.Gone
	case model.TierOpen:
		return s.theme.Open
	default:
		return s.theme.Unknown
	}
}

// tierBadge renders a themed "<icon> <LABEL>" badge.
func (s Styles) tierBadge(t model.Tier) string {
	icon, label := tierMeta(t)
	return lipgloss.NewStyle().Foreground(s.tierColor(t)).Render(icon + " " + label)
}

// selectedRowFor is the selected-row style for a tier: it keeps the selection
// background (and bold) but colors the text with the tier color, so a
// highlighted row still conveys its tier instead of going flat.
func (s Styles) selectedRowFor(t model.Tier) lipgloss.Style {
	return s.selectedRow.Foreground(s.tierColor(t))
}
