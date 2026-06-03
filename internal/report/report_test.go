package report

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/noamsto/resolved/internal/engine"
	"github.com/noamsto/resolved/internal/model"
)

func sampleResult() engine.Result {
	return engine.Result{
		Summary: engine.Summary{Scanned: 1, Refs: 1, Stale: 1},
		Findings: []model.Finding{{
			Reference: model.Reference{
				File: "a.go", Line: 2, Col: 4, Raw: "#1", Kind: model.KindBare,
				Owner: "o", Repo: "r", Number: 1, Type: model.TypeIssue,
				Keyword: "TODO", Confidence: model.ConfHigh,
			},
			Status: model.Status{State: "closed", Title: "bug"},
			Tier:   model.TierStale,
		}},
	}
}

func TestRenderJSONSchema(t *testing.T) {
	buf := new(bytes.Buffer)
	if err := RenderJSON(buf, sampleResult()); err != nil {
		t.Fatal(err)
	}
	var out struct {
		Summary  engine.Summary `json:"summary"`
		Findings []struct {
			File    string `json:"file"`
			Tier    string `json:"tier"`
			Kind    string `json:"kind"`
			State   string `json:"state"`
			Keyword string `json:"keyword"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if out.Summary.Stale != 1 || len(out.Findings) != 1 {
		t.Fatalf("bad summary/findings: %+v", out)
	}
	f := out.Findings[0]
	if f.Tier != "stale" || f.Kind != "bare" || f.State != "closed" {
		t.Fatalf("bad finding fields: %+v", f)
	}
}

func TestExitCode(t *testing.T) {
	r := sampleResult() // contains one stale finding
	if got := ExitCode(r, "stale"); got != 1 {
		t.Errorf("fail-on stale = %d, want 1", got)
	}

	openOnly := engine.Result{Findings: []model.Finding{{Tier: model.TierOpen}}}
	if got := ExitCode(openOnly, "stale"); got != 0 {
		t.Errorf("open-only fail-on stale = %d, want 0", got)
	}

	closedOnly := engine.Result{Findings: []model.Finding{{Tier: model.TierClosed}}}
	if got := ExitCode(closedOnly, "closed"); got != 1 {
		t.Errorf("closed fail-on closed = %d, want 1", got)
	}
	if got := ExitCode(closedOnly, "stale"); got != 0 {
		t.Errorf("closed fail-on stale = %d, want 0", got)
	}
}

func TestRenderHumanContainsLocation(t *testing.T) {
	buf := new(bytes.Buffer)
	RenderHuman(buf, sampleResult(), false)
	if !bytes.Contains(buf.Bytes(), []byte("a.go:2")) {
		t.Fatalf("human output missing location:\n%s", buf.String())
	}
}
