package methods

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

type Todo struct {
	ID        int64     `json:"id"`
	Text      string    `json:"text"`
	Done      bool      `json:"done"`
	Priority  string    `json:"priority"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ReplaceTodoRequest is the PUT body: all fields required, absent = zero.
type ReplaceTodoRequest struct {
	Text     string `json:"text"`
	Done     bool   `json:"done"`
	Priority string `json:"priority"`
}

func (req *ReplaceTodoRequest) Validate() error {
	if strings.TrimSpace(req.Text) == "" {
		return fmt.Errorf("text is required")
	}
	if req.Priority != "" {
		switch req.Priority {
		case "low", "medium", "high":
		default:
			return fmt.Errorf("priority must be one of: low, medium, high")
		}
	}
	return nil
}

// UpdateTodoRequest is the PATCH body: pointer fields — nil means "do not update".
type UpdateTodoRequest struct {
	Text     *string `json:"text"`
	Done     *bool   `json:"done"`
	Priority *string `json:"priority"`
}

func (req *UpdateTodoRequest) Validate() error {
	if req.Text != nil {
		if strings.TrimSpace(*req.Text) == "" {
			return fmt.Errorf("text must not be empty")
		}
		if len(*req.Text) > 500 {
			return fmt.Errorf("text must be 500 characters or fewer")
		}
	}
	if req.Priority != nil {
		switch *req.Priority {
		case "low", "medium", "high":
		default:
			return fmt.Errorf("priority must be one of: low, medium, high")
		}
	}
	return nil
}

type Store struct {
	mu     sync.Mutex
	todos  map[int64]*Todo
	nextID int64
}

func NewStore() *Store {
	return &Store{todos: make(map[int64]*Todo), nextID: 1}
}

func (s *Store) Create(text string) *Todo {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := &Todo{ID: s.nextID, Text: text, UpdatedAt: time.Now()}
	s.todos[s.nextID] = t
	s.nextID++
	return t
}

func (s *Store) Get(id int64) (*Todo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.todos[id]
	return t, ok
}

// Replace performs a full PUT replacement — all fields are overwritten.
func (s *Store) Replace(id int64, req ReplaceTodoRequest) (*Todo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.todos[id]
	if !ok {
		return nil, false
	}
	t.Text = req.Text
	t.Done = req.Done
	t.Priority = req.Priority
	t.UpdatedAt = time.Now()
	return t, true
}

// Patch performs a partial PATCH update — only non-nil fields change.
func (s *Store) Patch(id int64, req UpdateTodoRequest) (*Todo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.todos[id]
	if !ok {
		return nil, false
	}
	if req.Text != nil {
		t.Text = *req.Text
	}
	if req.Done != nil {
		t.Done = *req.Done
	}
	if req.Priority != nil {
		t.Priority = *req.Priority
	}
	t.UpdatedAt = time.Now()
	return t, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func BuildRouter(store *Store) http.Handler {
	r := chi.NewRouter()

	r.Post("/todos", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Text string `json:"text"` }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Text) == "" {
			writeError(w, http.StatusBadRequest, "text is required")
			return
		}
		t := store.Create(req.Text)
		w.Header().Set("Location", fmt.Sprintf("/todos/%d", t.ID))
		writeJSON(w, http.StatusCreated, t)
	})

	r.Get("/todos/{id}", func(w http.ResponseWriter, r *http.Request) {
		var id int64
		fmt.Sscanf(chi.URLParam(r, "id"), "%d", &id)
		t, ok := store.Get(id)
		if !ok {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeJSON(w, http.StatusOK, t)
	})

	// PUT: full replacement — omitted fields become zero values.
	r.Put("/todos/{id}", func(w http.ResponseWriter, r *http.Request) {
		var id int64
		fmt.Sscanf(chi.URLParam(r, "id"), "%d", &id)

		var req ReplaceTodoRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if err := req.Validate(); err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		t, ok := store.Replace(id, req)
		if !ok {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeJSON(w, http.StatusOK, t)
	})

	// PATCH: partial update — only supplied fields change.
	r.Patch("/todos/{id}", func(w http.ResponseWriter, r *http.Request) {
		var id int64
		fmt.Sscanf(chi.URLParam(r, "id"), "%d", &id)

		var req UpdateTodoRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if err := req.Validate(); err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		t, ok := store.Patch(id, req)
		if !ok {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeJSON(w, http.StatusOK, t)
	})

	return r
}
