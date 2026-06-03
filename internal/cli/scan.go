package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/noamsto/resolved/internal/cache"
	"github.com/noamsto/resolved/internal/engine"
	"github.com/noamsto/resolved/internal/github"
	"github.com/noamsto/resolved/internal/gitctx"
	"github.com/noamsto/resolved/internal/report"
	"github.com/spf13/cobra"
)

var defaultKeywords = []string{"TODO", "FIXME", "HACK", "XXX", "BUG"}

// scanConfig is the fully-resolved input to runScan (separated from cobra for
// testability).
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
	fetcher  engine.StatusFetcher // injected in tests; nil => real github client
	out      io.Writer
}

// runScan resolves targets, runs the engine, renders, and returns the exit code.
func runScan(cfg scanConfig) (int, error) {
	targets, err := resolveTargets(cfg.dir, cfg.args, cfg.staged, cfg.diffRef, cfg.exclude)
	if err != nil {
		// Not a git repo and no explicit args: fall back to walking cfg.dir directly.
		if len(cfg.args) == 0 && !cfg.staged && cfg.diffRef == "" {
			targets, err = resolveTargets(cfg.dir, []string{cfg.dir}, false, "", cfg.exclude)
		}
		if err != nil {
			return 2, err
		}
	}

	owner, repo, _ := gitctx.OriginRepo(cfg.dir) // best-effort; empty disables bare #n

	fetcher := cfg.fetcher
	if fetcher == nil {
		client, err := github.NewClient()
		if err != nil {
			return 2, err
		}
		fetcher = client
	}

	cacheDir := filepath.Join(os.TempDir(), "resolved-nocache")
	if !cfg.noCache {
		cacheDir = defaultCacheDir()
	}

	res, err := engine.Run(context.Background(), engine.Options{
		Targets:  targets,
		Keywords: cfg.keywords,
		Owner:    owner,
		Repo:     repo,
		Cache:    cache.New(cacheDir),
		GitHub:   fetcher,
	})
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

func init() {
	var (
		failOn   string
		jsonOut  bool
		noColor  bool
		staged   bool
		diffRef  string
		exclude  []string
		keywords []string
		noCache  bool
	)
	cmd := &cobra.Command{
		Use:   "scan [paths...]",
		Short: "Scan comments for stale GitHub references",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			kw := keywords
			if len(kw) == 0 {
				kw = defaultKeywords
			}
			code, err := runScan(scanConfig{
				dir: dir, args: args, keywords: kw, failOn: failOn,
				json: report.UseJSON(jsonOut), noColor: noColor,
				staged: staged, diffRef: diffRef, exclude: exclude,
				noCache: noCache, out: cmd.OutOrStdout(),
			})
			if err != nil {
				return err
			}
			os.Exit(code)
			return nil
		},
	}
	cmd.Flags().StringVar(&failOn, "fail-on", "stale", "tier that sets exit 1: stale|closed|any")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "force JSON output (default: auto by TTY)")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "disable colored human output")
	cmd.Flags().BoolVar(&staged, "staged", false, "scan only git-staged files")
	cmd.Flags().StringVar(&diffRef, "diff", "", "scan only files changed vs this git ref")
	cmd.Flags().StringSliceVar(&exclude, "exclude", nil, "glob(s) to exclude by base name")
	cmd.Flags().StringSliceVar(&keywords, "keywords", nil, "stale keywords (default TODO,FIXME,HACK,XXX,BUG)")
	cmd.Flags().BoolVar(&noCache, "no-cache", false, "bypass the on-disk cache")
	rootCmd.AddCommand(cmd)
}
