package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func withServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(func() { srv.Close(); setBaseURL("https://api.github.com") })
	setBaseURL(srv.URL)
	return NewClient("test-token")
}

func TestNewClientFromEnv_NoToken(t *testing.T) {
	os.Unsetenv("GITHUB_TOKEN")
	_, err := NewClientFromEnv()
	if err == nil {
		t.Fatal("expected error when GITHUB_TOKEN is not set")
	}
}

func TestNewClientFromEnv_WithToken(t *testing.T) {
	os.Setenv("GITHUB_TOKEN", "test-token-123")
	t.Cleanup(func() { os.Unsetenv("GITHUB_TOKEN") })

	c, err := NewClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if c.token != "test-token-123" {
		t.Errorf("token mismatch: got %q", c.token)
	}
}

func TestAuthHeaderSent(t *testing.T) {
	var gotAuth string
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(User{Login: "test"})
	})
	c.GetUser(context.Background(), "test")
	if gotAuth != "Bearer test-token" {
		t.Errorf("got Authorization: %q", gotAuth)
	}
}

func TestGetAuthenticatedUser(t *testing.T) {
	want := User{Login: "me", Name: "Me Myself"}
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(want)
	})
	got, err := c.GetAuthenticatedUser(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Login != want.Login {
		t.Errorf("got %q, want %q", got.Login, want.Login)
	}
}
