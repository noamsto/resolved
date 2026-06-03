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
		_, _ = w.Write([]byte(`{"data":{"r0":{"i0":null}}}`))
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
