package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"github.com/go-chi/chi/v5"
)

type Todo struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
	Done bool   `json:"done"`
}

type Store struct {
	mu    sync.RWMutex
	todos map[int]*Todo
	next  int
}

func NewStore() *Store {
	return &Store{todos: make(map[int]*Todo), next: 1}
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
	t := &Todo{ID: s.next, Text: text}
	s.todos[s.next] = t
	s.next++
	return t
}

func (s *Store) Get(id int) (*Todo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.todos[id]
	return t, ok
}

func (s *Store) Update(id int, done bool) (*Todo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.todos[id]
	if !ok {
		return nil, false
	}
	t.Done = done
	return t, true
}

func (s *Store) Delete(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.todos[id]
	delete(s.todos, id)
	return ok
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func BuildRouter(store *Store) http.Handler {
	r := chi.NewRouter()

	r.Get("/todos", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, store.List())
	})

	r.Post("/todos", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if req.Text == "" {
			writeError(w, http.StatusBadRequest, "text is required")
			return
		}
		writeJSON(w, http.StatusCreated, store.Create(req.Text))
	})

	r.Get("/todos/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		todo, ok := store.Get(id)
		if !ok {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeJSON(w, http.StatusOK, todo)
	})

	r.Patch("/todos/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		var req struct {
			Done bool `json:"done"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		todo, ok := store.Update(id, req.Done)
		if !ok {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeJSON(w, http.StatusOK, todo)
	})

	r.Delete("/todos/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		if !store.Delete(id) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	return r
}

func main() {
	store := NewStore()
	r := BuildRouter(store)
	fmt.Println("listening on :8080")
	http.ListenAndServe(":8080", r)
}

// Ensure context is used (needed for future extensions)
var _ = context.Background
