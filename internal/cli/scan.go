package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"
	"github.com/noamsto/resolved/internal/cache"
	"github.com/noamsto/resolved/internal/engine"
	"github.com/noamsto/resolved/internal/gitctx"
	"github.com/noamsto/resolved/internal/github"
	"github.com/noamsto/resolved/internal/report"
)

var defaultKeywords = []string{"TODO", "FIXME", "HACK", "XXX", "BUG"}

// scanConfig is the fully-resolved input to runScan (separated from the command
// grammar for testability).
type scanConfig struct {
	dir      string
	args     []string
	keywords []string
	failOn   string
	json     bool
	noColor  bool
	staged   bool
	diffRef  string
	exclude  []string
	noCache  bool
	bare     bool
	fetcher  engine.StatusFetcher // injected in tests; nil => real github client
	out      io.Writer
}

// scanToResult resolves targets and runs the engine, returning the findings.
// Shared by the scan and explore commands.
func scanToResult(cfg scanConfig) (engine.Result, error) {
	targets, err := resolveTargets(cfg.dir, cfg.args, cfg.staged, cfg.diffRef, cfg.exclude)
	if err != nil {
		return engine.Result{}, err
	}

	owner, repo, _ := gitctx.OriginRepo(cfg.dir) // best-effort; empty disables bare #n
	if !cfg.bare {
		owner, repo = "", "" // bare #n matching is opt-in
	}

	fetcher := cfg.fetcher
	if fetcher == nil {
		client, err := github.NewClient()
		if err != nil {
			return engine.Result{}, err
		}
		fetcher = client
	}

	var c *cache.Cache
	if cfg.noCache {
		c = cache.Disabled()
	} else {
		c = cache.New(defaultCacheDir())
	}

	return engine.Run(context.Background(), engine.Options{
		Targets:  targets,
		Keywords: cfg.keywords,
		Owner:    owner,
		Repo:     repo,
		Cache:    c,
		GitHub:   fetcher,
	})
}

// runScan resolves targets, runs the engine, renders, and returns the exit code.
// The returned int is only meaningful when err == nil; on a non-nil error the
// caller must treat it as a tool error (process exit 2), which Execute() handles.
func runScan(cfg scanConfig) (int, error) {
	switch cfg.failOn {
	case "stale", "closed", "any":
	default:
		return 2, fmt.Errorf("unknown --fail-on value %q: must be stale|closed|any", cfg.failOn)
	}

	res, err := scanToResult(cfg)
	if err != nil {
		return 2, err
	}

	if cfg.json {
		if err := report.RenderJSON(cfg.out, res); err != nil {
			return 2, err
		}
	} else {
		report.RenderHuman(cfg.out, res, !cfg.noColor)
	}
	return report.ExitCode(res, cfg.failOn), nil
}

func defaultCacheDir() string {
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "resolved")
	}
	return filepath.Join(os.TempDir(), "resolved")
}

// targetFlags are the target-selection flags shared by scan and explore. It is
// embedded without a prefix, so the flags appear on each command unqualified.
type targetFlags struct {
	Staged   bool     `help:"scan only git-staged files"`
	Diff     string   `help:"scan only files changed vs this git ref"`
	Exclude  []string `help:"glob(s) to exclude by base name"`
	Keywords []string `help:"stale keywords (default TODO,FIXME,HACK,XXX,BUG)"`
	NoCache  bool     `help:"bypass the on-disk cache"`
	Bare     bool     `help:"also match bare #123 references against the origin repo (noisy in active repos)"`
}

// config resolves the flags and paths into the scanConfig both commands run on.
func (f targetFlags) config(dir string, paths []string) scanConfig {
	kw := f.Keywords
	if len(kw) == 0 {
		kw = defaultKeywords
	}
	return scanConfig{
		dir: dir, args: paths, keywords: kw,
		staged: f.Staged, diffRef: f.Diff, exclude: f.Exclude,
		noCache: f.NoCache, bare: f.Bare,
	}
}

// ScanCmd scans comments for stale GitHub references.
type ScanCmd struct {
	Paths   []string    `arg:"" optional:"" name:"path" help:"Paths to scan (default: the whole repo)"`
	Targets targetFlags `embed:""`

	FailOn  string `default:"stale" help:"tier that sets exit 1: stale|closed|any"`
	JSON    bool   `name:"json" help:"force JSON output (default: auto by TTY)"`
	NoColor bool   `help:"disable colored human output"`
}

func (c ScanCmd) Run(kctx *kong.Context) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	cfg := c.Targets.config(dir, c.Paths)
	cfg.failOn = c.FailOn
	cfg.json = report.UseJSON(c.JSON)
	cfg.noColor = c.NoColor
	cfg.out = kctx.Stdout

	code, err := runScan(cfg)
	if err != nil {
		return err
	}
	os.Exit(code)
	return nil
}
