package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func withServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(func() { srv.Close(); setBaseURL("https://api.github.com") })
	setBaseURL(srv.URL)
	return NewClient("test-token")
}

func TestErrNotFound(t *testing.T) {
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`))
	})
	_, err := c.GetUser(context.Background(), "nobody")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestErrUnauthorized(t *testing.T) {
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"Bad credentials"}`))
	})
	_, err := c.GetUser(context.Background(), "test")
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

func TestErrRateLimited(t *testing.T) {
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	})
	_, err := c.GetUser(context.Background(), "test")
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("expected ErrRateLimited, got %v", err)
	}
}

func TestErrServerError(t *testing.T) {
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message":"Server Error"}`))
	})
	_, err := c.GetUser(context.Background(), "test")
	if !errors.Is(err, ErrServerError) {
		t.Errorf("expected ErrServerError, got %v", err)
	}
}

func TestAPIError_AsExtraction(t *testing.T) {
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`))
	})
	_, err := c.GetUser(context.Background(), "test")

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatal("expected to extract *APIError")
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode: got %d, want 404", apiErr.StatusCode)
	}
	if apiErr.Message != "Not Found" {
		t.Errorf("Message: got %q", apiErr.Message)
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		code     int
		wantRetry bool
	}{
		{200, false},
		{404, false},
		{429, true},
		{500, true},
		{502, true},
		{503, true},
		{504, true},
	}
	for _, tt := range tests {
		err := parseAPIError(tt.code, []byte(`{"message":"test"}`))
		if got := IsRetryable(err); got != tt.wantRetry {
			t.Errorf("IsRetryable for %d: got %v, want %v", tt.code, got, tt.wantRetry)
		}
	}
}

func TestFieldErrors(t *testing.T) {
	body := []byte(`{"message":"Validation Failed","errors":[{"resource":"Issue","field":"title","code":"missing_field"}]}`)
	err := parseAPIError(http.StatusUnprocessableEntity, body)

	if !errors.Is(err, ErrValidation) {
		t.Errorf("expected ErrValidation")
	}
	var apiErr *APIError
	errors.As(err, &apiErr)
	if len(apiErr.FieldErrors) != 1 {
		t.Fatalf("expected 1 field error, got %d", len(apiErr.FieldErrors))
	}
	if apiErr.FieldErrors[0].Field != "title" {
		t.Errorf("field: got %q", apiErr.FieldErrors[0].Field)
	}
}
