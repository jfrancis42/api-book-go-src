package performance

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newServer(t *testing.T) (*httptest.Server, *Store) {
	t.Helper()
	store := NewStore()
	srv := httptest.NewServer(BuildRouter(store))
	t.Cleanup(srv.Close)
	return srv, store
}

func populateStore(s *Store) {
	for i := range 5 {
		todo := s.AddTodo("todo", int64(i+1))
		s.AddTag(todo.ID, "work")
		s.AddTag(todo.ID, "urgent")
	}
}

func TestNaive_Returns200(t *testing.T) {
	srv, store := newServer(t)
	populateStore(store)

	resp, _ := srv.Client().Get(srv.URL + "/todos/naive")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
}

func TestBatched_Returns200(t *testing.T) {
	srv, store := newServer(t)
	populateStore(store)

	resp, _ := srv.Client().Get(srv.URL + "/todos/batched")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
}

func TestNaiveAndBatched_SameResultCount(t *testing.T) {
	srv, store := newServer(t)
	populateStore(store)

	getCount := func(url string) int {
		resp, _ := srv.Client().Get(url)
		defer resp.Body.Close()
		var out []any
		json.NewDecoder(resp.Body).Decode(&out)
		return len(out)
	}

	naiveCount := getCount(srv.URL + "/todos/naive")
	batchedCount := getCount(srv.URL + "/todos/batched")

	if naiveCount != batchedCount {
		t.Fatalf("naive=%d batched=%d, want equal", naiveCount, batchedCount)
	}
}

func TestBatched_IncludesTags(t *testing.T) {
	srv, store := newServer(t)
	todo := store.AddTodo("tagged todo", 1)
	store.AddTag(todo.ID, "important")

	resp, _ := srv.Client().Get(srv.URL + "/todos/batched")
	defer resp.Body.Close()

	var out []map[string]any
	json.NewDecoder(resp.Body).Decode(&out)

	if len(out) == 0 {
		t.Fatal("expected at least one result")
	}

	body, _ := json.Marshal(out)
	if !strings.Contains(string(body), "important") {
		t.Fatalf("expected tag 'important' in response: %s", body)
	}
}

func TestPprof_EndpointAccessible(t *testing.T) {
	srv, _ := newServer(t)
	resp, _ := srv.Client().Get(srv.URL + "/debug/pprof/")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
}

// BenchmarkNaive measures throughput of the N+1 approach.
func BenchmarkNaive(b *testing.B) {
	store := NewStore()
	for range 100 {
		todo := store.AddTodo("bench todo", 1)
		store.AddTag(todo.ID, "tag1")
		store.AddTag(todo.ID, "tag2")
	}

	srv := httptest.NewServer(BuildRouter(store))
	defer srv.Close()

	b.ResetTimer()
	for range b.N {
		resp, _ := srv.Client().Get(srv.URL + "/todos/naive")
		resp.Body.Close()
	}
}

// BenchmarkBatched measures throughput of the batched (join-style) approach.
func BenchmarkBatched(b *testing.B) {
	store := NewStore()
	for range 100 {
		todo := store.AddTodo("bench todo", 1)
		store.AddTag(todo.ID, "tag1")
		store.AddTag(todo.ID, "tag2")
	}

	srv := httptest.NewServer(BuildRouter(store))
	defer srv.Close()

	b.ResetTimer()
	for range b.N {
		resp, _ := srv.Client().Get(srv.URL + "/todos/batched")
		resp.Body.Close()
	}
}
