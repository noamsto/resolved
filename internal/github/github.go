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
	groups := map[string]*repoAlias{}
	var order []string
	seen := map[string]bool{}
	for _, r := range refs {
		if seen[r.Key()] {
			continue
		}
		seen[r.Key()] = true
		gk := r.Owner + "/" + r.Repo
		if _, ok := groups[gk]; !ok {
			groups[gk] = &repoAlias{owner: r.Owner, repo: r.Repo}
			order = append(order, gk)
		}
		groups[gk].refs = append(groups[gk].refs, r)
	}

	query, aliasToKey := buildQuery(order, groups)

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
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github graphql: status %d", resp.StatusCode)
	}

	var raw struct {
		Data   map[string]map[string]*node `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
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
	for repoAliasName, items := range raw.Data {
		for itemAlias, n := range items {
			key := aliasToKey[repoAliasName+"."+itemAlias]
			if key == "" {
				continue
			}
			out[key] = n.status()
		}
	}
	// Any ref not present in the response (shouldn't happen) defaults to gone.
	for _, r := range refs {
		if _, ok := out[r.Key()]; !ok {
			out[r.Key()] = model.Status{State: "gone"}
		}
	}
	return out, nil
}

type node struct {
	Typename  string    `json:"__typename"`
	State     string    `json:"state"`
	Title     string    `json:"title"`
	UpdatedAt time.Time `json:"updatedAt"`
	Merged    bool      `json:"merged"`
}

func (n *node) status() model.Status {
	if n == nil {
		return model.Status{State: "gone"}
	}
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

func buildQuery(order []string, groups map[string]*repoAlias) (string, map[string]string) {
	aliasToKey := map[string]string{}
	var b strings.Builder
	b.WriteString("query {\n")
	for ri, gk := range order {
		g := groups[gk]
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
