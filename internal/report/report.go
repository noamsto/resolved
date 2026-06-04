package report

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/noamsto/resolved/internal/engine"
	"github.com/noamsto/resolved/internal/model"
	"golang.org/x/term"
)

// UseJSON decides output mode: JSON when forced or when stdout is not a TTY.
func UseJSON(forceJSON bool) bool {
	if forceJSON {
		return true
	}
	return !term.IsTerminal(int(os.Stdout.Fd()))
}

type jsonFinding struct {
	File       string `json:"file"`
	Line       int    `json:"line"`
	Col        int    `json:"col"`
	Raw        string `json:"raw"`
	Kind       string `json:"kind"`
	Owner      string `json:"owner"`
	Repo       string `json:"repo"`
	Number     int    `json:"number"`
	Type       string `json:"type"`
	Keyword    string `json:"keyword"`
	Confidence string `json:"confidence"`
	State      string `json:"state"`
	Title      string `json:"title"`
	Tier       string `json:"tier"`
}

type jsonOutput struct {
	Summary  engine.Summary `json:"summary"`
	Findings []jsonFinding  `json:"findings"`
}

// RenderJSON writes the stable machine-readable schema.
func RenderJSON(w io.Writer, r engine.Result) error {
	out := jsonOutput{Summary: r.Summary, Findings: make([]jsonFinding, 0, len(r.Findings))}
	for _, f := range r.Findings {
		out.Findings = append(out.Findings, jsonFinding{
			File: f.File, Line: f.Line, Col: f.Col, Raw: f.Raw,
			Kind: f.Kind.String(), Owner: f.Owner, Repo: f.Repo, Number: f.Number,
			Type: f.Type.String(), Keyword: f.Keyword, Confidence: f.Confidence.String(),
			State: f.State, Title: f.Title, Tier: f.Tier.String(),
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// RenderHuman writes a grouped, readable report. color is currently used to
// gate ANSI codes; when false, output is plain.
func RenderHuman(w io.Writer, r engine.Result, color bool) {
	order := []model.Tier{model.TierStale, model.TierClosed, model.TierGone, model.TierOpen, model.TierUnknown}
	labels := map[model.Tier]string{
		model.TierStale: "STALE", model.TierClosed: "closed", model.TierGone: "gone",
		model.TierOpen: "open", model.TierUnknown: "unknown",
	}
	for _, tier := range order {
		var group []model.Finding
		for _, f := range r.Findings {
			if f.Tier == tier {
				group = append(group, f)
			}
		}
		if len(group) == 0 {
			continue
		}
		fmt.Fprintf(w, "%s (%d)\n", labels[tier], len(group))
		for _, f := range group {
			fmt.Fprintf(w, "  %s:%d  %s#%d  %s\n", f.File, f.Line, f.Owner+"/"+f.Repo, f.Number, f.Title)
		}
	}
	s := r.Summary
	fmt.Fprintf(w, "\n%d refs in %d files — %d stale, %d closed, %d open, %d gone, %d unknown",
		s.Refs, s.Scanned, s.Stale, s.Closed, s.Open, s.Gone, s.Unknown)
	if s.Skipped > 0 {
		// An all-unsupported repo must not read as a clean scan.
		fmt.Fprintf(w, " (%d skipped: unsupported language)", s.Skipped)
	}
	fmt.Fprintln(w)
}

// ExitCode returns 0 (clean) or 1 (gate tripped) per the fail-on policy.
// stale  -> only stale trips; closed -> stale+closed; any -> stale+closed+gone.
func ExitCode(r engine.Result, failOn string) int {
	for _, f := range r.Findings {
		switch failOn {
		case "stale":
			if f.Tier == model.TierStale {
				return 1
			}
		case "closed":
			if f.Tier == model.TierStale || f.Tier == model.TierClosed {
				return 1
			}
		case "any":
			if f.Tier == model.TierStale || f.Tier == model.TierClosed || f.Tier == model.TierGone {
				return 1
			}
		}
	}
	return 0
}
