package validation

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

// FieldError describes a validation failure on a single field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationError accumulates field-level validation failures.
type ValidationError struct {
	Errors []FieldError `json:"fields"`
}

func (e *ValidationError) Error() string {
	msgs := make([]string, len(e.Errors))
	for i, fe := range e.Errors {
		msgs[i] = fe.Field + ": " + fe.Message
	}
	return "validation failed: " + strings.Join(msgs, "; ")
}

func (e *ValidationError) Add(field, message string) {
	e.Errors = append(e.Errors, FieldError{Field: field, Message: message})
}

func (e *ValidationError) HasErrors() bool { return len(e.Errors) > 0 }

// CreateTodoRequest is the POST /todos request body.
type CreateTodoRequest struct {
	Text     string   `json:"text"`
	Priority string   `json:"priority"`
	Tags     []string `json:"tags"`
}

func (req *CreateTodoRequest) Validate() error {
	var v ValidationError

	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" {
		v.Add("text", "required")
	} else if len(req.Text) > 500 {
		v.Add("text", "must be 500 characters or fewer")
	}

	if req.Priority != "" {
		allowed := map[string]bool{"low": true, "medium": true, "high": true}
		if !allowed[req.Priority] {
			v.Add("priority", "must be one of: low, medium, high")
		}
	}

	for i, tag := range req.Tags {
		if len(tag) > 50 {
			v.Add(fmt.Sprintf("tags[%d]", i), "must be 50 characters or fewer")
		}
	}

	if v.HasErrors() {
		return &v
	}
	return nil
}

// decodeBody decodes JSON from r into v, enforcing a 1 MB limit and returning
// descriptive errors for common JSON problems.
func decodeBody(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()

	if err := d.Decode(v); err != nil {
		var syntaxErr *json.SyntaxError
		var unmarshalErr *json.UnmarshalTypeError
		var maxBytesErr *http.MaxBytesError

		switch {
		case errors.As(err, &syntaxErr):
			return fmt.Errorf("invalid JSON at position %d", syntaxErr.Offset)
		case errors.As(err, &unmarshalErr):
			return fmt.Errorf("field %q must be %s", unmarshalErr.Field, unmarshalErr.Type)
		case errors.As(err, &maxBytesErr):
			return fmt.Errorf("request body too large (max 1MB)")
		case errors.Is(err, io.EOF):
			return fmt.Errorf("request body is empty")
		default:
			return fmt.Errorf("decode error: %w", err)
		}
	}
	return nil
}

// parseID parses a positive integer from the chi URL parameter "id".
func parseID(r *http.Request) (int64, error) {
	s := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("id must be a positive integer")
	}
	return id, nil
}

// parseLimit parses ?limit= with bounds checking.
func parseLimit(r *http.Request, defaultVal int) (int, error) {
	s := r.URL.Query().Get("limit")
	if s == "" {
		return defaultVal, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("limit must be an integer")
	}
	if n < 1 || n > 100 {
		return 0, fmt.Errorf("limit must be between 1 and 100")
	}
	return n, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// Todo is the minimal domain type for this chapter.
type Todo struct {
	ID       int64    `json:"id"`
	Text     string   `json:"text"`
	Priority string   `json:"priority,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

// BuildRouter returns a chi router demonstrating request validation.
func BuildRouter() http.Handler {
	r := chi.NewRouter()
	todos := map[int64]*Todo{}
	var nextID int64 = 1

	r.Post("/todos", func(w http.ResponseWriter, r *http.Request) {
		var req CreateTodoRequest
		if err := decodeBody(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := req.Validate(); err != nil {
			var ve *ValidationError
			if errors.As(err, &ve) {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
					"error":  "validation failed",
					"fields": ve.Errors,
				})
				return
			}
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		t := &Todo{ID: nextID, Text: req.Text, Priority: req.Priority, Tags: req.Tags}
		todos[nextID] = t
		nextID++
		writeJSON(w, http.StatusCreated, t)
	})

	r.Get("/todos/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		t, ok := todos[id]
		if !ok {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeJSON(w, http.StatusOK, t)
	})

	return r
}
