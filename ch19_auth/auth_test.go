package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestStaticTokenSource(t *testing.T) {
	src := NewStaticTokenSource("ghp_testtoken")
	tok, err := src.Token(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "ghp_testtoken" {
		t.Fatalf("got %q, want %q", tok, "ghp_testtoken")
	}
}

func TestStaticTokenSourceEmpty(t *testing.T) {
	src := NewStaticTokenSource("")
	_, err := src.Token(context.Background())
	if err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestCachedTokenSource_FetchesOnce(t *testing.T) {
	var calls atomic.Int32
	src := NewCachedTokenSource(func(ctx context.Context) (string, time.Time, error) {
		calls.Add(1)
		return "tok1", time.Now().Add(time.Hour), nil
	})

	for range 10 {
		tok, err := src.Token(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tok != "tok1" {
			t.Fatalf("got %q, want %q", tok, "tok1")
		}
	}

	if n := calls.Load(); n != 1 {
		t.Fatalf("fetch called %d times, want 1", n)
	}
}

func TestCachedTokenSource_RefreshesExpired(t *testing.T) {
	var calls atomic.Int32
	src := NewCachedTokenSource(func(ctx context.Context) (string, time.Time, error) {
		n := calls.Add(1)
		// first token expires in the past; second valid for an hour
		if n == 1 {
			return "tok-old", time.Now().Add(-time.Second), nil
		}
		return "tok-new", time.Now().Add(time.Hour), nil
	})

	// Prime the cache with an already-expired token.
	src.Token(context.Background())

	tok, err := src.Token(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "tok-new" {
		t.Fatalf("got %q, want %q", tok, "tok-new")
	}
	if n := calls.Load(); n != 2 {
		t.Fatalf("fetch called %d times, want 2", n)
	}
}

func TestCachedTokenSource_PropagatesError(t *testing.T) {
	src := NewCachedTokenSource(func(ctx context.Context) (string, time.Time, error) {
		return "", time.Time{}, fmt.Errorf("credential server unreachable")
	})
	_, err := src.Token(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAuthTransport_InjectsHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	src := NewStaticTokenSource("my-secret-token")
	client := &http.Client{
		Transport: NewAuthTransport(http.DefaultTransport, src),
	}

	resp, err := client.Get(srv.URL + "/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	want := "Bearer my-secret-token"
	if gotAuth != want {
		t.Fatalf("Authorization header: got %q, want %q", gotAuth, want)
	}
}

func TestAuthTransport_DoesNotMutateOriginalRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	src := NewStaticTokenSource("tok")
	transport := NewAuthTransport(http.DefaultTransport, src)

	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	// Original request must not have been modified.
	if req.Header.Get("Authorization") != "" {
		t.Fatal("RoundTrip mutated the original request")
	}
}

func TestNewAuthClient(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewAuthClient(NewStaticTokenSource("pat-abc"))
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	if gotAuth != "Bearer pat-abc" {
		t.Fatalf("got %q, want %q", gotAuth, "Bearer pat-abc")
	}
}
