package github

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrRateLimited  = errors.New("rate limited")
	ErrValidation   = errors.New("validation failed")
	ErrConflict     = errors.New("conflict")
	ErrGone         = errors.New("gone")
	ErrServerError  = errors.New("server error")
)

// FieldError is a single field-level validation failure.
type FieldError struct {
	Resource string `json:"resource"`
	Field    string `json:"field"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

// APIError represents a non-2xx response from the GitHub API.
type APIError struct {
	StatusCode  int
	Message     string
	FieldErrors []FieldError
}

func (e *APIError) Error() string {
	return fmt.Sprintf("GitHub API error %d: %s", e.StatusCode, e.Message)
}

// IsRetryable reports whether the error represents a transient failure
// that is safe to retry.
func IsRetryable(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.StatusCode {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}
	return false
}

func parseAPIError(statusCode int, body []byte) error {
	apiErr := &APIError{StatusCode: statusCode}

	var raw struct {
		Message string       `json:"message"`
		Errors  []FieldError `json:"errors"`
	}
	if json.Unmarshal(body, &raw) == nil {
		apiErr.Message = raw.Message
		apiErr.FieldErrors = raw.Errors
	} else {
		apiErr.Message = string(body)
	}

	switch statusCode {
	case http.StatusNotFound:
		return fmt.Errorf("%w: %w", ErrNotFound, apiErr)
	case http.StatusUnauthorized:
		return fmt.Errorf("%w: %w", ErrUnauthorized, apiErr)
	case http.StatusForbidden:
		return fmt.Errorf("%w: %w", ErrForbidden, apiErr)
	case http.StatusTooManyRequests:
		return fmt.Errorf("%w: %w", ErrRateLimited, apiErr)
	case http.StatusUnprocessableEntity:
		return fmt.Errorf("%w: %w", ErrValidation, apiErr)
	case http.StatusConflict:
		return fmt.Errorf("%w: %w", ErrConflict, apiErr)
	case http.StatusGone:
		return fmt.Errorf("%w: %w", ErrGone, apiErr)
	default:
		if statusCode >= 500 {
			return fmt.Errorf("%w: %w", ErrServerError, apiErr)
		}
		return apiErr
	}
}
