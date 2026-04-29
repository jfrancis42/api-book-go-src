package github

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestFetchUsers_Concurrent(t *testing.T) {
	usernames := []string{"alice", "bob", "carol", "dave", "eve"}
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Simulate some latency so concurrency matters
		time.Sleep(10 * time.Millisecond)
		name := r.URL.Path[len("/users/"):]
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(User{Login: name, Name: "User " + name})
	})

	start := time.Now()
	users, err := c.FetchUsers(context.Background(), usernames, 5)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatal(err)
	}
	if len(users) != len(usernames) {
		t.Fatalf("got %d users, want %d", len(users), len(usernames))
	}
	// All 5 concurrent: should finish close to 10ms, not 50ms
	if elapsed > 100*time.Millisecond {
		t.Errorf("too slow (%v): requests may not be concurrent", elapsed)
	}
	// Verify order preserved
	for i, u := range users {
		if u.Login != usernames[i] {
			t.Errorf("users[%d]: got %q, want %q", i, u.Login, usernames[i])
		}
	}
}

func TestFetchUsers_BoundedConcurrency(t *testing.T) {
	var maxConcurrent int32
	var currentConcurrent int32

	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt32(&currentConcurrent, 1)
		defer atomic.AddInt32(&currentConcurrent, -1)

		// Track max seen
		for {
			old := atomic.LoadInt32(&maxConcurrent)
			if current <= old {
				break
			}
			if atomic.CompareAndSwapInt32(&maxConcurrent, old, current) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(User{Login: "test"})
	})

	names := make([]string, 20)
	for i := range names {
		names[i] = fmt.Sprintf("user%d", i)
	}
	_, err := c.FetchUsers(context.Background(), names, 3)
	if err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&maxConcurrent) > 3 {
		t.Errorf("max concurrent: got %d, want ≤3", maxConcurrent)
	}
}

func TestFetchUsers_ErrorPropagates(t *testing.T) {
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/users/bad" {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"message":"Not Found"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(User{Login: "good"})
	})

	_, err := c.FetchUsers(context.Background(), []string{"good", "bad", "good"}, 3)
	if err == nil {
		t.Fatal("expected error when one user not found")
	}
}

func TestFetchRepos_Order(t *testing.T) {
	pairs := [][2]string{{"golang", "go"}, {"kubernetes", "kubernetes"}, {"docker", "compose"}}
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Extract repo name from path
		parts := r.URL.Path // /repos/owner/repo
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Repo{FullName: parts[len("/repos/"):]})
	})

	repos, err := c.FetchRepos(context.Background(), pairs, 3)
	if err != nil {
		t.Fatal(err)
	}
	for i, p := range pairs {
		want := p[0] + "/" + p[1]
		if repos[i].FullName != want {
			t.Errorf("repos[%d]: got %q, want %q", i, repos[i].FullName, want)
		}
	}
}
