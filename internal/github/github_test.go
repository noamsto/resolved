package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/noamsto/resolved/internal/model"
)

func TestFetchParsesStatuses(t *testing.T) {
	// Mock GraphQL endpoint returning two refs: a closed issue and a merged PR.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"r0": {
					"i0": {"__typename":"Issue","state":"CLOSED","title":"bug","updatedAt":"2026-01-01T00:00:00Z"},
					"i1": {"__typename":"PullRequest","state":"MERGED","title":"fix","updatedAt":"2026-02-01T00:00:00Z","merged":true}
				}
			}
		}`))
	}))
	defer srv.Close()

	c := &Client{httpClient: srv.Client(), endpoint: srv.URL, token: "t"}
	refs := []model.Reference{
		{Owner: "o", Repo: "r", Number: 1},
		{Owner: "o", Repo: "r", Number: 2},
	}
	got, err := c.Fetch(context.Background(), refs)
	if err != nil {
		t.Fatal(err)
	}
	if got["o/r#1"].State != "closed" {
		t.Errorf("#1 state = %q, want closed", got["o/r#1"].State)
	}
	if got["o/r#2"].State != "merged" {
		t.Errorf("#2 state = %q, want merged", got["o/r#2"].State)
	}
}

func TestFetchGraphQLErrorReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":null,"errors":[{"message":"Could not resolve to a Repository with the name 'o/r'."}]}`))
	}))
	defer srv.Close()

	c := &Client{httpClient: srv.Client(), endpoint: srv.URL, token: "t"}
	_, err := c.Fetch(context.Background(), []model.Reference{{Owner: "o", Repo: "r", Number: 1}})
	if err == nil {
		t.Fatal("expected error from graphql errors response, got nil")
	}
}

func TestFetchMissingNodeIsGone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"r0":{"i0":null}},"errors":[{"type":"NOT_FOUND","path":["r0","i0"],"message":"Could not resolve to an issue or pull request with the number of 1."}]}`))
	}))
	defer srv.Close()

	c := &Client{httpClient: srv.Client(), endpoint: srv.URL, token: "t"}
	got, err := c.Fetch(context.Background(), []model.Reference{{Owner: "o", Repo: "r", Number: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if got["o/r#1"].State != "gone" {
		t.Fatalf("state = %q, want gone", got["o/r#1"].State)
	}
}

func fetchWith(t *testing.T, body string, refs []model.Reference) map[string]model.Status {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	c := &Client{httpClient: srv.Client(), endpoint: srv.URL, token: "t"}
	got, err := c.Fetch(context.Background(), refs)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestFetchUnresolvedIsUnknownUnlessNotFound(t *testing.T) {
	twoRefs := []model.Reference{
		{Owner: "o", Repo: "a", Number: 1},
		{Owner: "o", Repo: "a", Number: 2},
	}
	tests := []struct {
		name string
		body string
		refs []model.Reference
		want map[string]string
	}{
		{
			name: "null repository is unknown, other repo resolves",
			body: `{"data":{"r0":null,"r1":{"i0":{"__typename":"Issue","state":"CLOSED","title":"t","updatedAt":"2026-01-01T00:00:00Z"}}},
				"errors":[{"type":"NOT_FOUND","path":["r0"],"message":"Could not resolve to a Repository"}]}`,
			refs: append(append([]model.Reference{}, twoRefs...), model.Reference{Owner: "o", Repo: "b", Number: 3}),
			want: map[string]string{"o/a#1": "unknown", "o/a#2": "unknown", "o/b#3": "closed"},
		},
		{
			name: "null item with FORBIDDEN is unknown",
			body: `{"data":{"r0":{"i0":null}},"errors":[{"type":"FORBIDDEN","path":["r0","i0"],"message":"nope"}]}`,
			refs: twoRefs[:1],
			want: map[string]string{"o/a#1": "unknown"},
		},
		{
			name: "null item with NOT_FOUND and a pathless error is unknown",
			body: `{"data":{"r0":{"i0":{"__typename":"Issue","state":"CLOSED","title":"t","updatedAt":"2026-01-01T00:00:00Z"},"i1":null}},
				"errors":[{"message":"boom"},{"type":"NOT_FOUND","path":["r0","i1"],"message":"gone"}]}`,
			refs: twoRefs,
			want: map[string]string{"o/a#1": "closed", "o/a#2": "unknown"},
		},
		{
			name: "null item with NOT_FOUND and unrelated error is gone",
			body: `{"data":{"r0":{"i0":null,"i1":null}},
				"errors":[{"type":"NOT_FOUND","path":["r0","i0"],"message":"gone"},{"type":"FORBIDDEN","path":["r0","i1"],"message":"nope"}]}`,
			refs: twoRefs,
			want: map[string]string{"o/a#1": "gone", "o/a#2": "unknown"},
		},
		{
			name: "ref absent from data with no errors is unknown",
			body: `{"data":{"r0":{"i0":{"__typename":"Issue","state":"OPEN","title":"t","updatedAt":"2026-01-01T00:00:00Z"}}}}`,
			refs: twoRefs,
			want: map[string]string{"o/a#1": "open", "o/a#2": "unknown"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fetchWith(t, tt.body, tt.refs)
			for key, want := range tt.want {
				if got[key].State != want {
					t.Errorf("%s state = %q, want %q", key, got[key].State, want)
				}
			}
		})
	}
}
