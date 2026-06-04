package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/noamsto/resolved/internal/model"
)

func fixture() []model.Finding {
	return []model.Finding{
		{Reference: model.Reference{File: "z.go", Line: 9, Owner: "o", Repo: "r", Number: 3, Type: model.TypeIssue}, Status: model.Status{Title: "open one"}, Tier: model.TierOpen},
		{Reference: model.Reference{File: "a.go", Line: 2, Owner: "o", Repo: "r", Number: 1, Type: model.TypeIssue}, Status: model.Status{Title: "stale one"}, Tier: model.TierStale},
		{Reference: model.Reference{File: "m.go", Line: 5, Owner: "o", Repo: "r", Number: 2, Type: model.TypePR}, Status: model.Status{Title: "closed one"}, Tier: model.TierClosed},
	}
}

func TestNewSortsStaleFirst(t *testing.T) {
	m := New(fixture(), Deps{}, Mocha())
	if m.findings[0].Tier != model.TierStale {
		t.Fatalf("first finding tier = %v, want stale", m.findings[0].Tier)
	}
	if m.findings[1].Tier != model.TierClosed {
		t.Fatalf("second finding tier = %v, want closed", m.findings[1].Tier)
	}
}

func TestViewShowsLocationsAndCursor(t *testing.T) {
	m := New(fixture(), Deps{}, Mocha())
	out := m.View().Content
	if !strings.Contains(out, "a.go:2") {
		t.Fatalf("view missing a.go:2:\n%s", out)
	}
	if !strings.Contains(out, "❯") {
		t.Fatalf("view missing cursor marker:\n%s", out)
	}
}

func TestIssueURL(t *testing.T) {
	issue := model.Finding{Reference: model.Reference{Owner: "o", Repo: "r", Number: 7, Type: model.TypeIssue}}
	if got := issueURL(issue); got != "https://github.com/o/r/issues/7" {
		t.Fatalf("issue url = %q", got)
	}
	pr := model.Finding{Reference: model.Reference{Owner: "o", Repo: "r", Number: 8, Type: model.TypePR}}
	if got := issueURL(pr); got != "https://github.com/o/r/pull/8" {
		t.Fatalf("pr url = %q", got)
	}
}

func TestUpdateNavigation(t *testing.T) {
	m := New(fixture(), Deps{}, Mocha())

	down := tea.KeyPressMsg{Code: tea.KeyDown}
	nm, _ := m.Update(down)
	m = nm.(Model)
	if m.cursor != 1 {
		t.Fatalf("after down, cursor = %d, want 1", m.cursor)
	}

	nm, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = nm.(Model)
	if m.cursor != 2 {
		t.Fatalf("after j, cursor = %d, want 2", m.cursor)
	}

	// cannot go past the last item (3 findings -> max index 2)
	nm, _ = m.Update(down)
	m = nm.(Model)
	if m.cursor != 2 {
		t.Fatalf("cursor overran end: %d", m.cursor)
	}

	// up / k move back, clamped at 0
	nm, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m = nm.(Model)
	nm, _ = m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	m = nm.(Model)
	nm, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m = nm.(Model)
	if m.cursor != 0 {
		t.Fatalf("cursor underran start: %d", m.cursor)
	}
}

func TestUpdateQuit(t *testing.T) {
	m := New(fixture(), Deps{}, Mocha())
	nm, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = nm.(Model)
	if !m.quitting {
		t.Fatal("q should set quitting")
	}
	if cmd == nil {
		t.Fatal("q should return a quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q command should produce tea.QuitMsg")
	}
}

func TestEnterOpensIssueURL(t *testing.T) {
	var opened string
	m := New(fixture(), Deps{
		OpenURL: func(url string) error { opened = url; return nil },
	}, Mocha())
	// cursor at 0 -> stale finding o/r#1 (issue)
	nm, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = nm.(Model)
	if opened != "https://github.com/o/r/issues/1" {
		t.Fatalf("opened = %q", opened)
	}
	if m.status == "" {
		t.Fatal("expected a status message after opening")
	}
}

