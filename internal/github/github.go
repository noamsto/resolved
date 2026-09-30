package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/noamsto/resolved/internal/model"
)

const defaultEndpoint = "https://api.github.com/graphql"

type Client struct {
	httpClient *http.Client
	endpoint   string
	token      string
}

// NewClient resolves auth: GITHUB_TOKEN / GH_TOKEN env, else `gh auth token`.
// Returns an error if no credential can be found.
func NewClient() (*Client, error) {
	token := firstNonEmpty(os.Getenv("GITHUB_TOKEN"), os.Getenv("GH_TOKEN"))
	if token == "" {
		if out, err := exec.Command("gh", "auth", "token").Output(); err == nil {
			token = strings.TrimSpace(string(out))
		}
	}
	if token == "" {
		return nil, fmt.Errorf("no GitHub credential: set GITHUB_TOKEN or run `gh auth login`")
	}
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		endpoint:   defaultEndpoint,
		token:      token,
	}, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// repoAlias groups a set of references in one repository.
type repoAlias struct {
	owner, repo string
	refs        []model.Reference // index i -> alias i<n>
}

// Fetch resolves the status of every reference in one GraphQL request.
func (c *Client) Fetch(ctx context.Context, refs []model.Reference) (map[string]model.Status, error) {
	if len(refs) == 0 {
		return map[string]model.Status{}, nil
	}

	// Group references by owner/repo and assign aliases.
	groups := map[string]repoAlias{}
	var order []string
	seen := map[string]bool{}
	for _, r := range refs {
		if seen[r.Key()] {
			continue
		}
		seen[r.Key()] = true
		gk := r.Owner + "/" + r.Repo
		g, ok := groups[gk]
		if !ok {
			g = repoAlias{owner: r.Owner, repo: r.Repo}
			order = append(order, gk)
		}
		g.refs = append(g.refs, r)
		groups[gk] = g
	}
	ordered := make([]repoAlias, 0, len(order))
	for _, gk := range order {
		ordered = append(ordered, groups[gk])
	}

	query, aliasToKey := buildQuery(ordered)

	body, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		return nil, fmt.Errorf("marshal graphql body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github graphql: status %d", resp.StatusCode)
	}

	var raw struct {
		Data   map[string]map[string]*node `json:"data"`
		Errors []gqlError                  `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	if raw.Data == nil && len(raw.Errors) > 0 {
		msgs := make([]string, len(raw.Errors))
		for i, e := range raw.Errors {
			msgs[i] = e.Message
		}
		return nil, fmt.Errorf("github graphql: %s", strings.Join(msgs, "; "))
	}

	out := make(map[string]model.Status, len(refs))
	for alias, key := range aliasToKey {
		ra, ia, _ := strings.Cut(alias, ".")
		out[key] = classify(raw.Data[ra], ra, ia, raw.Errors)
	}
	return out, nil
}

type gqlError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Path    []any  `json:"path"`
}

func (e gqlError) exactNotFound(ra, ia string) bool {
	return e.Type == "NOT_FOUND" && len(e.Path) == 2 && e.Path[0] == ra && e.Path[1] == ia
}

// covers reports whether the error could explain a missing (ra, ia). Pathless
// errors, or ones with a non-string path head, cover everything.
func (e gqlError) covers(ra, ia string) bool {
	if len(e.Path) == 0 {
		return true
	}
	if _, ok := e.Path[0].(string); !ok {
		return true
	}
	target := [2]string{ra, ia}
	for i := 0; i < len(e.Path) && i < len(target); i++ {
		if s, ok := e.Path[i].(string); !ok || s != target[i] {
			return false
		}
	}
	return true
}

// classify reports "gone" only when GitHub positively says the item does not
// exist; anything else unresolved is "unknown" so it is never treated as stale.
func classify(repo map[string]*node, ra, ia string, errs []gqlError) model.Status {
	unknown := model.Status{State: "unknown"}
	if repo == nil {
		return unknown
	}
	n, present := repo[ia]
	if n != nil {
		return n.status()
	}
	if !present {
		return unknown
	}
	found := false
	for _, e := range errs {
		switch {
		case e.exactNotFound(ra, ia):
			found = true
		case e.covers(ra, ia):
			return unknown
		}
	}
	if !found {
		return unknown
	}
	return model.Status{State: "gone"}
}

type node struct {
	Typename  string    `json:"__typename"`
	State     string    `json:"state"`
	Title     string    `json:"title"`
	UpdatedAt time.Time `json:"updatedAt"`
	Merged    bool      `json:"merged"`
}

func (n *node) status() model.Status {
	state := "open"
	switch {
	case n.Typename == "PullRequest" && n.Merged:
		state = "merged"
	case strings.EqualFold(n.State, "CLOSED"):
		state = "closed"
	case strings.EqualFold(n.State, "MERGED"):
		state = "merged"
	}
	return model.Status{State: state, Title: n.Title, UpdatedAt: n.UpdatedAt}
}

func buildQuery(groups []repoAlias) (string, map[string]string) {
	aliasToKey := map[string]string{}
	var b strings.Builder
	b.WriteString("query {\n")
	for ri, g := range groups {
		ra := fmt.Sprintf("r%d", ri)
		fmt.Fprintf(&b, "  %s: repository(owner: %q, name: %q) {\n", ra, g.owner, g.repo)
		for ii, ref := range g.refs {
			itemAlias := fmt.Sprintf("i%d", ii)
			aliasToKey[ra+"."+itemAlias] = ref.Key()
			fmt.Fprintf(&b, "    %s: issueOrPullRequest(number: %d) {\n", itemAlias, ref.Number)
			b.WriteString("      __typename\n")
			b.WriteString("      ... on Issue { state title updatedAt }\n")
			b.WriteString("      ... on PullRequest { state title updatedAt merged }\n")
			b.WriteString("    }\n")
		}
		b.WriteString("  }\n")
	}
	b.WriteString("}\n")
	return b.String(), aliasToKey
}
