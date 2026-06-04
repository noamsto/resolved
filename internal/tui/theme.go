package tui

import (
	"fmt"
	"image/color"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
)

// Theme holds the semantic colors the TUI styles consume.
type Theme struct {
	Accent  color.Color // borders, header, file-group headers
	Text    color.Color
	Muted   color.Color // footer, detail keys
	SelBg   color.Color // selected row background
	SelFg   color.Color // selected row foreground
	Snippet color.Color // detail source line
	Stale   color.Color
	Closed  color.Color
	Gone    color.Color
	Open    color.Color
	Unknown color.Color
}

func c(hex string) color.Color { return lipgloss.Color(hex) }

// Mocha is the default (dark) Catppuccin flavor.
func Mocha() Theme {
	return Theme{
		Accent: c("#cba6f7"), Text: c("#cdd6f4"), Muted: c("#6c7086"),
		SelBg: c("#45475a"), SelFg: c("#cdd6f4"), Snippet: c("#a6adc8"),
		Stale: c("#f38ba8"), Closed: c("#6c7086"), Gone: c("#fab387"),
		Open: c("#a6e3a1"), Unknown: c("#585b70"),
	}
}

// Latte is the light Catppuccin flavor.
func Latte() Theme {
	return Theme{
		Accent: c("#8839ef"), Text: c("#4c4f69"), Muted: c("#9ca0b0"),
		SelBg: c("#bcc0cc"), SelFg: c("#4c4f69"), Snippet: c("#6c6f85"),
		Stale: c("#d20f39"), Closed: c("#9ca0b0"), Gone: c("#fe640b"),
		Open: c("#40a02b"), Unknown: c("#acb0be"),
	}
}

func Frappe() Theme {
	return Theme{
		Accent: c("#ca9ee6"), Text: c("#c6d0f5"), Muted: c("#737994"),
		SelBg: c("#51576d"), SelFg: c("#c6d0f5"), Snippet: c("#a5adce"),
		Stale: c("#e78284"), Closed: c("#737994"), Gone: c("#ef9f76"),
		Open: c("#a6d189"), Unknown: c("#626880"),
	}
}

func Macchiato() Theme {
	return Theme{
		Accent: c("#c6a4f7"), Text: c("#cad3f5"), Muted: c("#6e738d"),
		SelBg: c("#494d64"), SelFg: c("#cad3f5"), Snippet: c("#a5adcb"),
		Stale: c("#ed8796"), Closed: c("#6e738d"), Gone: c("#f5a97f"),
		Open: c("#a6da95"), Unknown: c("#5b6078"),
	}
}

// ThemeByName resolves a flavor name (case-insensitive).
func ThemeByName(name string) (Theme, error) {
	switch strings.ToLower(name) {
	case "mocha":
		return Mocha(), nil
	case "latte":
		return Latte(), nil
	case "frappe", "frappé":
		return Frappe(), nil
	case "macchiato":
		return Macchiato(), nil
	default:
		return Theme{}, fmt.Errorf("unknown theme %q: must be mocha|latte|frappe|macchiato", name)
	}
}
