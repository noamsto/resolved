package tui

import "testing"

func TestThemeByName(t *testing.T) {
	for _, name := range []string{"mocha", "latte", "frappe", "macchiato"} {
		if _, err := ThemeByName(name); err != nil {
			t.Errorf("ThemeByName(%q) error: %v", name, err)
		}
	}
	if _, err := ThemeByName("nope"); err == nil {
		t.Fatal("expected error for unknown theme")
	}
}

func TestMochaColorsDistinct(t *testing.T) {
	m := Mocha()
	if m.Stale == nil || m.Open == nil || m.Accent == nil {
		t.Fatal("mocha colors must be non-nil")
	}
	if m.Stale == m.Open {
		t.Fatal("stale and open colors should differ")
	}
}