func TestEditInvokesEditorCmd(t *testing.T) {
	var gotFile string
	var gotLine int
	m := New(fixture(), Deps{
		EditorCmd: func(file string, line int) tea.Cmd {
			gotFile, gotLine = file, line
			return nil
		},
	}, Mocha())
	// cursor at 0 -> a.go:2 after sorting (stale finding)
	nm, _ := m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	_ = nm.(Model)
	if gotFile != "a.go" || gotLine != 2 {
		t.Fatalf("editor invoked with %s:%d, want a.go:2", gotFile, gotLine)
	}
}

func TestEditorDoneSetsErrorStatus(t *testing.T) {
	m := New(fixture(), Deps{}, Mocha())
	nm, _ := m.Update(editorDoneMsg{err: errTest})
	m = nm.(Model)
	if !strings.Contains(m.status, "editor") {
		t.Fatalf("status = %q, want editor error", m.status)
	}
}

var errTest = fmt.Errorf("boom")

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// strip removes ANSI color codes so substring assertions are color-independent.
func strip(s string) string { return ansiRe.ReplaceAllString(s, "") }

func TestTierBadgeLabels(t *testing.T) {
	st := newStyles(Mocha())
	cases := map[model.Tier]string{
		model.TierStale: "STALE", model.TierClosed: "closed", model.TierGone: "gone",
		model.TierOpen: "open", model.TierUnknown: "unknown",
	}
	for tier, want := range cases {
		if got := strip(st.tierBadge(tier)); !strings.Contains(got, want) {
			t.Errorf("tierBadge(%v) = %q, want contains %q", tier, got, want)
		}
	}
}

func TestRefreshReplacesFindings(t *testing.T) {
	fresh := []model.Finding{
		{Reference: model.Reference{File: "b.go", Line: 1, Owner: "o", Repo: "r", Number: 9}, Tier: model.TierOpen},
	}
	m := New(fixture(), Deps{
		Rescan: func() ([]model.Finding, error) { return fresh, nil },
	}, Mocha())

	// pressing r returns a command that performs the rescan
	nm, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = nm.(Model)
	if cmd == nil {
		t.Fatal("r should return a rescan command")
	}
	msg := cmd()
	done, ok := msg.(rescanDoneMsg)
	if !ok {
		t.Fatalf("rescan command produced %T, want rescanDoneMsg", msg)
	}

	// feeding the done message back replaces the findings
	nm, _ = m.Update(done)
	m = nm.(Model)
	if len(m.findings) != 1 || m.findings[0].Number != 9 {
		t.Fatalf("findings not replaced: %+v", m.findings)
	}
	if !strings.Contains(m.status, "refreshed") {
		t.Fatalf("status = %q, want refreshed", m.status)
	}
}

func TestRefreshErrorSetsStatus(t *testing.T) {
	m := New(fixture(), Deps{}, Mocha())
	nm, _ := m.Update(rescanDoneMsg{err: errTest})
	m = nm.(Model)
	if !strings.Contains(m.status, "refresh failed") {
		t.Fatalf("status = %q", m.status)
	}
}

func TestWindowSizeSetsDims(t *testing.T) {
	m := New(fixture(), Deps{}, Mocha())
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = nm.(Model)
	if m.width != 120 || m.height != 40 {
		t.Fatalf("dims = %dx%d, want 120x40", m.width, m.height)
	}
}

func TestViewRendersHeaderListDetail(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "demo.go")
	if err := os.WriteFile(p, []byte("package d\n// TODO drop once https://github.com/o/r/issues/1 ships\nfunc x(){}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := model.Finding{
		Reference: model.Reference{File: p, Line: 2, Owner: "o", Repo: "r", Number: 1, Type: model.TypeIssue, Keyword: "TODO"},
		Status:    model.Status{State: "closed", Title: "the bug"},
		Tier:      model.TierStale,
	}
	m := New([]model.Finding{f}, Deps{}, Mocha())
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = nm.(Model)

	out := strip(m.View().Content)
	for _, want := range []string{"1 ref", "STALE", "the bug", "o/r#1", "TODO drop once"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q in:\n%s", want, out)
		}
	}
}

func mkF(file string, line int, tier model.Tier, updated time.Time) model.Finding {
	return model.Finding{
		Reference: model.Reference{File: file, Line: line, Owner: "o", Repo: "r", Number: line},
		Status:    model.Status{UpdatedAt: updated},
		Tier:      tier,
	}
}

