package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/noamsto/resolved/internal/model"
)

// Deps are the injectable side-effects (real ones wired by the explore command;
// stubbed in tests).
type Deps struct {
	OpenURL   func(url string) error              // open an issue/PR in the browser
	EditorCmd func(file string, line int) tea.Cmd // open a source line in $EDITOR
	Rescan    func() ([]model.Finding, error)     // re-run the scan
}

// editorDoneMsg is delivered when the external editor process exits.
type editorDoneMsg struct{ err error }

// rescanDoneMsg carries the result of an async rescan.
type rescanDoneMsg struct {
	findings []model.Finding
	err      error
}

// Model is the Bubble Tea model for the explore TUI.
type Model struct {
	findings []model.Finding
	cursor   int
	status   string
	deps     Deps
	quitting bool
}

// New builds a Model with findings sorted by tier (stale first).
func New(findings []model.Finding, deps Deps) Model {
	return Model{findings: sortFindings(findings), deps: deps}
}

func (m Model) Init() tea.Cmd { return nil }

// Update handles key and async messages. Side-effects go through m.deps.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "down", "j":
			if m.cursor < len(m.findings)-1 {
				m.cursor++
			}
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "enter":
			if f, ok := m.current(); ok && m.deps.OpenURL != nil {
				url := issueURL(f)
				if err := m.deps.OpenURL(url); err != nil {
					m.status = "open url: " + err.Error()
				} else {
					m.status = "opened " + url
				}
			}
		case "e":
			if f, ok := m.current(); ok && m.deps.EditorCmd != nil {
				m.status = fmt.Sprintf("opening %s:%d", f.File, f.Line)
				return m, m.deps.EditorCmd(f.File, f.Line)
			}
		case "r":
			if m.deps.Rescan != nil {
				m.status = "refreshing…"
				return m, m.rescanCmd()
			}
		}
	case rescanDoneMsg:
		if msg.err != nil {
			m.status = "refresh failed: " + msg.err.Error()
		} else {
			m.findings = sortFindings(msg.findings)
			if m.cursor >= len(m.findings) {
				m.cursor = max(0, len(m.findings)-1)
			}
			m.status = fmt.Sprintf("refreshed — %d refs", len(m.findings))
		}
	case editorDoneMsg:
		if msg.err != nil {
			m.status = "editor: " + msg.err.Error()
		}
	}
	return m, nil
}

func tierRank(t model.Tier) int {
	switch t {
	case model.TierStale:
		return 0
	case model.TierClosed:
		return 1
	case model.TierGone:
		return 2
	case model.TierOpen:
		return 3
	default:
		return 4
	}
}

// sortFindings orders by tier (stale first), then file, then line.
func sortFindings(in []model.Finding) []model.Finding {
	out := make([]model.Finding, len(in))
	copy(out, in)
	sort.SliceStable(out, func(i, j int) bool {
		if ri, rj := tierRank(out[i].Tier), tierRank(out[j].Tier); ri != rj {
			return ri < rj
		}
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	return out
}

func (m Model) rescanCmd() tea.Cmd {
	return func() tea.Msg {
		fs, err := m.deps.Rescan()
		return rescanDoneMsg{findings: fs, err: err}
	}
}

func (m Model) current() (model.Finding, bool) {
	if m.cursor < 0 || m.cursor >= len(m.findings) {
		return model.Finding{}, false
	}
	return m.findings[m.cursor], true
}

func issueURL(f model.Finding) string {
	kind := "issues"
	if f.Type == model.TypePR {
		kind = "pull"
	}
	return fmt.Sprintf("https://github.com/%s/%s/%s/%d", f.Owner, f.Repo, kind, f.Number)
}

// View renders the finding list with a cursor, plus a help/status footer.
func (m Model) View() string {
	if m.quitting {
		return ""
	}
	var b strings.Builder
	b.WriteString("resolved — explore  (j/k move · enter open issue · e edit · r refresh · q quit)\n\n")
	if len(m.findings) == 0 {
		b.WriteString("  no references found\n")
	}
	for i, f := range m.findings {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		fmt.Fprintf(&b, "%s[%s] %s:%d  %s/%s#%d  %s\n",
			cursor, f.Tier.String(), f.File, f.Line, f.Owner, f.Repo, f.Number, f.Title)
	}
	if m.status != "" {
		fmt.Fprintf(&b, "\n%s\n", m.status)
	}
	return b.String()
}
