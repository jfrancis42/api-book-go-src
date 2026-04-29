package errs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// Sentinel errors — returned by the repository layer.
var (
	ErrNotFound  = errors.New("not found")
	ErrConflict  = errors.New("conflict")
	ErrForbidden = errors.New("forbidden")
)

// FieldError describes a validation failure on a single field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationError accumulates field-level failures.
type ValidationError struct {
	Errors []FieldError `json:"fields"`
}

func (e *ValidationError) Error() string { return "validation failed" }
func (e *ValidationError) Add(f, m string) {
	e.Errors = append(e.Errors, FieldError{Field: f, Message: m})
}

// ErrorResponse is the standard error JSON envelope.
type ErrorResponse struct {
	Error   string       `json:"error"`
	Code    string       `json:"code,omitempty"`
	Fields  []FieldError `json:"fields,omitempty"`
	TraceID string       `json:"trace_id,omitempty"`
}

type contextKey string

const requestIDKey contextKey = "request-id"

func requestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// RequestID middleware injects a unique trace ID into each request.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			b := make([]byte, 8)
			rand.Read(b)
			id = hex.EncodeToString(b)
		}
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type responseWriter struct {
	http.ResponseWriter
	status  int
	written bool
}

func (rw *responseWriter) WriteHeader(s int) {
	if !rw.written {
		rw.status = s
		rw.written = true
		rw.ResponseWriter.WriteHeader(s)
	}
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}

// Logger logs at INFO/WARN/ERROR based on status.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &responseWriter{ResponseWriter: w}
		start := time.Now()
		next.ServeHTTP(rw, r)

		attrs := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", requestID(r.Context()),
		}
		switch {
		case rw.status >= 500:
			slog.Error("request", attrs...)
		case rw.status >= 400:
			slog.Warn("request", attrs...)
		default:
			slog.Info("request", attrs...)
		}
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// handleError maps application errors to HTTP responses, logging internal errors
// without leaking details to the caller.
func handleError(w http.ResponseWriter, r *http.Request, err error) {
	traceID := requestID(r.Context())

	var ve *ValidationError
	switch {
	case errors.Is(err, ErrNotFound):
		writeJSON(w, http.StatusNotFound, ErrorResponse{
			Error:   "the requested resource was not found",
			Code:    "NOT_FOUND",
			TraceID: traceID,
		})
	case errors.Is(err, ErrConflict):
		writeJSON(w, http.StatusConflict, ErrorResponse{
			Error:   "resource already exists",
			Code:    "CONFLICT",
			TraceID: traceID,
		})
	case errors.Is(err, ErrForbidden):
		writeJSON(w, http.StatusForbidden, ErrorResponse{
			Error:   "access denied",
			Code:    "FORBIDDEN",
			TraceID: traceID,
		})
	case errors.As(err, &ve):
		writeJSON(w, http.StatusUnprocessableEntity, ErrorResponse{
			Error:   "validation failed",
			Code:    "VALIDATION_ERROR",
			Fields:  ve.Errors,
			TraceID: traceID,
		})
	default:
		// Log full error internally; send nothing sensitive to caller.
		slog.Error("internal server error",
			"err", err,
			"request_id", traceID,
			"method", r.Method,
			"path", r.URL.Path,
		)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{
			Error:   "an internal error occurred",
			Code:    "INTERNAL_ERROR",
			TraceID: traceID,
		})
	}
}

// Todo is a minimal domain type.
type Todo struct {
	ID   int64  `json:"id"`
	Text string `json:"text"`
}

// repo is an in-memory store that can simulate errors.
type repo struct {
	todos    map[int64]*Todo
	nextID   int64
	forceErr error // if set, all Get calls return this error
}

func newRepo() *repo {
	return &repo{todos: make(map[int64]*Todo), nextID: 1}
}

func (r *repo) Create(text string) *Todo {
	t := &Todo{ID: r.nextID, Text: text}
	r.todos[r.nextID] = t
	r.nextID++
	return t
}

func (r *repo) Get(id int64) (*Todo, error) {
	if r.forceErr != nil {
		return nil, r.forceErr
	}
	t, ok := r.todos[id]
	if !ok {
		return nil, fmt.Errorf("todo %d: %w", id, ErrNotFound)
	}
	return t, nil
}

// BuildRouter returns a chi router demonstrating server error handling patterns.
func BuildRouter(db *repo) http.Handler {
	r := chi.NewRouter()
	r.Use(RequestID)
	r.Use(Logger)

	r.Post("/todos", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Text string `json:"text"` }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON"})
			return
		}
		req.Text = strings.TrimSpace(req.Text)
		if req.Text == "" {
			var ve ValidationError
			ve.Add("text", "required")
			handleError(w, r, &ve)
			return
		}
		writeJSON(w, http.StatusCreated, db.Create(req.Text))
	})

	r.Get("/todos/{id}", func(w http.ResponseWriter, r *http.Request) {
		var id int64
		fmt.Sscanf(chi.URLParam(r, "id"), "%d", &id)
		todo, err := db.Get(id)
		if err != nil {
			handleError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, todo)
	})

	return r
}
