package gitctx

import "testing"

func TestParseRemoteURL(t *testing.T) {
	cases := []struct {
		url, owner, repo string
	}{
		{"git@github.com:noamsto/resolved.git", "noamsto", "resolved"},
		{"https://github.com/noamsto/resolved.git", "noamsto", "resolved"},
		{"https://github.com/noamsto/resolved", "noamsto", "resolved"},
		{"ssh://git@github.com/noamsto/resolved.git", "noamsto", "resolved"},
	}
	for _, c := range cases {
		o, r, err := parseRemoteURL(c.url)
		if err != nil {
			t.Errorf("%s: unexpected error %v", c.url, err)
			continue
		}
		if o != c.owner || r != c.repo {
			t.Errorf("%s => %q/%q, want %q/%q", c.url, o, r, c.owner, c.repo)
		}
	}
}

func TestParseRemoteURLNonGitHub(t *testing.T) {
	if _, _, err := parseRemoteURL("https://gitlab.com/a/b.git"); err == nil {
		t.Fatal("expected error for non-github remote")
	}
}
