package caching

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func newServer(t *testing.T) (*httptest.Server, *TodoRepo) {
	t.Helper()
	cache := NewCache()
	repo := NewTodoRepo(cache)
	srv := httptest.NewServer(BuildRouter(repo))
	t.Cleanup(srv.Close)
	return srv, repo
}

func createTodo(t *testing.T, srv *httptest.Server) *Todo {
	t.Helper()
	resp, _ := srv.Client().Post(srv.URL+"/todos", "application/json",
		strings.NewReader(`{"text":"cache test"}`))
	var todo Todo
	json.NewDecoder(resp.Body).Decode(&todo)
	resp.Body.Close()
	return &todo
}

func TestETag_FirstRequestSetsHeader(t *testing.T) {
	srv, _ := newServer(t)
	todo := createTodo(t, srv)

	resp, _ := srv.Client().Get(fmt.Sprintf("%s/todos/%d", srv.URL, todo.ID))
	defer resp.Body.Close()

	if etag := resp.Header.Get("ETag"); etag == "" {
		t.Fatal("expected ETag header on first request")
	}
}

func TestETag_MatchingReturns304(t *testing.T) {
	srv, _ := newServer(t)
	todo := createTodo(t, srv)

	// Get ETag.
	resp1, _ := srv.Client().Get(fmt.Sprintf("%s/todos/%d", srv.URL, todo.ID))
	io.Copy(io.Discard, resp1.Body)
	resp1.Body.Close()
	etag := resp1.Header.Get("ETag")

	// Send If-None-Match.
	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/todos/%d", srv.URL, todo.ID), nil)
	req.Header.Set("If-None-Match", etag)
	resp2, _ := srv.Client().Do(req)
	resp2.Body.Close()

	if resp2.StatusCode != http.StatusNotModified {
		t.Fatalf("got %d, want 304", resp2.StatusCode)
	}
}

func TestETag_MismatchReturns200(t *testing.T) {
	srv, _ := newServer(t)
	todo := createTodo(t, srv)

	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/todos/%d", srv.URL, todo.ID), nil)
	req.Header.Set("If-None-Match", `"stale-etag"`)
	resp, _ := srv.Client().Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
}

func TestCacheAside_HitAvoidsDBFetch(t *testing.T) {
	srv, repo := newServer(t)
	todo := createTodo(t, srv)

	// First GET — DB fetch.
	srv.Client().Get(fmt.Sprintf("%s/todos/%d", srv.URL, todo.ID))
	firstCount := repo.FetchCount.Load()

	// Second GET — should be served from cache.
	srv.Client().Get(fmt.Sprintf("%s/todos/%d", srv.URL, todo.ID))
	secondCount := repo.FetchCount.Load()

	if secondCount != firstCount {
		t.Fatalf("DB fetch count changed (got %d → %d): cache miss on second request",
			firstCount, secondCount)
	}
}

func TestSingleflight_OnlyOneConcurrentFetch(t *testing.T) {
	cache := NewCache()
	repo := NewTodoRepo(cache)
	todo := repo.Create("singleflight test")

	var wg sync.WaitGroup
	const n = 50
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			repo.Get(todo.ID)
		}()
	}
	wg.Wait()

	// Despite 50 concurrent callers, the DB should have been queried very few times.
	// With singleflight, all concurrent callers share one fetch, so count <= 2
	// (once on first burst, possibly once more after singleflight clears).
	if count := repo.FetchCount.Load(); count > 3 {
		t.Fatalf("expected <=3 DB fetches with singleflight, got %d", count)
	}
}

func TestCacheControl_PrivateHeader(t *testing.T) {
	srv, _ := newServer(t)
	todo := createTodo(t, srv)

	resp, _ := srv.Client().Get(fmt.Sprintf("%s/todos/%d", srv.URL, todo.ID))
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	cc := resp.Header.Get("Cache-Control")
	if !strings.Contains(cc, "private") {
		t.Fatalf("Cache-Control should be private for user-specific resources, got %q", cc)
	}
}

func TestCache_TTLExpiry(t *testing.T) {
	c := NewCache()
	c.Set("key", "value", 10*time.Millisecond)

	if v, ok := c.Get("key"); !ok || v != "value" {
		t.Fatal("expected cache hit immediately after set")
	}
	time.Sleep(20 * time.Millisecond)
	if _, ok := c.Get("key"); ok {
		t.Fatal("expected cache miss after TTL expired")
	}
}

func TestCache_Delete(t *testing.T) {
	c := NewCache()
	c.Set("k", 42, time.Hour)
	c.Delete("k")
	if _, ok := c.Get("k"); ok {
		t.Fatal("expected cache miss after delete")
	}
}
