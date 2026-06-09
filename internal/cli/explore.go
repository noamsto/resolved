package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"

	tea "charm.land/bubbletea/v2"
	"github.com/noamsto/resolved/internal/cache"
	"github.com/noamsto/resolved/internal/engine"
	"github.com/noamsto/resolved/internal/gitctx"
	"github.com/noamsto/resolved/internal/github"
	"github.com/noamsto/resolved/internal/model"
	"github.com/noamsto/resolved/internal/tui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// scanRefs runs the fast local pass: references without resolved statuses, so
// the explore TUI can paint them before any network call.
func scanRefs(cfg scanConfig) ([]model.Finding, error) {
	targets, err := resolveTargets(cfg.dir, cfg.args, cfg.staged, cfg.diffRef, cfg.exclude)
	if err != nil {
		return nil, err
	}
	owner, repo := "", ""
	if cfg.bare {
		owner, repo, _ = gitctx.OriginRepo(cfg.dir)
	}
	findings, _, err := engine.Scan(engine.Options{
		Targets: targets, Keywords: cfg.keywords, Owner: owner, Repo: repo,
	})
	return findings, err
}

// resolveStream resolves statuses for findings' unique issues, streaming a batch
// per chunk so the TUI fills rows progressively. Cached statuses come first (a
// repeat open is near-instant); misses are fetched in batches. It closes the
// channel when done, and stops quietly if no GitHub credential is available.
func resolveStream(cfg scanConfig, findings []model.Finding) <-chan tui.StatusBatch {
	out := make(chan tui.StatusBatch)
	go func() {
		defer close(out)

		seen := map[string]model.Reference{}
		for _, f := range findings {
			if _, ok := seen[f.Key()]; !ok {
				seen[f.Key()] = f.Reference
			}
		}
		total := len(seen)
		if total == 0 {
			return
		}

		var c *cache.Cache
		if cfg.noCache {
			c = cache.Disabled()
		} else {
			c = cache.New(defaultCacheDir())
		}

		cached := map[string]model.Status{}
		var misses []model.Reference
		for key, r := range seen {
			if st, ok := c.Get(key); ok {
				cached[key] = st
			} else {
				misses = append(misses, r)
			}
		}
		done := 0
		if len(cached) > 0 {
			done = len(cached)
			out <- tui.StatusBatch{Statuses: cached, Done: done, Total: total}
		}
		if len(misses) == 0 {
			return
		}

		fetcher := cfg.fetcher
		if fetcher == nil {
			client, err := github.NewClient()
			if err != nil {
				return
			}
			fetcher = client
		}
		// Deterministic order keeps batches stable across runs.
		sort.Slice(misses, func(i, j int) bool { return misses[i].Key() < misses[j].Key() })

		const batchSize = 12
		for start := 0; start < len(misses); start += batchSize {
			end := start + batchSize
			if end > len(misses) {
				end = len(misses)
			}
			chunk := misses[start:end]
			statuses, err := fetcher.Fetch(context.Background(), chunk)
			if err != nil {
				return
			}
			for k, st := range statuses {
				c.Put(k, st)
			}
			done += len(chunk)
			out <- tui.StatusBatch{Statuses: statuses, Done: done, Total: total}
		}
	}()
	return out
}

// exploreFindings runs the scan pipeline and returns just the findings.
func exploreFindings(cfg scanConfig) ([]model.Finding, error) {
	res, err := scanToResult(cfg)
	if err != nil {
		return nil, err
	}
	return res.Findings, nil
}

