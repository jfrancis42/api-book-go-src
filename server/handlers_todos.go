package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

const defaultLimit = 20
const maxLimit = 100

func handleListTodos(todos *TodoRepo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, _ := userIDFromContext(r.Context())

		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = defaultLimit
		}
		if limit > maxLimit {
			limit = maxLimit
		}

		items, total, err := todos.List(r.Context(), userID, offset, limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
			return
		}
		if items == nil {
			items = []*Todo{}
		}

		// Link header for pagination
		if offset+limit < total {
			next := fmt.Sprintf(`<%s?offset=%d&limit=%d>; rel="next"`,
				r.URL.Path, offset+limit, limit)
			w.Header().Set("Link", next)
		}

		writeJSON(w, http.StatusOK, Page[*Todo]{
			Items:  items,
			Total:  total,
			Offset: offset,
			Limit:  limit,
		})
	}
}

func handleCreateTodo(todos *TodoRepo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, _ := userIDFromContext(r.Context())

		var req CreateTodoRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON"})
			return
		}

		req.Text = strings.TrimSpace(req.Text)
		if req.Text == "" {
			writeJSON(w, http.StatusUnprocessableEntity, ErrorResponse{
				Error:  "validation failed",
				Code:   "VALIDATION_ERROR",
				Fields: []FieldError{{Field: "text", Message: "required"}},
			})
			return
		}

		todo, err := todos.Create(r.Context(), userID, req.Text)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
			return
		}
		writeJSON(w, http.StatusCreated, todo)
	}
}

func handleGetTodo(todos *TodoRepo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, _ := userIDFromContext(r.Context())
		id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
			return
		}

		todo, err := todos.Get(r.Context(), id)
		if err != nil {
			handleError(w, err)
			return
		}
		if todo.UserID != userID {
			handleError(w, ErrForbidden)
			return
		}
		writeJSON(w, http.StatusOK, todo)
	}
}

func handleUpdateTodo(todos *TodoRepo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, _ := userIDFromContext(r.Context())
		id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
			return
		}

		existing, err := todos.Get(r.Context(), id)
		if err != nil {
			handleError(w, err)
			return
		}
		if existing.UserID != userID {
			handleError(w, ErrForbidden)
			return
		}

		var req UpdateTodoRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON"})
			return
		}

		if req.Text != nil {
			*req.Text = strings.TrimSpace(*req.Text)
			if *req.Text == "" {
				writeJSON(w, http.StatusUnprocessableEntity, ErrorResponse{
					Error:  "validation failed",
					Code:   "VALIDATION_ERROR",
					Fields: []FieldError{{Field: "text", Message: "cannot be empty"}},
				})
				return
			}
		}

		todo, err := todos.Update(r.Context(), id, req.Text, req.Done)
		if err != nil {
			handleError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, todo)
	}
}

func handleDeleteTodo(todos *TodoRepo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, _ := userIDFromContext(r.Context())
		id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
			return
		}

		existing, err := todos.Get(r.Context(), id)
		if err != nil {
			handleError(w, err)
			return
		}
		if existing.UserID != userID {
			handleError(w, ErrForbidden)
			return
		}

		if err := todos.Delete(r.Context(), id); err != nil {
			handleError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
