package validation

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(BuildRouter())
	t.Cleanup(srv.Close)
	return srv
}

func postJSON(t *testing.T, srv *httptest.Server, body string) *http.Response {
	t.Helper()
	resp, err := srv.Client().Post(srv.URL+"/todos", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	return resp
}

func TestCreateTodo_Valid(t *testing.T) {
	srv := newServer(t)
	resp := postJSON(t, srv, `{"text":"buy milk","priority":"high"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("got %d, want 201", resp.StatusCode)
	}
}

func TestCreateTodo_MissingText(t *testing.T) {
	srv := newServer(t)
	resp := postJSON(t, srv, `{"priority":"low"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("got %d, want 422", resp.StatusCode)
	}
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["fields"] == nil {
		t.Fatal("expected 'fields' in error response")
	}
}

func TestCreateTodo_InvalidPriority(t *testing.T) {
	srv := newServer(t)
	resp := postJSON(t, srv, `{"text":"hello","priority":"urgent"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("got %d, want 422", resp.StatusCode)
	}
}

func TestCreateTodo_TextTooLong(t *testing.T) {
	srv := newServer(t)
	long := strings.Repeat("a", 501)
	resp := postJSON(t, srv, `{"text":"`+long+`"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("got %d, want 422", resp.StatusCode)
	}
}

func TestCreateTodo_TagTooLong(t *testing.T) {
	srv := newServer(t)
	longTag := strings.Repeat("x", 51)
	body, _ := json.Marshal(map[string]any{
		"text": "hello",
		"tags": []string{"ok", longTag},
	})
	resp, _ := srv.Client().Post(srv.URL+"/todos", "application/json", bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("got %d, want 422", resp.StatusCode)
	}
}

func TestCreateTodo_MultipleFieldErrors(t *testing.T) {
	srv := newServer(t)
	resp := postJSON(t, srv, `{"priority":"ASAP"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("got %d, want 422", resp.StatusCode)
	}
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	fields := body["fields"].([]any)
	if len(fields) < 2 {
		t.Fatalf("expected at least 2 field errors (text + priority), got %d", len(fields))
	}
}

func TestCreateTodo_InvalidJSON(t *testing.T) {
	srv := newServer(t)
	resp := postJSON(t, srv, `not json`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", resp.StatusCode)
	}
}

func TestCreateTodo_EmptyBody(t *testing.T) {
	srv := newServer(t)
	resp := postJSON(t, srv, ``)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", resp.StatusCode)
	}
}

func TestGetTodo_InvalidID(t *testing.T) {
	srv := newServer(t)
	resp, _ := srv.Client().Get(srv.URL + "/todos/abc")
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", resp.StatusCode)
	}
}

func TestGetTodo_NegativeID(t *testing.T) {
	srv := newServer(t)
	resp, _ := srv.Client().Get(srv.URL + "/todos/-1")
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", resp.StatusCode)
	}
}

func TestValidationError_Error(t *testing.T) {
	var v ValidationError
	v.Add("text", "required")
	v.Add("priority", "invalid")
	msg := v.Error()
	if !strings.Contains(msg, "text: required") {
		t.Fatalf("unexpected error message: %s", msg)
	}
}
