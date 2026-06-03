package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/noamsto/resolved/internal/engine"
	"github.com/noamsto/resolved/internal/gitctx"
	"github.com/noamsto/resolved/internal/github"
	"github.com/noamsto/resolved/internal/model"
	"github.com/noamsto/resolved/internal/patterns"
	"github.com/spf13/cobra"
)

// parseRef turns a single ref string (URL, owner/repo#n, or #n) into a Reference.
func parseRef(s, owner, repo string) (model.Reference, error) {
	matches := patterns.Extract(s, owner, repo)
	if len(matches) == 0 {
		return model.Reference{}, fmt.Errorf("not a recognizable GitHub reference: %q", s)
	}
	m := matches[0]
	return model.Reference{
		Raw: m.Raw, Kind: m.Kind, Owner: m.Owner, Repo: m.Repo,
		Number: m.Number, Type: m.Type, Confidence: m.Confidence,
	}, nil
}

// runCheck fetches one reference's status and classifies it (no keyword context).
func runCheck(ctx context.Context, ref model.Reference, fetcher engine.StatusFetcher) (model.Finding, error) {
	statuses, err := fetcher.Fetch(ctx, []model.Reference{ref})
	if err != nil {
		return model.Finding{}, err
	}
	st := statuses[ref.Key()]
	if st.State == "" {
		st.State = "unknown"
	}
	return model.Finding{Reference: ref, Status: st, Tier: model.ClassifyTier(st.State, ref.Keyword)}, nil
}

func init() {
	cmd := &cobra.Command{
		Use:   "check <url|owner/repo#n|#n>",
		Short: "Print the status of a single GitHub reference",
		Long: "Print the status of a single GitHub reference. " +
			"Exits 1 if the reference is closed or stale; 0 for open/gone/unknown; 2 on tool error.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _ := os.Getwd()                     // cwd only used to find git origin; failure is non-fatal
			owner, repo, _ := gitctx.OriginRepo(dir) // best-effort; empty disables bare #n resolution
			ref, err := parseRef(args[0], owner, repo)
			if err != nil {
				return err
			}
			client, err := github.NewClient()
			if err != nil {
				return err
			}
			f, err := runCheck(context.Background(), ref, client)
			if err != nil {
				return err
			}
			cmd.Printf("%s#%d  %s  %s\n", f.Owner+"/"+f.Repo, f.Number, f.State, f.Title)
			if f.Tier == model.TierClosed || f.Tier == model.TierStale {
				os.Exit(1)
			}
			return nil
		},
	}
	rootCmd.AddCommand(cmd)
}
