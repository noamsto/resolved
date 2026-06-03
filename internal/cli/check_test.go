package cli

import (
	"context"
	"testing"

	"github.com/noamsto/resolved/internal/model"
)

func TestParseRefURL(t *testing.T) {
	r, err := parseRef("https://github.com/o/r/issues/5", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if r.Owner != "o" || r.Repo != "r" || r.Number != 5 {
		t.Fatalf("bad ref: %+v", r)
	}
}

func TestParseRefBareNeedsOrigin(t *testing.T) {
	if _, err := parseRef("#5", "", ""); err == nil {
		t.Fatal("bare ref without origin should error")
	}
	r, err := parseRef("#5", "o", "r")
	if err != nil || r.Number != 5 || r.Owner != "o" {
		t.Fatalf("bare with origin failed: %+v err=%v", r, err)
	}
}

func TestRunCheckReturnsTier(t *testing.T) {
	fetcher := stubFetcher{statuses: map[string]model.Status{"o/r#5": {State: "closed", Title: "bug"}}}
	f, err := runCheck(context.Background(), model.Reference{Owner: "o", Repo: "r", Number: 5}, fetcher)
	if err != nil {
		t.Fatal(err)
	}
	if f.Tier != model.TierClosed {
		t.Fatalf("tier = %v, want closed (no keyword)", f.Tier)
	}
}