// openInBrowser opens url with the platform's default handler (non-blocking).
func openInBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("cmd", "/c", "start", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

// popupGuardEnv marks a process already running inside the popup, so the inner
// run doesn't spawn another. An env var (not a flag) is used because flags after
// a user's `--` become positional args and would defeat the guard.
const popupGuardEnv = "RESOLVED_IN_POPUP"

// shouldPopup reports whether explore should relaunch itself inside a tmux
// floating popup: only under tmux, not opted out, and not already in a popup.
func shouldPopup(noPopup bool) bool {
	return !noPopup && os.Getenv("TMUX") != "" && os.Getenv(popupGuardEnv) == ""
}

// tmuxPopupArgs builds the `tmux display-popup` argv that re-runs this binary
// (self) with the original CLI args inside a floating pane rooted at dir. The
// guard is passed with `-e`: display-popup runs its command from the tmux
// server, which does not inherit the caller's environment, so setting the var
// on our own process would not reach the inner run and it would pop up again.
func tmuxPopupArgs(self, dir string, cliArgs []string) []string {
	args := []string{"display-popup", "-E", "-e", popupGuardEnv + "=1", "-w", "90%", "-h", "90%", "-d", dir, "--", self}
	return append(args, cliArgs...)
}

// relaunchInPopup re-execs explore inside a tmux floating popup. display-popup
// blocks until the inner TUI exits, so the original invocation waits as if it
// had run the TUI itself.
func relaunchInPopup() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	c := exec.Command("tmux", tmuxPopupArgs(self, dir, os.Args[1:])...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

// editorCmd opens file at line in $EDITOR, releasing the terminal to the editor.
func editorCmd(file string, line int) tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	// +N line syntax works for vi/vim/nvim/hx; emacs/nano use different flags.
	c := exec.Command(editor, fmt.Sprintf("+%d", line), file)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return tui.EditorDone(err)
	})
}

func init() {
	var (
		exclude   []string
		keywords  []string
		staged    bool
		diffRef   string
		noCache   bool
		bare      bool
		themeName string
		noPopup   bool
	)
	cmd := &cobra.Command{
		Use:   "explore [paths...]",
		Short: "Interactively browse stale GitHub references",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !term.IsTerminal(int(os.Stdout.Fd())) {
				return fmt.Errorf("explore requires an interactive terminal; use `resolved scan` for piped output")
			}
			if shouldPopup(noPopup) {
				return relaunchInPopup()
			}
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			kw := keywords
			if len(kw) == 0 {
				kw = defaultKeywords
			}
			cfg := scanConfig{
				dir: dir, args: args, keywords: kw,
				staged: staged, diffRef: diffRef, exclude: exclude, noCache: noCache, bare: bare,
			}

			theme, err := tui.ThemeByName(themeName)
			if err != nil {
				return err
			}

			// Don't block on the network: paint refs from the local scan, then
			// stream statuses in. New starts in the loading state when Scan is set.
			deps := tui.Deps{
				OpenURL:   openInBrowser,
				EditorCmd: editorCmd,
				Root:      dir,
				Scan:      func() ([]model.Finding, error) { return scanRefs(cfg) },
				Resolve:   func(fs []model.Finding) <-chan tui.StatusBatch { return resolveStream(cfg, fs) },
				Rescan: func() ([]model.Finding, error) {
					rc := cfg
					rc.noCache = true // explicit refresh always re-queries GitHub
					return exploreFindings(rc)
				},
			}
			p := tea.NewProgram(tui.New(nil, deps, theme))
			_, err = p.Run()
			return err
		},
	}
	cmd.Flags().BoolVar(&staged, "staged", false, "scan only git-staged files")
	cmd.Flags().StringVar(&diffRef, "diff", "", "scan only files changed vs this git ref")
	cmd.Flags().StringSliceVar(&exclude, "exclude", nil, "glob(s) to exclude by base name")
	cmd.Flags().StringSliceVar(&keywords, "keywords", nil, "stale keywords (default TODO,FIXME,HACK,XXX,BUG)")
	cmd.Flags().BoolVar(&noCache, "no-cache", false, "bypass the on-disk cache")
	cmd.Flags().BoolVar(&bare, "bare", false, "also match bare #123 references against the origin repo (noisy in active repos)")
	cmd.Flags().StringVar(&themeName, "theme", "mocha", "color theme: mocha|latte|frappe|macchiato")
	cmd.Flags().BoolVar(&noPopup, "no-popup", false, "run inline in the current pane instead of a tmux floating popup")
	rootCmd.AddCommand(cmd)
}
