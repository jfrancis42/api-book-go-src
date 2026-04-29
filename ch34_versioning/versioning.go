package versioning

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

// V1Todo is the v1 response shape — no timestamps, no priority.
type V1Todo struct {
	ID   int64  `json:"id"`
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// V2Todo is the v2 response shape — adds created_at and priority.
type V2Todo struct {
	ID        int64     `json:"id"`
	Text      string    `json:"text"`
	Done      bool      `json:"done"`
	Priority  string    `json:"priority"`
	CreatedAt time.Time `json:"created_at"`
}

// todo is the internal domain model.
type todo struct {
	id        int64
	text      string
	done      bool
	priority  string
	createdAt time.Time
}

func (t *todo) toV1() V1Todo { return V1Todo{ID: t.id, Text: t.text, Done: t.done} }
func (t *todo) toV2() V2Todo {
	return V2Todo{ID: t.id, Text: t.text, Done: t.done, Priority: t.priority, CreatedAt: t.createdAt}
}

type Store struct {
	mu     sync.RWMutex
	todos  []*todo
	nextID int64
}

func NewStore() *Store { return &Store{nextID: 1} }

func (s *Store) Add(text string) *todo {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := &todo{id: s.nextID, text: text, createdAt: time.Now()}
	s.todos = append(s.todos, t)
	s.nextID++
	return t
}

func (s *Store) All() []*todo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*todo, len(s.todos))
	copy(out, s.todos)
	return out
}

// Deprecated middleware adds Deprecation and Sunset headers to all wrapped routes.
func Deprecated(sunsetDate string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Deprecation", "true")
			w.Header().Set("Sunset", sunsetDate)
			w.Header().Set("Link", `<https://api.example.com/docs/migration>; rel="successor-version"`)
			next.ServeHTTP(w, r)
		})
	}
}

// Gone returns a 410 handler for retired API versions.
func Gone(message string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusGone, map[string]string{
			"error": message,
			"code":  "API_VERSION_REMOVED",
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// BuildRouter returns a chi router with /api/v1 (deprecated) and /api/v2.
func BuildRouter(store *Store) http.Handler {
	r := chi.NewRouter()

	// V1: deprecated, will sunset 2026-01-01.
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(Deprecated("Sat, 01 Jan 2026 00:00:00 GMT"))

		r.Get("/todos", func(w http.ResponseWriter, r *http.Request) {
			all := store.All()
			out := make([]V1Todo, len(all))
			for i, t := range all {
				out[i] = t.toV1()
			}
			writeJSON(w, http.StatusOK, out)
		})

		r.Post("/todos", func(w http.ResponseWriter, r *http.Request) {
			var req struct{ Text string `json:"text"` }
			json.NewDecoder(r.Body).Decode(&req)
			t := store.Add(req.Text)
			writeJSON(w, http.StatusCreated, t.toV1())
		})
	})

	// V2: current version with richer response shape.
	r.Route("/api/v2", func(r chi.Router) {
		r.Get("/todos", func(w http.ResponseWriter, r *http.Request) {
			all := store.All()
			out := make([]V2Todo, len(all))
			for i, t := range all {
				out[i] = t.toV2()
			}
			writeJSON(w, http.StatusOK, out)
		})

		r.Post("/todos", func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				Text     string `json:"text"`
				Priority string `json:"priority"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			t := store.Add(req.Text)
			t.priority = req.Priority
			writeJSON(w, http.StatusCreated, t.toV2())
		})
	})

	// Retired V0: returns 410.
	r.Get("/api/v0/*", Gone("API v0 was retired on 2025-01-01. Please migrate to v2: https://api.example.com/docs/migration"))

	return r
}
