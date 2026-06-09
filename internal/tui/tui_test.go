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

	locW := m.locColWidth(60)
	r0 := strip(m.renderFindingRow(m.findings[0], false, locW, 60))
	r1 := strip(m.renderFindingRow(m.findings[1], false, locW, 60))
	off0 := strings.Index(r0, "o/r#")
	off1 := strings.Index(r1, "o/r#")
	if off0 < 0 || off1 < 0 {
		t.Fatalf("ref column missing: %q / %q", r0, r1)
	}
	if off0 != off1 {
		t.Fatalf("ref column not aligned: offset %d vs %d\n%q\n%q", off0, off1, r0, r1)
	}
}

func TestApplyStatusesDropsBareGone(t *testing.T) {
	bare := model.Finding{Reference: model.Reference{Owner: "o", Repo: "r", Number: 9, Kind: model.KindBare}}
	url := model.Finding{Reference: model.Reference{Owner: "o", Repo: "r", Number: 8, Kind: model.KindURL}}
	out := applyStatuses([]model.Finding{bare, url}, map[string]model.Status{
		"o/r#9": {State: "gone"},
		"o/r#8": {State: "gone"},
	})
	if len(out) != 1 || out[0].Number != 8 {
		t.Fatalf("bare gone should drop, explicit gone should stay: %+v", out)
	}
	if out[0].Tier != model.TierGone {
		t.Fatalf("resolved finding should reclassify: %+v", out[0])
	}
}

func TestProgressiveLoadFillsStatuses(t *testing.T) {
	f := model.Finding{
		Reference: model.Reference{File: "a.go", Line: 2, Owner: "o", Repo: "r", Number: 1, Type: model.TypeIssue, Keyword: "TODO"},
		Tier:      model.TierUnknown,
	}
	ch := make(chan StatusBatch, 1)
	close(ch) // handler stores it but we drive messages manually
	deps := Deps{
		Scan:    func() ([]model.Finding, error) { return []model.Finding{f}, nil },
		Resolve: func([]model.Finding) <-chan StatusBatch { return ch },
	}
	m := New(nil, deps, Mocha())
	if !m.loading {
		t.Fatal("expected loading state at startup when Scan is provided")
	}

	nm, _ := m.Update(scannedMsg{findings: []model.Finding{f}})
	m = nm.(Model)
	if len(m.findings) != 1 || m.findings[0].Tier != model.TierUnknown {
		t.Fatalf("scan should paint unresolved refs: %+v", m.findings)
	}

	nm, _ = m.Update(statusBatchMsg{
		Statuses: map[string]model.Status{"o/r#1": {State: "closed", Title: "bug"}},
		Done:     1, Total: 1,
	})
	m = nm.(Model)
	if m.findings[0].Title != "bug" || m.findings[0].Tier != model.TierStale {
		t.Fatalf("batch should fill status + reclassify: %+v", m.findings[0])
	}

	nm, _ = m.Update(resolveDoneMsg{})
	m = nm.(Model)
	if m.loading {
		t.Fatal("loading should end after resolveDone")
	}
}

func TestLoadingViewShowsSpinnerAndProgress(t *testing.T) {
	deps := Deps{Scan: func() ([]model.Finding, error) { return nil, nil }}
	m := New(nil, deps, Mocha())
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m = nm.(Model)
	if !m.loading {
		t.Fatal("expected loading state")
	}

	// Before the scan returns: empty list + footer say "scanning…".
	out := strip(m.View().Content)
	if !strings.Contains(out, "scanning…") {
		t.Fatalf("loading view missing 'scanning…':\n%s", out)
	}

	// Refs painted, statuses resolving: footer shows the N/M counter and the
	// unresolved row shows the "…" state placeholder.
	m.findings = []model.Finding{{
		Reference: model.Reference{File: "a.go", Line: 1, Owner: "o", Repo: "r", Number: 1},
		Tier:      model.TierUnknown,
	}}
	m.resolveTotal = 3
	m.resolveDone = 1
	out = strip(m.View().Content)
	if !strings.Contains(out, "resolving 1/3 issues…") {
		t.Fatalf("loading view missing progress counter:\n%s", out)
	}
	if !strings.Contains(out, "o/r#1") {
		t.Fatalf("refs should be painted while statuses resolve:\n%s", out)
	}
}

