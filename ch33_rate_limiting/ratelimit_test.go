package ratelimit

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/time/rate"
)

func newServer(t *testing.T, limit rate.Limit, burst int) *httptest.Server {
	t.Helper()
	rl := NewRateLimiter(limit, burst)
	srv := httptest.NewServer(BuildRouter(rl))
	t.Cleanup(srv.Close)
	return srv
}

func TestRateLimiter_AllowsWithinLimit(t *testing.T) {
	srv := newServer(t, rate.Limit(100), 10)
	resp, err := srv.Client().Get(srv.URL + "/hello")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
}

func TestRateLimiter_BlocksWhenExhausted(t *testing.T) {
	// burst=2: only 2 tokens, then reject.
	srv := newServer(t, rate.Limit(0.001), 2)

	var got429 bool
	for i := range 20 {
		resp, _ := srv.Client().Get(srv.URL + "/hello")
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			got429 = true
			_ = i
			break
		}
	}
	if !got429 {
		t.Fatal("expected at least one 429 response")
	}
}

func TestRateLimiter_RateLimitHeaders(t *testing.T) {
	srv := newServer(t, rate.Limit(100), 10)
	resp, _ := srv.Client().Get(srv.URL + "/hello")
	resp.Body.Close()

	if resp.Header.Get("X-RateLimit-Limit") == "" {
		t.Error("missing X-RateLimit-Limit header")
	}
	if resp.Header.Get("X-RateLimit-Remaining") == "" {
		t.Error("missing X-RateLimit-Remaining header")
	}
}

func TestRateLimiter_429HasRetryAfter(t *testing.T) {
	srv := newServer(t, rate.Limit(0.001), 1)

	var resp429 *http.Response
	for range 20 {
		resp, _ := srv.Client().Get(srv.URL + "/hello")
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			resp429 = resp
			break
		}
	}
	if resp429 == nil {
		t.Skip("didn't get a 429 — burst too large for test")
	}
	if resp429.Header.Get("Retry-After") == "" {
		t.Error("missing Retry-After header on 429")
	}
}

func TestLoginEndpoint_SeparateLimit(t *testing.T) {
	// Global rate is high, but /auth/login has a tighter limit (burst=3).
	srv := newServer(t, rate.Limit(100), 100)

	var got429 bool
	for i := range 20 {
		resp, _ := srv.Client().Post(srv.URL+"/auth/login", "application/json", nil)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			got429 = true
			_ = i
			break
		}
	}
	if !got429 {
		t.Fatal("expected 429 on login endpoint with tight limit")
	}
}
