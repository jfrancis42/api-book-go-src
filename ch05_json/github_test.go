package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func withServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(func() { srv.Close(); setBaseURL("https://api.github.com") })
	setBaseURL(srv.URL)
	return NewClient("test-token")
}

func TestGetUser_WithDates(t *testing.T) {
	want := User{
		Login:     "octocat",
		Name:      "The Octocat",
		Followers: 9001,
		CreatedAt: time.Date(2011, 1, 25, 18, 44, 36, 0, time.UTC),
	}
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(want)
	})
	got, err := c.GetUser(context.Background(), "octocat")
	if err != nil {
		t.Fatal(err)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("CreatedAt: got %v, want %v", got.CreatedAt, want.CreatedAt)
	}
}

func TestGetRepo_NullLicense(t *testing.T) {
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"full_name":"test/repo","license":null}`))
	})
	got, err := c.GetRepo(context.Background(), "test", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if got.License != nil {
		t.Error("expected nil license")
	}
}

func TestGetRepo_WithLicense(t *testing.T) {
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"full_name":"golang/go","license":{"key":"bsd-3-clause","name":"BSD 3-Clause"},"language":"Go"}`))
	})
	got, err := c.GetRepo(context.Background(), "golang", "go")
	if err != nil {
		t.Fatal(err)
	}
	if got.License == nil {
		t.Fatal("expected non-nil license")
	}
	if got.License.Key != "bsd-3-clause" {
		t.Errorf("license key: got %q", got.License.Key)
	}
}

func TestListUserRepos(t *testing.T) {
	repos := []Repo{
		{FullName: "octocat/hello-world", Language: "Go"},
		{FullName: "octocat/other", Language: "Python"},
	}
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/octocat/repos" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(repos)
	})
	got, err := c.ListUserRepos(context.Background(), "octocat")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("got %d repos, want 2", len(got))
	}
}

func TestRepo_Topics(t *testing.T) {
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"full_name":"test/repo","topics":["go","api","rest"]}`))
	})
	got, err := c.GetRepo(context.Background(), "test", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Topics) != 3 || got.Topics[0] != "go" {
		t.Errorf("unexpected topics: %v", got.Topics)
	}
}
