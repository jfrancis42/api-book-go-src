package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
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

func TestETag_304OnSecondRequest(t *testing.T) {
	var callCount int32
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		if r.Header.Get("If-None-Match") == `"etag-v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"etag-v1"`)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Repo{FullName: "golang/go", Language: "Go"})
	})

	r1, err := c.GetRepo(context.Background(), "golang", "go")
	if err != nil || r1.Language != "Go" {
		t.Fatalf("first call failed: %v", err)
	}
	r2, err := c.GetRepo(context.Background(), "golang", "go")
	if err != nil || r2.Language != "Go" {
		t.Fatalf("second call failed: %v", err)
	}
	if atomic.LoadInt32(&callCount) != 2 {
		t.Errorf("expected 2 HTTP calls, got %d", callCount)
	}
	stats := c.CacheStats()
	if stats.Revalidations != 1 {
		t.Errorf("revalidations: got %d, want 1", stats.Revalidations)
	}
}

func TestFreshness_SkipsRequest(t *testing.T) {
	var callCount int32
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Repo{FullName: "test/repo"})
	})

	c.GetRepo(context.Background(), "test", "repo")
	c.GetRepo(context.Background(), "test", "repo") // should use cache

	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected 1 HTTP call (second should use fresh cache), got %d", callCount)
	}
	stats := c.CacheStats()
	if stats.Hits != 1 {
		t.Errorf("hits: got %d, want 1", stats.Hits)
	}
}

func TestClearCache(t *testing.T) {
	var callCount int32
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Repo{FullName: "test/repo"})
	})

	c.GetRepo(context.Background(), "test", "repo")
	c.ClearCache()
	c.GetRepo(context.Background(), "test", "repo")

	if atomic.LoadInt32(&callCount) != 2 {
		t.Errorf("expected 2 HTTP calls after cache clear, got %d", callCount)
	}
}

func TestParseMaxAge(t *testing.T) {
	tests := []struct {
		header string
		want   int
	}{
		{"max-age=60", 60},
		{"public, max-age=3600", 3600},
		{"no-cache", 0},
		{"", 0},
	}
	for _, tt := range tests {
		got := parseMaxAge(tt.header)
		if got != tt.want {
			t.Errorf("parseMaxAge(%q) = %d, want %d", tt.header, got, tt.want)
		}
	}
}

func TestExpiredCache_Revalidates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != `"old-etag"` {
			t.Errorf("expected If-None-Match header, got %q", r.Header.Get("If-None-Match"))
		}
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()
	setBaseURL(srv.URL)
	defer setBaseURL("https://api.github.com")

	c := NewClient("token")
	// Pre-seed with an expired entry (must be after setBaseURL so key matches)
	url := apiBaseURL + "/repos/old/repo"
	c.cache[url] = cacheEntry{
		etag:      `"old-etag"`,
		body:      []byte(`{"full_name":"old/repo"}`),
		expiresAt: time.Now().Add(-1 * time.Hour), // expired
	}

	_, err := c.GetRepo(context.Background(), "old", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if c.revalidations.Load() != 1 {
		t.Error("expected a revalidation request")
	}
}
