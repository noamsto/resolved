package tui

import (
	"fmt"
	"sort"

	tea "charm.land/bubbletea/v2"
	"github.com/noamsto/resolved/internal/model"
)

type sortMode int

const (
	modeTier sortMode = iota
	modeFile
	modeRecency
)

func (s sortMode) label() string {
	switch s {
	case modeFile:
		return "by file"
	case modeRecency:
		return "recency"
	default:
		return "tier"
	}
}

// Deps are the injectable side-effects (real ones wired by the explore command;
// stubbed in tests).
type Deps struct {
	OpenURL   func(url string) error              // open an issue/PR in the browser
	EditorCmd func(file string, line int) tea.Cmd // open a source line in $EDITOR
	Rescan    func() ([]model.Finding, error)     // re-run the scan
	Root      string                              // scan base dir; list paths render relative to it (empty => ~-collapsed absolute)
}

// editorDoneMsg is delivered when the external editor process exits.
type editorDoneMsg struct{ err error }

// EditorDone wraps an editor-process exit error into the model's done message.
func EditorDone(err error) tea.Msg { return editorDoneMsg{err: err} }

// rescanDoneMsg carries the result of an async rescan.
type rescanDoneMsg struct {
	findings []model.Finding
	err      error
}

// Model is the Bubble Tea model for the explore TUI.
type Model struct {
	findings   []model.Finding
	cursor     int
	status     string
	deps       Deps
	quitting   bool
	width      int
	height     int
	listOffset int
	sources    *sourceCache
	styles     Styles
	theme      Theme
	mode       sortMode
}

// New builds a Model with findings sorted by tier (stale first).
func New(findings []model.Finding, deps Deps, theme Theme) Model {
	return Model{
		findings: sortFindings(findings, modeTier),
		deps:     deps,
		sources:  newSourceCache(),
		theme:    theme,
		styles:   newStyles(theme),
	}
}

func (m Model) Init() tea.Cmd { return nil }

// Update handles key and async messages. Side-effects go through m.deps.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
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
		case "s":
			m.mode = (m.mode + 1) % 3
			m.findings = sortFindings(m.findings, m.mode)
			m.cursor = 0
			m.listOffset = 0
		}
	case rescanDoneMsg:
		if msg.err != nil {
			m.status = "refresh failed: " + msg.err.Error()
		} else {
			m.findings = sortFindings(msg.findings, m.mode)
			if m.cursor >= len(m.findings) {
				m.cursor = max(0, len(m.findings)-1)
			}
			m.status = fmt.Sprintf("refreshed — %d refs", len(m.findings))
		}
	case editorDoneMsg:
		if msg.err != nil {
			m.status = "editor: " + msg.err.Error()
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.clampScroll()
	}
	m.clampScroll()
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

func locLess(a, b model.Finding) bool {
	if a.File != b.File {
		return a.File < b.File
	}
	return a.Line < b.Line
}

// sortFindings returns a copy ordered per mode.
func sortFindings(in []model.Finding, mode sortMode) []model.Finding {
	out := make([]model.Finding, len(in))
	copy(out, in)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		switch mode {
		case modeRecency:
			if !a.UpdatedAt.Equal(b.UpdatedAt) {
				return a.UpdatedAt.After(b.UpdatedAt)
			}
			return locLess(a, b)
		case modeFile:
			if a.File != b.File {
				return a.File < b.File
			}
			if ra, rb := tierRank(a.Tier), tierRank(b.Tier); ra != rb {
				return ra < rb
			}
			if !a.UpdatedAt.Equal(b.UpdatedAt) {
				return a.UpdatedAt.After(b.UpdatedAt)
			}
			return a.Line < b.Line
		default: // modeTier
			if ra, rb := tierRank(a.Tier), tierRank(b.Tier); ra != rb {
				return ra < rb
			}
			if !a.UpdatedAt.Equal(b.UpdatedAt) {
				return a.UpdatedAt.After(b.UpdatedAt)
			}
			return locLess(a, b)
		}
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

// render produces the styled TUI output.
func (m Model) render() string {
	if m.quitting {
		return ""
	}
	return m.renderAll()
}

// View renders the model. AltScreen is declared on the View in v2.
func (m Model) View() tea.View {
	var v tea.View
	v.SetContent(m.render())
	v.AltScreen = true
	return v
}

// listHeight is the number of finding rows visible in the list pane, derived
// from terminal height minus header+footer chrome; a sane default applies
// before the first WindowSizeMsg.
func (m Model) listHeight() int {
	h := m.height
	if h <= 0 {
		h = 24
	}
	rows := h - 5
	if rows < 1 {
		rows = 1
	}
	return rows
}

// displayRow is one rendered list line: either a file-group header or a finding.
type displayRow struct {
	header bool
	text   string // header: the file path
	idx    int    // finding row: index into m.findings
}

// displayRows builds the list rows. In modeFile a header precedes each file
// group; other modes are a flat finding list.
func (m Model) displayRows() []displayRow {
	if m.mode != modeFile {
		rows := make([]displayRow, len(m.findings))
		for i := range m.findings {
			rows[i] = displayRow{idx: i}
		}
		return rows
	}
	var rows []displayRow
	last := ""
	for i, f := range m.findings {
		if f.File != last {
			rows = append(rows, displayRow{header: true, text: f.File})
			last = f.File
		}
		rows = append(rows, displayRow{idx: i})
	}
	return rows
}

// cursorRow is the display-row index of the currently selected finding.
func (m Model) cursorRow() int {
	for ri, r := range m.displayRows() {
		if !r.header && r.idx == m.cursor {
			return ri
		}
	}
	return 0
}

// clampScroll keeps listOffset so the cursor stays within the visible window.
func (m *Model) clampScroll() {
	vh := m.listHeight()
	cr := m.cursorRow()
	if cr < m.listOffset {
		m.listOffset = cr
	}
	if cr >= m.listOffset+vh {
		m.listOffset = cr - vh + 1
	}
	if m.listOffset < 0 {
		m.listOffset = 0
	}
}