func TestFindingRowUnresolvedShowsPlaceholder(t *testing.T) {
	f := model.Finding{
		Reference: model.Reference{File: "a.go", Line: 1, Owner: "o", Repo: "r", Number: 1},
		Tier:      model.TierUnknown, // no status yet
	}
	m := New([]model.Finding{f}, Deps{}, Mocha())
	row := strip(m.renderFindingRow(f, false, m.locColWidth(80), 80))
	if !strings.Contains(row, "…") {
		t.Fatalf("unresolved row should show '…' state placeholder: %q", row)
	}
}

func TestFindingRowShowsStateAndTitle(t *testing.T) {
	f := model.Finding{
		Reference: model.Reference{File: "a.go", Line: 2, Owner: "o", Repo: "r", Number: 1, Type: model.TypeIssue},
		Status:    model.Status{State: "closed", Title: "the bug title"},
		Tier:      model.TierClosed,
	}
	m := New([]model.Finding{f}, Deps{}, Mocha())
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = nm.(Model)
	row := strip(m.renderFindingRow(f, false, m.locColWidth(120), 120))
	for _, want := range []string{"closed", "o/r#1", "the bug title"} {
		if !strings.Contains(row, want) {
			t.Fatalf("row missing %q in: %q", want, row)
		}
	}
	if strings.Contains(row, "[") {
		t.Fatalf("state should render without brackets: %q", row)
	}
}

func TestFindingRowShowsParentDirFile(t *testing.T) {
	f := model.Finding{
		Reference: model.Reference{File: "internal/net/http/handler.go", Line: 51, Owner: "o", Repo: "r", Number: 1},
		Status:    model.Status{State: "open"},
		Tier:      model.TierOpen,
	}
	m := New([]model.Finding{f}, Deps{}, Mocha()) // default sort is tier (flat)
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	m = nm.(Model)
	row := strip(m.renderFindingRow(f, false, m.locColWidth(140), 140))
	if !strings.Contains(row, "http/handler.go:51") {
		t.Fatalf("row should show parent_dir/file: %q", row)
	}
	if strings.Contains(row, "internal/net/http") {
		t.Fatalf("row should not show the full path: %q", row)
	}
}

func TestRowShortLocIsRootRelative(t *testing.T) {
	deep := model.Finding{
		Reference: model.Reference{File: "/repo/home/ai/claude-code/default.nix", Line: 8, Owner: "o", Repo: "r", Number: 1},
		Status:    model.Status{State: "open"}, Tier: model.TierOpen,
	}
	root := model.Finding{
		Reference: model.Reference{File: "/repo/flake.nix", Line: 1, Owner: "o", Repo: "r", Number: 2},
		Status:    model.Status{State: "open"}, Tier: model.TierOpen,
	}
	m := New([]model.Finding{deep, root}, Deps{Root: "/repo"}, Mocha())
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	m = nm.(Model)

	dr := strip(m.renderFindingRow(deep, false, m.locColWidth(140), 140))
	if !strings.Contains(dr, "claude-code/default.nix:8") {
		t.Fatalf("deep path should trim to last two components: %q", dr)
	}
	rr := strip(m.renderFindingRow(root, false, m.locColWidth(140), 140))
	if !strings.Contains(rr, "flake.nix:1") || strings.Contains(rr, "repo/flake.nix") {
		t.Fatalf("root-level file should show no parent dir (not the repo name): %q", rr)
	}
}

func TestFindingRowTruncatesLongTitle(t *testing.T) {
	f := model.Finding{
		Reference: model.Reference{File: "a.go", Line: 2, Owner: "o", Repo: "r", Number: 1},
		Status:    model.Status{State: "open", Title: strings.Repeat("x", 200)},
		Tier:      model.TierOpen,
	}
	m := New([]model.Finding{f}, Deps{}, Mocha())
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = nm.(Model)
	row := strip(m.renderFindingRow(f, false, m.locColWidth(40), 40))
	if strings.Contains(row, strings.Repeat("x", 200)) {
		t.Fatalf("long title not truncated: %q", row)
	}
	if !strings.Contains(row, "…") {
		t.Fatalf("expected truncation ellipsis in: %q", row)
	}
}

