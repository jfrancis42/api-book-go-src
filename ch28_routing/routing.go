package routing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

type Todo struct {
	ID   int64  `json:"id"`
	Text string `json:"text"`
	Done bool   `json:"done"`
}

type contextKey string

const todoKey contextKey = "todo"

type Store struct {
	mu     sync.RWMutex
	todos  map[int64]*Todo
	nextID int64
	start  time.Time
}

func NewStore() *Store {
	return &Store{todos: make(map[int64]*Todo), nextID: 1, start: time.Now()}
}

func (s *Store) List() []*Todo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Todo, 0, len(s.todos))
	for _, t := range s.todos {
		out = append(out, t)
	}
	return out
}

func (s *Store) Create(text string) *Todo {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := &Todo{ID: s.nextID, Text: text}
	s.todos[s.nextID] = t
	s.nextID++
	return t
}

func (s *Store) Get(id int64) (*Todo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.todos[id]
	return t, ok
}

func (s *Store) Uptime() time.Duration {
	return time.Since(s.start)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// TodoCtx loads a Todo from the store and attaches it to the request context.
// Any route inside this middleware can call todoFromCtx(r) instead of fetching again.
func TodoCtx(store *Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
			if err != nil || id <= 0 {
				writeError(w, http.StatusBadRequest, "invalid id")
				return
			}
			todo, ok := store.Get(id)
			if !ok {
				writeError(w, http.StatusNotFound, "not found")
				return
			}
			ctx := context.WithValue(r.Context(), todoKey, todo)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func todoFromCtx(r *http.Request) *Todo {
	return r.Context().Value(todoKey).(*Todo)
}

// adminRouter returns a subrouter for admin-only endpoints.
func adminRouter(store *Store) chi.Router {
	r := chi.NewRouter()
	// In a real app, you'd add RequireAdmin middleware here.
	r.Get("/todos", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, store.List())
	})
	return r
}

// BuildRouter assembles a chi router demonstrating routing patterns.
func BuildRouter(store *Store) http.Handler {
	r := chi.NewRouter()

	r.Use(chimw.RequestID)
	r.Use(chimw.Logger)

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "route not found")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	})

	// Health endpoint — no auth needed.
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":          "ok",
			"uptime_seconds":  int(store.Uptime().Seconds()),
		})
	})

	// API v1 route group.
	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/todos", func(r chi.Router) {
			r.Get("/", func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, http.StatusOK, store.List())
			})
			r.Post("/", func(w http.ResponseWriter, r *http.Request) {
				var req struct{ Text string `json:"text"` }
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Text == "" {
					writeError(w, http.StatusBadRequest, "text is required")
					return
				}
				t := store.Create(req.Text)
				w.Header().Set("Location", fmt.Sprintf("/api/v1/todos/%d", t.ID))
				writeJSON(w, http.StatusCreated, t)
			})

			// Nested /{id} routes with TodoCtx middleware.
			r.Route("/{id}", func(r chi.Router) {
				r.Use(TodoCtx(store))
				r.Get("/", func(w http.ResponseWriter, r *http.Request) {
					writeJSON(w, http.StatusOK, todoFromCtx(r))
				})
				r.Post("/done", func(w http.ResponseWriter, r *http.Request) {
					todo := todoFromCtx(r)
					todo.Done = true
					writeJSON(w, http.StatusOK, todo)
				})
			})
		})
	})

	// Admin subrouter mounted at /admin.
	r.Mount("/admin", adminRouter(store))

	return r
}
