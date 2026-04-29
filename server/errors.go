package server

import (
	"errors"
	"fmt"
	"net/http"
)

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

// ValidationError is returned when request data fails validation.
type ValidationError struct {
	Fields []FieldError
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed: %d field(s)", len(e.Fields))
}

// ErrorResponse is the JSON envelope returned for all error responses.
type ErrorResponse struct {
	Error  string       `json:"error"`
	Code   string       `json:"code,omitempty"`
	Fields []FieldError `json:"fields,omitempty"`
}

func handleError(w http.ResponseWriter, err error) {
	var ve *ValidationError
	switch {
	case errors.As(err, &ve):
		writeJSON(w, http.StatusUnprocessableEntity, ErrorResponse{
			Error:  "validation failed",
			Code:   "VALIDATION_ERROR",
			Fields: ve.Fields,
		})
	case errors.Is(err, ErrNotFound):
		writeJSON(w, http.StatusNotFound, ErrorResponse{
			Error: "not found",
			Code:  "NOT_FOUND",
		})
	case errors.Is(err, ErrConflict):
		writeJSON(w, http.StatusConflict, ErrorResponse{
			Error: "conflict",
			Code:  "CONFLICT",
		})
	case errors.Is(err, ErrForbidden):
		writeJSON(w, http.StatusForbidden, ErrorResponse{
			Error: "forbidden",
			Code:  "FORBIDDEN",
		})
	default:
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{
			Error: "internal server error",
			Code:  "INTERNAL_ERROR",
		})
	}
}
