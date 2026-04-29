package github

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func init() {
	// Use zero jitter for deterministic tests
	BackoffFunc = func(attempt int) time.Duration {
		if attempt == 0 {
			return 0
		}
		return time.Millisecond
	}
}

func withServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(func() { srv.Close(); setBaseURL("https://api.github.com") })
	setBaseURL(srv.URL)
	return NewClient("test-token")
}

func TestRetry_SucceedsOnSecondAttempt(t *testing.T) {
	var callCount int32
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		if n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"message":"try again"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(User{Login: "octocat"})
	})
	user, err := c.GetUser(context.Background(), "test")
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if user.Login != "octocat" {
		t.Errorf("login: got %q", user.Login)
	}
	if atomic.LoadInt32(&callCount) != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
}

func TestRetry_NoRetryOn404(t *testing.T) {
	var callCount int32
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`))
	})
	_, err := c.GetUser(context.Background(), "nobody")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound")
	}
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected exactly 1 call (no retry), got %d", callCount)
	}
}

func TestRetry_ExhaustsMaxRetries(t *testing.T) {
	var callCount int32
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message":"boom"}`))
	})
	c.maxRetries = 2

	_, err := c.GetUser(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if atomic.LoadInt32(&callCount) != 3 { // 1 initial + 2 retries
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestRetry_ContextCancel(t *testing.T) {
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"message":"down"}`))
	})
	BackoffFunc = func(attempt int) time.Duration { return 100 * time.Millisecond }
	defer func() {
		BackoffFunc = func(attempt int) time.Duration { return 0 }
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := c.GetUser(ctx, "test")
	if err == nil {
		t.Fatal("expected error from context cancellation")
	}
}

func TestWithMaxRetries(t *testing.T) {
	c := NewClient("token", WithMaxRetries(7))
	if c.maxRetries != 7 {
		t.Errorf("maxRetries: got %d, want 7", c.maxRetries)
	}
}
