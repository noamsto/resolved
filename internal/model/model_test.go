package model

import "testing"

func TestReferenceKey(t *testing.T) {
	r := Reference{Owner: "noamsto", Repo: "resolved", Number: 42}
	if got := r.Key(); got != "noamsto/resolved#42" {
		t.Fatalf("Key() = %q", got)
	}
}

func TestClassifyTier(t *testing.T) {
	cases := []struct {
		state, keyword string
		want           Tier
	}{
		{"open", "TODO", TierOpen},
		{"open", "", TierOpen},
		{"closed", "TODO", TierStale},
		{"merged", "FIXME", TierStale},
		{"closed", "", TierClosed},
		{"merged", "", TierClosed},
		{"gone", "TODO", TierGone},
		{"unknown", "", TierUnknown},
	}
	for _, c := range cases {
		if got := ClassifyTier(c.state, c.keyword); got != c.want {
			t.Errorf("ClassifyTier(%q,%q) = %v, want %v", c.state, c.keyword, got, c.want)
		}
	}
}

func TestTierString(t *testing.T) {
	if TierStale.String() != "stale" {
		t.Fatalf("TierStale.String() = %q", TierStale.String())
	}
}