func TestRowTitleScalesWithPane(t *testing.T) {
	// The location is a fixed parent_dir/file now, so the width-adaptive part of
	// a row is the title: a wider pane should show more of it.
	f := model.Finding{
		Reference: model.Reference{File: "a/very/long/path/to/some/file.go", Line: 1, Owner: "o", Repo: "r", Number: 1},
		Status:    model.Status{State: "open", Title: strings.Repeat("word ", 60)},
		Tier:      model.TierOpen,
	}
	m := New([]model.Finding{f}, Deps{}, Mocha())
	narrow := strip(m.renderFindingRow(f, false, m.locColWidth(40), 40))
	wide := strip(m.renderFindingRow(f, false, m.locColWidth(100), 100))
	if len(strings.TrimSpace(wide)) <= len(strings.TrimSpace(narrow)) {
		t.Fatalf("wider pane should show more of the title:\nnarrow=%q\nwide=%q", narrow, wide)
	}
}

func TestFindingRowFitsWidth(t *testing.T) {
	f := mkF("a/very/long/path/that/would/overflow/file.go", 123, model.TierStale, time.Time{})
	m := New([]model.Finding{f}, Deps{}, Mocha())
	locW := m.locColWidth(40)
	if w := lipgloss.Width(m.renderFindingRow(f, true, locW, 40)); w != 40 {
		t.Fatalf("selected row width = %d, want 40 (full-width highlight)", w)
	}
	if w := lipgloss.Width(m.renderFindingRow(f, false, locW, 40)); w > 40 {
		t.Fatalf("unselected row width = %d, want <= 40 (no overflow)", w)
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

func TestGroupedRowOmitsFilePath(t *testing.T) {
	f := mkF("some/dir/file.go", 7, model.TierOpen, time.Time{})
	m := New([]model.Finding{f}, Deps{}, Mocha())
	m.mode = modeFile
	row := strip(m.renderFindingRow(f, false, m.locColWidth(60), 60))
	if strings.Contains(row, "file.go") {
		t.Fatalf("grouped row should omit the filename (it's in the header): %q", row)
	}
	if !strings.Contains(row, ":7") {
		t.Fatalf("grouped row should still show the line: %q", row)
	}
	if !strings.Contains(row, "o/r#7") {
		t.Fatalf("grouped row should still show the ref: %q", row)
	}
}

func TestUngroupedRowKeepsFilePath(t *testing.T) {
	f := mkF("some/dir/file.go", 7, model.TierOpen, time.Time{})
	m := New([]model.Finding{f}, Deps{}, Mocha()) // modeTier
	row := strip(m.renderFindingRow(f, false, m.locColWidth(60), 60))
	if !strings.Contains(row, "file.go") {
		t.Fatalf("non-grouped row should show the filename: %q", row)
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

func TestYankFileLine(t *testing.T) {
	m := New(fixture(), Deps{}, Mocha())
	nm, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = nm.(Model)
	if !strings.Contains(m.status, "a.go:2") {
		t.Fatalf("status should mention yanked file:line, got %q", m.status)
	}
	if cmd == nil {
		t.Fatal("y should return a clipboard command")
	}
}

func TestYankURL(t *testing.T) {
	m := New(fixture(), Deps{}, Mocha())
	nm, cmd := m.Update(tea.KeyPressMsg{Code: 'Y', Text: "Y"})
	m = nm.(Model)
	if !strings.Contains(m.status, "https://github.com/o/r/issues/1") {
		t.Fatalf("status should mention yanked URL, got %q", m.status)
	}
	if cmd == nil {
		t.Fatal("Y should return a clipboard command")
	}
}

func TestColumnsLeftPacked(t *testing.T) {
	f := mkF("a.go", 1, model.TierOpen, time.Time{})
	m := New([]model.Finding{f}, Deps{}, Mocha())
	locW := m.locColWidth(80)
	row := strip(m.renderFindingRow(f, false, locW, 80))
	off := strings.Index(row, "o/r#1")
	if off < 0 {
		t.Fatalf("ref missing: %q", row)
	}
	// Left-packed: ref sits just after the location + fixed [state] column, far
	// from where a right-stretched ref (~width-len(ref)) would land.
	if off > 36 {
		t.Fatalf("ref not left-packed (sits at offset %d, expected near the filename): %q", off, row)
	}
}
