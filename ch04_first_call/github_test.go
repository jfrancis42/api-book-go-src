package github

import (
	"context"
	"encoding/json"
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

func TestGetZen(t *testing.T) {
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zen" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Write([]byte("Practicality beats purity."))
	})
	zen, err := c.GetZen(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if zen != "Practicality beats purity." {
		t.Errorf("got %q", zen)
	}
}

func TestGetUser(t *testing.T) {
	want := User{Login: "octocat", Name: "The Octocat", Followers: 9001}
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/octocat" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(want)
	})
	got, err := c.GetUser(context.Background(), "octocat")
	if err != nil {
		t.Fatal(err)
	}
	if got.Login != want.Login || got.Followers != want.Followers {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestGetUser_NotFound(t *testing.T) {
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`))
	})
	_, err := c.GetUser(context.Background(), "nobody")
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
}

func TestNoAuthHeaderForEmptyToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("expected no Authorization header for empty token")
		}
		w.Write([]byte("Keep it logically awesome."))
	}))
	t.Cleanup(func() { srv.Close(); setBaseURL("https://api.github.com") })
	setBaseURL(srv.URL)

	c := NewClient("") // no token
	_, err := c.GetZen(context.Background())
	if err != nil {
		t.Fatal(err)
	}
}

func TestGetRepo(t *testing.T) {
	want := Repo{FullName: "golang/go", Language: "Go", StargazersCount: 123}
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/golang/go" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(want)
	})
	got, err := c.GetRepo(context.Background(), "golang", "go")
	if err != nil {
		t.Fatal(err)
	}
	if got.FullName != want.FullName || got.Language != want.Language {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
