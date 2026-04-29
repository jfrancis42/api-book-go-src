package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func withServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(func() { srv.Close(); setBaseURL("https://api.github.com") })
	setBaseURL(srv.URL)
	return NewClient("test-token")
}

func TestListIssues_DefaultParams(t *testing.T) {
	var gotQuery url.Values
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]Issue{{Number: 1, Title: "Bug", State: "open"}})
	})
	issues, err := c.ListIssues(context.Background(), "golang", "go", IssueOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 {
		t.Fatalf("got %d issues, want 1", len(issues))
	}
	if gotQuery.Get("state") != "" {
		t.Error("expected no state param for zero options")
	}
}

func TestListIssues_WithState(t *testing.T) {
	var gotState string
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotState = r.URL.Query().Get("state")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]Issue{})
	})
	c.ListIssues(context.Background(), "owner", "repo", IssueOptions{State: "closed", PerPage: 50})
	if gotState != "closed" {
		t.Errorf("got state=%q, want closed", gotState)
	}
}

func TestListIssues_PerPage(t *testing.T) {
	var gotPerPage string
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPerPage = r.URL.Query().Get("per_page")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]Issue{})
	})
	c.ListIssues(context.Background(), "o", "r", IssueOptions{PerPage: 75})
	if gotPerPage != "75" {
		t.Errorf("got per_page=%q, want 75", gotPerPage)
	}
}

func TestSearchRepos(t *testing.T) {
	want := SearchResult{TotalCount: 42, Items: []Repo{{FullName: "foo/bar", Language: "Go"}}}
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/repositories" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		q := r.URL.Query().Get("q")
		if q != "language:go stars:>1000" {
			t.Errorf("unexpected query: %q", q)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(want)
	})
	got, err := c.SearchRepos(context.Background(), "language:go stars:>1000", SearchOptions{Sort: "stars"})
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalCount != 42 {
		t.Errorf("TotalCount: got %d, want 42", got.TotalCount)
	}
}

func TestQueryParamEncoding(t *testing.T) {
	var gotQ string
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQ = r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SearchResult{})
	})
	c.SearchRepos(context.Background(), "user:foo&malicious=true", SearchOptions{})
	// url.Values.Encode properly escapes the & so it arrives as a single value
	if gotQ != "user:foo&malicious=true" {
		t.Errorf("query not properly encoded: got %q", gotQ)
	}
}
