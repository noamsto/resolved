package cli

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	tea "charm.land/bubbletea/v2"
	"github.com/noamsto/resolved/internal/model"
	"github.com/noamsto/resolved/internal/tui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

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
// (self) with the original CLI args inside a floating pane rooted at dir.
func tmuxPopupArgs(self, dir string, cliArgs []string) []string {
	args := []string{"display-popup", "-E", "-w", "90%", "-h", "90%", "-d", dir, "--", self}
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
	c.Env = append(os.Environ(), popupGuardEnv+"=1")
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

			findings, err := exploreFindings(cfg)
			if err != nil {
				return err
			}

			theme, err := tui.ThemeByName(themeName)
			if err != nil {
				return err
			}

			deps := tui.Deps{
				OpenURL:   openInBrowser,
				EditorCmd: editorCmd,
				Root:      dir,
				Rescan: func() ([]model.Finding, error) {
					rc := cfg
					rc.noCache = true // explicit refresh always re-queries GitHub
					return exploreFindings(rc)
				},
			}
			p := tea.NewProgram(tui.New(findings, deps, theme))
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