func TestSortTierThenRecency(t *testing.T) {
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	in := []model.Finding{
		mkF("a.go", 1, model.TierOpen, recent),
		mkF("b.go", 2, model.TierStale, old),
		mkF("c.go", 3, model.TierStale, recent),
	}
	out := sortFindings(in, modeTier)
	if out[0].Tier != model.TierStale || out[1].Tier != model.TierStale {
		t.Fatalf("stale should come first: %+v", out)
	}
	if !out[0].UpdatedAt.Equal(recent) {
		t.Fatalf("within stale, recent should precede old: %+v", out)
	}
}

func TestSortRecencyFlat(t *testing.T) {
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	in := []model.Finding{
		mkF("a.go", 1, model.TierStale, old),
		mkF("b.go", 2, model.TierOpen, recent),
	}
	out := sortFindings(in, modeRecency)
	if !out[0].UpdatedAt.Equal(recent) {
		t.Fatalf("recency mode: newest first regardless of tier: %+v", out)
	}
}

func TestSKeyCyclesMode(t *testing.T) {
	m := New(fixture(), Deps{}, Mocha())
	if m.mode != modeTier {
		t.Fatalf("default mode = %v, want tier", m.mode)
	}
	nm, _ := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = nm.(Model)
	if m.mode != modeFile {
		t.Fatalf("after s, mode = %v, want file", m.mode)
	}
	nm, _ = m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = nm.(Model)
	nm, _ = m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = nm.(Model)
	if m.mode != modeTier {
		t.Fatalf("after 3x s, mode = %v, want tier (cycled)", m.mode)
	}
}

func TestHeaderShowsGoneAndMode(t *testing.T) {
	m := New([]model.Finding{mkF("a.go", 1, model.TierGone, time.Time{})}, Deps{}, Mocha())
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = nm.(Model)
	out := strip(m.View().Content)
	if !strings.Contains(out, "1 gone") {
		t.Fatalf("header missing gone count:\n%s", out)
	}
	if !strings.Contains(out, "sort:") {
		t.Fatalf("header missing sort mode:\n%s", out)
	}
}

func TestListScrollFollowsCursor(t *testing.T) {
	var fs []model.Finding
	for i := 0; i < 30; i++ {
		fs = append(fs, model.Finding{
			Reference: model.Reference{File: "f.go", Line: i + 1, Owner: "o", Repo: "r", Number: i + 1},
			Tier:      model.TierOpen,
		})
	}
	m := New(fs, Deps{}, Mocha())
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 12})
	m = nm.(Model)

	for i := 0; i < 29; i++ {
		nm, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		m = nm.(Model)
	}
	if m.cursor != 29 {
		t.Fatalf("cursor = %d, want 29", m.cursor)
	}
	if m.listOffset == 0 {
		t.Fatal("listOffset should have advanced as the cursor moved past the viewport")
	}
	if m.cursor < m.listOffset || m.cursor >= m.listOffset+m.listHeight() {
		t.Fatalf("cursor %d not within visible window [%d, %d)", m.cursor, m.listOffset, m.listOffset+m.listHeight())
	}
}

func TestDisplayRowsGroupByFile(t *testing.T) {
	in := []model.Finding{
		mkF("a.go", 1, model.TierOpen, time.Time{}),
		mkF("a.go", 2, model.TierOpen, time.Time{}),
		mkF("b.go", 3, model.TierOpen, time.Time{}),
	}
	m := New(in, Deps{}, Mocha())
	m.mode = modeFile
	m.findings = sortFindings(in, modeFile)

	rows := m.displayRows()
	headers := 0
	for _, r := range rows {
		if r.header {
			headers++
		}
	}
	if headers != 2 {
		t.Fatalf("want 2 file headers, got %d (rows=%+v)", headers, rows)
	}
	if len(rows) != 5 { // 2 headers + 3 findings
		t.Fatalf("want 5 display rows, got %d", len(rows))
	}
}

func TestDisplayRowsFlatInTierMode(t *testing.T) {
	m := New(fixture(), Deps{}, Mocha()) // modeTier
	rows := m.displayRows()
	if len(rows) != len(m.findings) {
		t.Fatalf("tier mode should have no headers: %d rows vs %d findings", len(rows), len(m.findings))
	}
	for _, r := range rows {
		if r.header {
			t.Fatal("tier mode should emit no header rows")
		}
	}
}

