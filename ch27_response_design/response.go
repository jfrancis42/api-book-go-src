package response

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// ErrorResponse is the standard error envelope.
type ErrorResponse struct {
	Error   string       `json:"error"`
	Code    string       `json:"code,omitempty"`
	Fields  []FieldError `json:"fields,omitempty"`
	TraceID string       `json:"trace_id,omitempty"`
}

// FieldError is a single field-level validation failure.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

const (
	ErrCodeNotFound   = "NOT_FOUND"
	ErrCodeUnauthorized = "UNAUTHORIZED"
	ErrCodeForbidden  = "FORBIDDEN"
	ErrCodeValidation = "VALIDATION_ERROR"
	ErrCodeInternal   = "INTERNAL_ERROR"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrForbidden = errors.New("forbidden")
)

// ValidationError accumulates field-level failures.
type ValidationError struct {
	Errors []FieldError
}

func (e *ValidationError) Error() string { return "validation failed" }

func (e *ValidationError) Add(field, msg string) {
	e.Errors = append(e.Errors, FieldError{Field: field, Message: msg})
}

// Page is the standard paginated response envelope.
type Page[T any] struct {
	Items   []T   `json:"items"`
	Total   int64 `json:"total"`
	Page    int   `json:"page"`
	PerPage int   `json:"per_page"`
	HasMore bool  `json:"has_more"`
}

func NewPage[T any](items []T, total int64, page, perPage int) Page[T] {
	return Page[T]{
		Items:   items,
		Total:   total,
		Page:    page,
		PerPage: perPage,
		HasMore: int64(page*perPage) < total,
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// handleError maps Go errors to HTTP responses.
func handleError(w http.ResponseWriter, r *http.Request, err error) {
	traceID := r.Header.Get("X-Request-ID")

	var ve *ValidationError
	switch {
	case errors.Is(err, ErrNotFound):
		writeJSON(w, http.StatusNotFound, ErrorResponse{
			Error:   "the requested resource was not found",
			Code:    ErrCodeNotFound,
			TraceID: traceID,
		})
	case errors.As(err, &ve):
		writeJSON(w, http.StatusUnprocessableEntity, ErrorResponse{
			Error:   "validation failed",
			Code:    ErrCodeValidation,
			Fields:  ve.Errors,
			TraceID: traceID,
		})
	default:
		slog.Error("internal error", "err", err, "trace_id", traceID)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{
			Error:   "an internal error occurred",
			Code:    ErrCodeInternal,
			TraceID: traceID,
		})
	}
}

// Todo is the domain model, demonstrating RFC3339 timestamps and snake_case.
type Todo struct {
	ID        int64     `json:"id"`
	Text      string    `json:"text"`
	Done      bool      `json:"done"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// store is a minimal in-memory backing store.
type store struct {
	todos  map[int64]*Todo
	nextID int64
}

func newStore() *store {
	return &store{todos: make(map[int64]*Todo), nextID: 1}
}

func (s *store) create(text string) *Todo {
	now := time.Now().UTC()
	t := &Todo{ID: s.nextID, Text: text, CreatedAt: now, UpdatedAt: now}
	s.todos[s.nextID] = t
	s.nextID++
	return t
}

func (s *store) get(id int64) (*Todo, bool) {
	t, ok := s.todos[id]
	return t, ok
}

func (s *store) list(page, perPage int) ([]*Todo, int64) {
	all := make([]*Todo, 0, len(s.todos))
	for _, t := range s.todos {
		all = append(all, t)
	}
	total := int64(len(all))
	start := (page - 1) * perPage
	if start >= len(all) {
		return []*Todo{}, total
	}
	end := start + perPage
	if end > len(all) {
		end = len(all)
	}
	return all[start:end], total
}

// BuildRouter assembles a chi router demonstrating response design patterns.
func BuildRouter() http.Handler {
	s := newStore()
	r := chi.NewRouter()

	r.Post("/todos", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON"})
			return
		}
		if strings.TrimSpace(req.Text) == "" {
			var ve ValidationError
			ve.Add("text", "required")
			handleError(w, r, &ve)
			return
		}
		todo := s.create(req.Text)
		// Set Location header on 201.
		w.Header().Set("Location", fmt.Sprintf("/todos/%d", todo.ID))
		writeJSON(w, http.StatusCreated, todo)
	})

	r.Get("/todos/{id}", func(w http.ResponseWriter, r *http.Request) {
		var id int64
		if _, err := fmt.Sscanf(chi.URLParam(r, "id"), "%d", &id); err != nil || id <= 0 {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
			return
		}
		todo, ok := s.get(id)
		if !ok {
			handleError(w, r, ErrNotFound)
			return
		}
		writeJSON(w, http.StatusOK, todo)
	})

	r.Get("/todos", func(w http.ResponseWriter, r *http.Request) {
		page := 1
		perPage := 20
		fmt.Sscanf(r.URL.Query().Get("page"), "%d", &page)
		fmt.Sscanf(r.URL.Query().Get("per_page"), "%d", &perPage)
		if page < 1 {
			page = 1
		}
		if perPage < 1 || perPage > 100 {
			perPage = 20
		}
		items, total := s.list(page, perPage)
		writeJSON(w, http.StatusOK, NewPage(items, total, page, perPage))
	})

	return r
}
