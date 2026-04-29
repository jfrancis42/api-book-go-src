package caching

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/sync/singleflight"
)

// CacheEntry holds a cached value and its expiry.
type CacheEntry struct {
	Value     any
	ExpiresAt time.Time
}

// Cache is a thread-safe in-memory TTL cache.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]CacheEntry
}

func NewCache() *Cache {
	c := &Cache{entries: make(map[string]CacheEntry)}
	go c.cleanup()
	return c
}

func (c *Cache) Get(key string) (any, bool) {
	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.ExpiresAt) {
		return nil, false
	}
	return e.Value, true
}

func (c *Cache) Set(key string, value any, ttl time.Duration) {
	c.mu.Lock()
	c.entries[key] = CacheEntry{Value: value, ExpiresAt: time.Now().Add(ttl)}
	c.mu.Unlock()
}

func (c *Cache) Delete(key string) {
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

func (c *Cache) cleanup() {
	for range time.Tick(time.Minute) {
		c.mu.Lock()
		for k, e := range c.entries {
			if time.Now().After(e.ExpiresAt) {
				delete(c.entries, k)
			}
		}
		c.mu.Unlock()
	}
}

// Todo is the domain type.
type Todo struct {
	ID        int64     `json:"id"`
	Text      string    `json:"text"`
	Done      bool      `json:"done"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TodoRepo demonstrates cache-aside + singleflight.
type TodoRepo struct {
	mu      sync.RWMutex
	todos   map[int64]*Todo
	nextID  int64
	cache   *Cache
	sf      singleflight.Group
	FetchCount atomic.Int64 // counts DB fetches for testing
}

func NewTodoRepo(cache *Cache) *TodoRepo {
	return &TodoRepo{todos: make(map[int64]*Todo), nextID: 1, cache: cache}
}

func (r *TodoRepo) Create(text string) *Todo {
	r.mu.Lock()
	defer r.mu.Unlock()
	t := &Todo{ID: r.nextID, Text: text, UpdatedAt: time.Now()}
	r.todos[r.nextID] = t
	r.nextID++
	return t
}

// Get demonstrates cache-aside + singleflight to prevent stampedes.
func (r *TodoRepo) Get(id int64) (*Todo, bool) {
	key := fmt.Sprintf("todo:%d", id)

	if v, ok := r.cache.Get(key); ok {
		return v.(*Todo), true
	}

	// singleflight: concurrent callers for the same key share one DB fetch.
	v, err, _ := r.sf.Do(key, func() (any, error) {
		r.FetchCount.Add(1)

		r.mu.RLock()
		t, ok := r.todos[id]
		r.mu.RUnlock()
		if !ok {
			return nil, nil
		}
		r.cache.Set(key, t, 30*time.Second)
		return t, nil
	})
	if err != nil || v == nil {
		return nil, false
	}
	return v.(*Todo), true
}

// bufferedResponseWriter captures the response body for ETag hashing.
type bufferedResponseWriter struct {
	http.ResponseWriter
	buf    *bytes.Buffer
	status int
}

func (b *bufferedResponseWriter) WriteHeader(s int) {
	b.status = s
	// Don't call ResponseWriter.WriteHeader yet — we need to add the ETag first.
}

func (b *bufferedResponseWriter) Write(p []byte) (int, error) {
	return b.buf.Write(p)
}

// WithETag is a middleware that computes an ETag from the response body and
// returns 304 Not Modified if the client's If-None-Match matches.
func WithETag(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &bufferedResponseWriter{ResponseWriter: w, buf: &bytes.Buffer{}, status: http.StatusOK}
		next.ServeHTTP(rw, r)

		if rw.status == http.StatusOK {
			h := sha256.Sum256(rw.buf.Bytes())
			etag := fmt.Sprintf(`"%x"`, h[:8])

			w.Header().Set("ETag", etag)
			if r.Header.Get("If-None-Match") == etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}

		w.WriteHeader(rw.status)
		w.Write(rw.buf.Bytes())
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// BuildRouter returns a chi router demonstrating HTTP caching and singleflight.
func BuildRouter(repo *TodoRepo) http.Handler {
	r := chi.NewRouter()

	r.Post("/todos", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Text string `json:"text"` }
		json.NewDecoder(r.Body).Decode(&req)
		writeJSON(w, http.StatusCreated, repo.Create(req.Text))
	})

	// ETag caching on individual todo reads.
	r.With(WithETag).Get("/todos/{id}", func(w http.ResponseWriter, r *http.Request) {
		var id int64
		fmt.Sscanf(chi.URLParam(r, "id"), "%d", &id)

		todo, ok := repo.Get(id)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// Private cache: 30 seconds in browser only.
		w.Header().Set("Cache-Control", "private, max-age=30")
		writeJSON(w, http.StatusOK, todo)
	})

	return r
}
