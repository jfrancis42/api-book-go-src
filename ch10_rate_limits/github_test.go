package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func TestRateLimitHeadersParsed(t *testing.T) {
	reset := time.Now().Add(30 * time.Minute).Unix()
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.Header().Set("X-RateLimit-Remaining", "4999")
		w.Header().Set("X-RateLimit-Used", "1")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", reset))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(User{Login: "test"})
	})
	c.GetUser(context.Background(), "test")

	rl := c.RateLimit()
	if rl.Limit != 5000 {
		t.Errorf("Limit: got %d, want 5000", rl.Limit)
	}
	if rl.Remaining != 4999 {
		t.Errorf("Remaining: got %d, want 4999", rl.Remaining)
	}
}

func TestRateLimitedResponse(t *testing.T) {
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0") // instant reset for test
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	})
	_, err := c.GetUser(context.Background(), "test")
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("expected ErrRateLimited, got %v", err)
	}
}

func TestProactiveRateLimit_ContextCancel(t *testing.T) {
	// Set up client as if already rate limited with a long reset time
	c := NewClient("test-token")
	c.rateLimit = RateLimit{
		Limit:     5000,
		Remaining: 0,
		Reset:     time.Now().Add(10 * time.Minute),
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request should not have been made")
	}))
	defer srv.Close()
	setBaseURL(srv.URL)
	defer setBaseURL("https://api.github.com")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := c.GetUser(ctx, "test")
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestParseRateLimit(t *testing.T) {
	h := http.Header{}
	h.Set("X-RateLimit-Limit", "60")
	h.Set("X-RateLimit-Remaining", "59")
	h.Set("X-RateLimit-Reset", "1700000000")

	rl := parseRateLimit(h)
	if rl.Limit != 60 {
		t.Errorf("Limit: got %d", rl.Limit)
	}
	if rl.Remaining != 59 {
		t.Errorf("Remaining: got %d", rl.Remaining)
	}
	if rl.Reset.Unix() != 1700000000 {
		t.Errorf("Reset: got %v", rl.Reset)
	}
}
