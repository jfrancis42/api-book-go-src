package performance

import (
	"encoding/json"
	"net/http"
	_ "net/http/pprof" // registers /debug/pprof endpoints on DefaultServeMux
	"sync"

	"github.com/go-chi/chi/v5"
)

// Todo is the domain type for this chapter's examples.
type Todo struct {
	ID     int64  `json:"id"`
	Text   string `json:"text"`
	Done   bool   `json:"done"`
	UserID int64  `json:"user_id"`
}

// Tag is deliberately separate to illustrate the N+1 query problem.
type Tag struct {
	TodoID int64  `json:"todo_id"`
	Name   string `json:"name"`
}

// Store simulates a database with two separate data stores.
// In production this would be a SQL database; the separation here lets us
// demonstrate N+1 by requiring two lookups per todo.
type Store struct {
	mu     sync.RWMutex
	todos  map[int64]*Todo
	tags   map[int64][]Tag // keyed by todo ID
	nextID int64
}

func NewStore() *Store {
	return &Store{
		todos:  make(map[int64]*Todo),
		tags:   make(map[int64][]Tag),
		nextID: 1,
	}
}

func (s *Store) AddTodo(text string, userID int64) *Todo {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := &Todo{ID: s.nextID, Text: text, UserID: userID}
	s.todos[s.nextID] = t
	s.nextID++
	return t
}

func (s *Store) AddTag(todoID int64, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tags[todoID] = append(s.tags[todoID], Tag{TodoID: todoID, Name: name})
}

// ListTodosNPlusOne demonstrates the N+1 problem: one query for todos,
// then one query per todo to fetch its tags.
func (s *Store) ListTodosNPlusOne() []map[string]any {
	s.mu.RLock()
	todos := make([]*Todo, 0, len(s.todos))
	for _, t := range s.todos {
		todos = append(todos, t)
	}
	s.mu.RUnlock()

	result := make([]map[string]any, 0, len(todos))
	for _, t := range todos {
		// N+1: separate lock acquisition per todo
		s.mu.RLock()
		tags := s.tags[t.ID]
		s.mu.RUnlock()

		tagNames := make([]string, len(tags))
		for i, tag := range tags {
			tagNames[i] = tag.Name
		}
		result = append(result, map[string]any{
			"todo": t,
			"tags": tagNames,
		})
	}
	return result
}

// ListTodosBatched fixes N+1 by loading all tags in a single pass,
// then joining in-memory — the same approach as a SQL JOIN.
func (s *Store) ListTodosBatched() []map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	todos := make([]*Todo, 0, len(s.todos))
	for _, t := range s.todos {
		todos = append(todos, t)
	}

	result := make([]map[string]any, 0, len(todos))
	for _, t := range todos {
		tags := s.tags[t.ID]
		tagNames := make([]string, len(tags))
		for i, tag := range tags {
			tagNames[i] = tag.Name
		}
		result = append(result, map[string]any{
			"todo": t,
			"tags": tagNames,
		})
	}
	return result
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// BuildRouter returns a router with both the naive and batched endpoints,
// plus the pprof debug endpoints.
func BuildRouter(store *Store) http.Handler {
	r := chi.NewRouter()

	r.Get("/todos/naive", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, store.ListTodosNPlusOne())
	})

	r.Get("/todos/batched", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, store.ListTodosBatched())
	})

	// Mount pprof on /debug/pprof — only expose behind auth in production.
	r.Mount("/debug", http.DefaultServeMux)

	return r
}