func TestCursorRowMapsThroughHeaders(t *testing.T) {
	in := []model.Finding{
		mkF("a.go", 1, model.TierOpen, time.Time{}),
		mkF("b.go", 2, model.TierOpen, time.Time{}),
	}
	m := New(in, Deps{}, Mocha())
	m.mode = modeFile
	m.findings = sortFindings(in, modeFile)
	m.cursor = 1 // rows: [hdr a.go][a#1][hdr b.go][b#2] -> finding idx 1 is display row 3
	if cr := m.cursorRow(); cr != 3 {
		t.Fatalf("cursorRow = %d, want 3", cr)
	}
}

func TestListColumnsAlign(t *testing.T) {
	in := []model.Finding{
		mkF("short.go", 1, model.TierOpen, time.Time{}),
		mkF("a/very/long/path/to/file.go", 2, model.TierOpen, time.Time{}),
	}
	m := New(in, Deps{}, Mocha())
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	m = nm.(Model)

	r0 := strip(m.renderFindingRow(m.findings[0], false, 60))
	r1 := strip(m.renderFindingRow(m.findings[1], false, 60))
	off0 := strings.Index(r0, "o/r#")
	off1 := strings.Index(r1, "o/r#")
	if off0 < 0 || off1 < 0 {
		t.Fatalf("ref column missing: %q / %q", r0, r1)
	}
	if off0 != off1 {
		t.Fatalf("ref column not aligned: offset %d vs %d\n%q\n%q", off0, off1, r0, r1)
	}
}

func TestFilenameWidthScalesWithPane(t *testing.T) {
	f := mkF("a/very/long/path/to/some/file.go", 1, model.TierOpen, time.Time{})
	m := New([]model.Finding{f}, Deps{}, Mocha())
	narrow := strip(m.renderFindingRow(f, false, 40))
	wide := strip(m.renderFindingRow(f, false, 100))
	if len(strings.TrimSpace(wide)) <= len(strings.TrimSpace(narrow)) {
		t.Fatalf("wider pane should show a longer row:\nnarrow=%q\nwide=%q", narrow, wide)
	}
}

func TestFindingRowFitsWidth(t *testing.T) {
	f := mkF("a/very/long/path/that/would/overflow/file.go", 123, model.TierStale, time.Time{})
	m := New([]model.Finding{f}, Deps{}, Mocha())
	row := m.renderFindingRow(f, false, 40)
	if w := lipgloss.Width(row); w != 40 {
		t.Fatalf("row display width = %d, want exactly 40 (no overflow/underflow)", w)
	}
}

func TestListRowsDoNotWrap(t *testing.T) {
	in := []model.Finding{
		mkF("/tmp/resolved-demo/demo.go", 3, model.TierStale, time.Time{}),
		mkF("/tmp/resolved-demo/other.go", 3, model.TierStale, time.Time{}),
	}
	m := New(in, Deps{}, Mocha())
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 130, Height: 22})
	m = nm.(Model)

	out := strip(m.View().Content)
	for _, line := range strings.Split(out, "\n") {
		// A list line that contains a ref ("o/r#") must also contain a tier
		// icon on the SAME line; otherwise the row wrapped.
		if strings.Contains(line, "o/r#") {
			if !strings.ContainsAny(line, "◆●○✗?") {
				t.Fatalf("ref wrapped onto its own line (no tier icon): %q", line)
			}
		}
	}
}

func TestSelectedRowKeepsTierColor(t *testing.T) {
	st := newStyles(Mocha())
	if st.selectedRowFor(model.TierStale).GetForeground() != Mocha().Stale {
		t.Fatal("selected stale row should use the stale tier color as foreground")
	}
	if st.selectedRowFor(model.TierOpen).GetForeground() != Mocha().Open {
		t.Fatal("selected open row should use the open tier color as foreground")
	}
	// background is preserved so the row still reads as highlighted
	if st.selectedRowFor(model.TierStale).GetBackground() != Mocha().SelBg {
		t.Fatal("selected row should keep the selection background")
	}
}
