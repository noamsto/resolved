package engine

import (
	"context"
	"os"
	"runtime"
	"sort"

	"golang.org/x/sync/errgroup"

	"github.com/noamsto/resolved/internal/cache"
	"github.com/noamsto/resolved/internal/detect"
	"github.com/noamsto/resolved/internal/model"
	"github.com/noamsto/resolved/internal/patterns"
)

// StatusFetcher resolves reference statuses (implemented by github.Client).
type StatusFetcher interface {
	Fetch(ctx context.Context, refs []model.Reference) (map[string]model.Status, error)
}

type Options struct {
	Targets  []string // file paths to scan
	Keywords []string
	Owner    string // origin owner, for bare #n
	Repo     string // origin repo, for bare #n
	Cache    *cache.Cache
	GitHub   StatusFetcher
}

type Summary struct {
	Scanned int `json:"scanned"`
	Skipped int `json:"skipped"` // targets with no grammar for their extension
	Refs    int `json:"refs"`
	Stale   int `json:"stale"`
	Closed  int `json:"closed"`
	Open    int `json:"open"`
	Gone    int `json:"gone"`
	Unknown int `json:"unknown"`
}

type Result struct {
	Findings []model.Finding
	Summary  Summary
}

// Run executes the full pipeline: detect -> patterns -> dedupe -> cache/github
// -> classify -> summarize.
func Run(ctx context.Context, opts Options) (Result, error) {
	refs, scanned, skipped, err := scanTargets(ctx, opts)
	if err != nil {
		return Result{}, err
	}

	statuses, err := resolveStatuses(ctx, opts, refs)
	if err != nil {
		return Result{}, err
	}

	res := Result{Summary: Summary{Scanned: scanned, Skipped: skipped}}
	for _, r := range refs {
		st := statuses[r.Key()]
		if st.State == "" {
			st.State = "unknown"
		}
		if r.Kind == model.KindBare && st.State == "gone" {
			// A bare #n pointing at an issue that never existed wasn't an
			// issue reference; explicit URL/owner-repo refs stay reported.
			continue
		}
		tier := model.ClassifyTier(st.State, r.Keyword)
		res.Findings = append(res.Findings, model.Finding{Reference: r, Status: st, Tier: tier})
		switch tier {
		case model.TierStale:
			res.Summary.Stale++
		case model.TierClosed:
			res.Summary.Closed++
		case model.TierOpen:
			res.Summary.Open++
		case model.TierGone:
			res.Summary.Gone++
		default:
			res.Summary.Unknown++
		}
	}
	res.Summary.Refs = len(res.Findings)
	return res, nil
}

// Scan runs the local pass only: it extracts references and reports scan/skip
// counts without resolving any statuses. Findings come back unclassified
// (TierUnknown, empty Status) so a caller can paint them before the network
// fetch — used by the explore TUI to show refs while statuses stream in.
func Scan(opts Options) ([]model.Finding, Summary, error) {
	refs, scanned, skipped, err := scanTargets(context.Background(), opts)
	if err != nil {
		return nil, Summary{}, err
	}
	findings := make([]model.Finding, 0, len(refs))
	for _, r := range refs {
		findings = append(findings, model.Finding{Reference: r, Tier: model.TierUnknown})
	}
	return findings, Summary{Scanned: scanned, Skipped: skipped, Refs: len(findings), Unknown: len(findings)}, nil
}

// scanTargets reads every target file and extracts references with keywords.
// Files are processed concurrently (capped at NumCPU); each writes into its own
// result slot, so findings stay in target order regardless of completion order.
// skipped counts targets with no grammar for their extension; an unreadable
// supported file is counted as neither scanned nor skipped.
func scanTargets(ctx context.Context, opts Options) ([]model.Reference, int, int, error) {
	type target struct {
		refs    []model.Reference
		scanned bool
		skipped bool
	}
	out := make([]target, len(opts.Targets))

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(runtime.NumCPU())
	for i, path := range opts.Targets {
		if ctx.Err() != nil {
			break
		}
		g.Go(func() error {
			if !detect.Supported(path) {
				out[i].skipped = true
				return nil
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return nil // unreadable: counted as neither
			}
			comments, err := detect.Comments(path, src)
			if err != nil {
				return err
			}
			out[i].scanned = true
			for _, cm := range comments {
				kw := patterns.DetectKeyword(cm.Text, opts.Keywords)
				for _, m := range patterns.Extract(cm.Text, opts.Owner, opts.Repo) {
					out[i].refs = append(out[i].refs, model.Reference{
						File: path, Line: cm.Line, Col: cm.Col + m.Col,
						Raw: m.Raw, Kind: m.Kind, Owner: m.Owner, Repo: m.Repo,
						Number: m.Number, Type: m.Type, Keyword: kw, Confidence: m.Confidence,
					})
				}
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, 0, 0, err
	}

	var refs []model.Reference
	scanned, skipped := 0, 0
	for _, t := range out {
		switch {
		case t.scanned:
			scanned++
			refs = append(refs, t.refs...)
		case t.skipped:
			skipped++
		}
	}
	return refs, scanned, skipped, nil
}

// resolveStatuses returns a status per reference key, using the cache first and
// batching cache-misses through GitHub.
func resolveStatuses(ctx context.Context, opts Options, refs []model.Reference) (map[string]model.Status, error) {
	statuses := map[string]model.Status{}
	seen := map[string]model.Reference{}
	for _, r := range refs {
		key := r.Key()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = r
		if st, ok := opts.Cache.Get(key); ok {
			statuses[key] = st
		}
	}

	var misses []model.Reference
	for key, r := range seen {
		if _, ok := statuses[key]; !ok {
			misses = append(misses, r)
		}
	}
	// Deterministic order for stable GraphQL queries / tests.
	sort.Slice(misses, func(i, j int) bool { return misses[i].Key() < misses[j].Key() })

	if len(misses) > 0 {
		fetched, err := opts.GitHub.Fetch(ctx, misses)
		if err != nil {
			return nil, err
		}
		for key, st := range fetched {
			statuses[key] = st
			opts.Cache.Put(key, st)
		}
	}
	return statuses, nil
}
