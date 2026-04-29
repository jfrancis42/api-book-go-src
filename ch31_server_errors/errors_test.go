package errs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newServer(t *testing.T, db *repo) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(BuildRouter(db))
	t.Cleanup(srv.Close)
	return srv
}

func TestNotFound_ReturnsCorrectShape(t *testing.T) {
	srv := newServer(t, newRepo())
	resp, _ := srv.Client().Get(srv.URL + "/todos/99")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("got %d, want 404", resp.StatusCode)
	}
	var er ErrorResponse
	json.NewDecoder(resp.Body).Decode(&er)
	if er.Code != "NOT_FOUND" {
		t.Fatalf("code: got %q, want NOT_FOUND", er.Code)
	}
	if er.TraceID == "" {
		t.Fatal("expected trace_id in 404 response")
	}
}

func TestValidationError_Returns422WithFields(t *testing.T) {
	srv := newServer(t, newRepo())
	resp, _ := srv.Client().Post(srv.URL+"/todos", "application/json",
		strings.NewReader(`{"text":""}`))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("got %d, want 422", resp.StatusCode)
	}
	var er ErrorResponse
	json.NewDecoder(resp.Body).Decode(&er)
	if er.Code != "VALIDATION_ERROR" {
		t.Fatalf("code: got %q", er.Code)
	}
	if len(er.Fields) == 0 {
		t.Fatal("expected field errors")
	}
}

func TestInternalError_DoesNotLeakDetails(t *testing.T) {
	db := newRepo()
	db.Create("one") // ID=1
	db.forceErr = fmt.Errorf("database connection: FATAL: password authentication failed for user \"admin\"")

	srv := newServer(t, db)
	resp, _ := srv.Client().Get(srv.URL + "/todos/1")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("got %d, want 500", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), "password") {
		t.Fatalf("internal error message leaked to client: %s", body)
	}
	if strings.Contains(string(body), "FATAL") {
		t.Fatalf("internal error message leaked to client: %s", body)
	}
}

func TestInternalError_IncludesTraceID(t *testing.T) {
	db := newRepo()
	db.Create("one")
	db.forceErr = fmt.Errorf("oh no")

	srv := newServer(t, db)
	req, _ := http.NewRequest("GET", srv.URL+"/todos/1", nil)
	req.Header.Set("X-Request-ID", "test-trace-123")
	resp, _ := srv.Client().Do(req)
	defer resp.Body.Close()

	var er ErrorResponse
	json.NewDecoder(resp.Body).Decode(&er)
	if er.TraceID != "test-trace-123" {
		t.Fatalf("trace_id: got %q, want test-trace-123", er.TraceID)
	}
}

func TestRequestID_Generated(t *testing.T) {
	srv := newServer(t, newRepo())
	resp, _ := srv.Client().Get(srv.URL + "/todos/1")
	resp.Body.Close()

	if resp.Header.Get("X-Request-ID") == "" {
		t.Fatal("expected X-Request-ID header in response")
	}
}

func TestRequestID_EchoedFromClient(t *testing.T) {
	srv := newServer(t, newRepo())
	req, _ := http.NewRequest("GET", srv.URL+"/todos/1", nil)
	req.Header.Set("X-Request-ID", "client-set-id")
	resp, _ := srv.Client().Do(req)
	resp.Body.Close()

	if got := resp.Header.Get("X-Request-ID"); got != "client-set-id" {
		t.Fatalf("got %q, want client-set-id", got)
	}
}

func TestSentinelErrors_WrappedNotFound(t *testing.T) {
	// errors.Is unwraps the chain — repository can add context without breaking callers.
	wrapped := fmt.Errorf("TodoRepo.Get(id=99): %w", ErrNotFound)
	if !errors.Is(wrapped, ErrNotFound) {
		t.Fatal("errors.Is should find ErrNotFound through wrapping")
	}
}
