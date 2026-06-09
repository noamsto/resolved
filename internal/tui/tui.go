package tui

import (
	"fmt"
	"sort"
	"time"

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

	// Scan and Resolve drive the progressive startup load: Scan does the fast
	// local pass (refs, unresolved), then Resolve streams status batches over a
	// channel it closes when done. When Scan is nil, New uses the findings it's
	// given directly (the path tests take).
	Scan    func() ([]model.Finding, error)
	Resolve func(findings []model.Finding) <-chan StatusBatch
}

// StatusBatch is one increment of resolved statuses streamed during loading.
// Done/Total count unique issues, for the progress readout.
type StatusBatch struct {
	Statuses map[string]model.Status
	Done     int
	Total    int
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

// Progressive-load messages: scannedMsg paints the local refs, statusBatchMsg
// fills a chunk of resolved statuses, resolveDoneMsg ends loading, spinTickMsg
// advances the spinner.
type scannedMsg struct {
	findings []model.Finding
	err      error
}
type statusBatchMsg StatusBatch
type resolveDoneMsg struct{}
type spinTickMsg struct{}

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

	loading      bool
	spinFrame    int
	resolveDone  int
	resolveTotal int
	batches      <-chan StatusBatch

	detailScroll int // horizontal column offset for the detail/preview pane
}

const detailScrollStep = 8

var spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// New builds a Model with findings sorted by tier (stale first). With no
// findings and a Scan dep, it starts in the loading state and fetches on Init.
func New(findings []model.Finding, deps Deps, theme Theme) Model {
	return Model{
		findings: sortFindings(findings, modeTier),
		deps:     deps,
		sources:  newSourceCache(),
		theme:    theme,
		styles:   newStyles(theme),
		loading:  len(findings) == 0 && deps.Scan != nil,
	}
}

func (m Model) Init() tea.Cmd {
	if !m.loading {
		return nil
	}
	return tea.Batch(m.scanCmd(), m.spinCmd())
}

func (m Model) scanCmd() tea.Cmd {
	return func() tea.Msg {
		fs, err := m.deps.Scan()
		return scannedMsg{findings: fs, err: err}
	}
}

func (m Model) spinCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return spinTickMsg{} })
}

// waitBatch reads the next streamed status batch; a closed channel ends loading.
func (m Model) waitBatch() tea.Cmd {
	ch := m.batches
	return func() tea.Msg {
		b, ok := <-ch
		if !ok {
			return resolveDoneMsg{}
		}
		return statusBatchMsg(b)
	}
}

// applyStatuses fills resolved statuses into matching findings and reclassifies
// their tier. A bare #n that resolves to "gone" never named a real issue, so it
// is dropped (mirrors engine.Run).
func applyStatuses(findings []model.Finding, statuses map[string]model.Status) []model.Finding {
	out := findings[:0]
	for _, f := range findings {
		if st, ok := statuses[f.Key()]; ok {
			f.Status = st
			f.Tier = model.ClassifyTier(st.State, f.Keyword)
			if f.Kind == model.KindBare && st.State == "gone" {
				continue
			}
		}
		out = append(out, f)
	}
	return out
}

func uniqueKeys(findings []model.Finding) int {
	seen := make(map[string]struct{}, len(findings))
	for _, f := range findings {
		seen[f.Key()] = struct{}{}
	}
	return len(seen)
}

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
			m.detailScroll = 0 // a new finding's preview starts at the left
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			m.detailScroll = 0
		case "alt+l", "alt+right":
			m.detailScroll += detailScrollStep
		case "alt+h", "alt+left":
			m.detailScroll = max(0, m.detailScroll-detailScrollStep)
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
		case "y":
			if f, ok := m.current(); ok {
				text := fmt.Sprintf("%s:%d", f.File, f.Line)
				m.status = "yanked " + text
				return m, tea.SetClipboard(text)
			}
		case "Y":
			if f, ok := m.current(); ok {
				url := issueURL(f)
				m.status = "yanked " + url
				return m, tea.SetClipboard(url)
			}
		}
	case scannedMsg:
		if msg.err != nil {
			m.loading = false
			m.status = "scan failed: " + msg.err.Error()
			return m, nil
		}
		m.findings = sortFindings(msg.findings, m.mode)
		if m.deps.Resolve != nil && len(m.findings) > 0 {
			m.resolveTotal = uniqueKeys(m.findings)
			m.batches = m.deps.Resolve(m.findings)
			return m, m.waitBatch()
		}
		m.loading = false
	case statusBatchMsg:
		m.findings = applyStatuses(m.findings, msg.Statuses)
		if m.cursor >= len(m.findings) {
			m.cursor = max(0, len(m.findings)-1)
		}
		m.resolveDone = msg.Done
		return m, m.waitBatch()
	case resolveDoneMsg:
		m.loading = false
		m.findings = sortFindings(m.findings, m.mode)
		if m.cursor >= len(m.findings) {
			m.cursor = max(0, len(m.findings)-1)
		}
		m.status = fmt.Sprintf("%d refs", len(m.findings))
	case spinTickMsg:
		if m.loading {
			m.spinFrame++
			return m, m.spinCmd()
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

// listHeight is the number of finding rows visible in the list pane: terminal
// height minus header (1), footer (1), and the pane border (2) — lipgloss
// Width/Height include the border. A sane default applies before the first
// WindowSizeMsg.
func (m Model) listHeight() int {
	h := m.height
	if h <= 0 {
		h = 24
	}
	rows := h - 4
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
