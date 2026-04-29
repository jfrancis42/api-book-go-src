package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func withServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(func() {
		srv.Close()
		setBaseURL("https://api.github.com")
	})
	setBaseURL(srv.URL)
	return NewClient("test-token")
}

const testFeedXML = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Release notes from go</title>
  <entry>
    <id>tag:github.com,2013:v1.22.0</id>
    <title>Go 1.22</title>
    <updated>2024-02-06T00:00:00Z</updated>
    <link rel="alternate" href="https://github.com/golang/go/releases/tag/v1.22.0"/>
  </entry>
  <entry>
    <id>tag:github.com,2013:v1.21.0</id>
    <title>Go 1.21</title>
    <updated>2023-08-08T00:00:00Z</updated>
    <link rel="alternate" href="https://github.com/golang/go/releases/tag/v1.21.0"/>
  </entry>
</feed>`

func TestGetReleases_ParsesFeed(t *testing.T) {
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		w.Write([]byte(testFeedXML))
	})

	feed, err := c.GetReleases(context.Background(), "golang", "go")
	if err != nil {
		t.Fatal(err)
	}

	if feed.Title != "Release notes from go" {
		t.Errorf("title: got %q, want %q", feed.Title, "Release notes from go")
	}
	if len(feed.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(feed.Entries))
	}
	if feed.Entries[0].Title != "Go 1.22" {
		t.Errorf("entry[0] title: got %q, want %q", feed.Entries[0].Title, "Go 1.22")
	}
	if feed.Entries[1].Title != "Go 1.21" {
		t.Errorf("entry[1] title: got %q, want %q", feed.Entries[1].Title, "Go 1.21")
	}
}

func TestGetReleases_EntryURL(t *testing.T) {
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		w.Write([]byte(testFeedXML))
	})

	feed, err := c.GetReleases(context.Background(), "golang", "go")
	if err != nil {
		t.Fatal(err)
	}

	want := "https://github.com/golang/go/releases/tag/v1.22.0"
	if got := feed.Entries[0].URL(); got != want {
		t.Errorf("URL(): got %q, want %q", got, want)
	}
}

func TestGetReleases_EntryUpdatedTime(t *testing.T) {
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		w.Write([]byte(testFeedXML))
	})

	feed, err := c.GetReleases(context.Background(), "golang", "go")
	if err != nil {
		t.Fatal(err)
	}

	got := feed.Entries[0].Updated
	if got.Year() != 2024 || got.Month() != 2 || got.Day() != 6 {
		t.Errorf("updated: got %v, want 2024-02-06", got)
	}
}

func TestGetReleases_404(t *testing.T) {
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_, err := c.GetReleases(context.Background(), "no", "repo")
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestGetReleases_RequestPath(t *testing.T) {
	var gotPath string
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/atom+xml")
		w.Write([]byte(testFeedXML))
	})

	c.GetReleases(context.Background(), "golang", "go")

	if gotPath != "/golang/go/releases.atom" {
		t.Errorf("path: got %q, want %q", gotPath, "/golang/go/releases.atom")
	}
}
