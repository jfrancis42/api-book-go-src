package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func withServer(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(func() { srv.Close(); setBaseURL("https://api.github.com") })
	setBaseURL(srv.URL)
	return NewClient("test-token"), srv
}

func TestParseLinkHeader(t *testing.T) {
	header := `<https://api.github.com/repos/foo/bar/issues?page=2>; rel="next", <https://api.github.com/repos/foo/bar/issues?page=5>; rel="last"`
	links := ParseLinkHeader(header)
	if links["next"] != "https://api.github.com/repos/foo/bar/issues?page=2" {
		t.Errorf("next: got %q", links["next"])
	}
	if links["last"] != "https://api.github.com/repos/foo/bar/issues?page=5" {
		t.Errorf("last: got %q", links["last"])
	}
}

func TestParseLinkHeader_Empty(t *testing.T) {
	links := ParseLinkHeader("")
	if len(links) != 0 {
		t.Errorf("expected empty map, got %v", links)
	}
}

func TestPages_SinglePage(t *testing.T) {
	issues := []Issue{{Number: 1, Title: "bug"}, {Number: 2, Title: "feature"}}
	c, _ := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// No Link header = last page
		json.NewEncoder(w).Encode(issues)
	})

	url := apiBaseURL + "/repos/owner/repo/issues?per_page=100"
	got, err := CollectAll(c.Pages(context.Background(), url))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("got %d issues, want 2", len(got))
	}
}

func TestPages_MultiPage(t *testing.T) {
	c, srv := withServer(t, nil)

	// Build a 3-page server
	page1 := []Issue{{Number: 1}, {Number: 2}}
	page2 := []Issue{{Number: 3}, {Number: 4}}
	page3 := []Issue{{Number: 5}}

	mux := http.NewServeMux()
	mux.HandleFunc("/issues", func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")
		switch page {
		case "", "1":
			w.Header().Set("Link", fmt.Sprintf(`<%s/issues?page=2>; rel="next"`, srv.URL))
			json.NewEncoder(w).Encode(page1)
		case "2":
			w.Header().Set("Link", fmt.Sprintf(`<%s/issues?page=3>; rel="next"`, srv.URL))
			json.NewEncoder(w).Encode(page2)
		case "3":
			json.NewEncoder(w).Encode(page3)
		}
	})
	srv.Config.Handler = mux

	got, err := CollectAll(c.Pages(context.Background(), srv.URL+"/issues"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Errorf("got %d issues, want 5", len(got))
	}
	if got[4].Number != 5 {
		t.Errorf("last issue number: got %d, want 5", got[4].Number)
	}
}

func TestFirst_StopsEarly(t *testing.T) {
	var pagesFetched int
	c, srv := withServer(t, nil)

	mux := http.NewServeMux()
	mux.HandleFunc("/issues", func(w http.ResponseWriter, r *http.Request) {
		pagesFetched++
		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")
		issues := []Issue{{Number: pagesFetched*10 + 1}, {Number: pagesFetched*10 + 2}}
		if page == "" || page == "1" {
			w.Header().Set("Link", fmt.Sprintf(`<%s/issues?page=2>; rel="next"`, srv.URL))
		}
		json.NewEncoder(w).Encode(issues)
	})
	srv.Config.Handler = mux

	got, err := First(3, c.Pages(context.Background(), srv.URL+"/issues"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Errorf("got %d items, want 3", len(got))
	}
	// Should have fetched 2 pages (first page = 2 items, need 1 more from page 2)
	if pagesFetched > 2 {
		t.Errorf("fetched %d pages, expected at most 2", pagesFetched)
	}
}
