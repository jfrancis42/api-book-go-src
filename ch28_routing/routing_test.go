package routing

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(BuildRouter(NewStore()))
	t.Cleanup(srv.Close)
	return srv
}

func createTodo(t *testing.T, srv *httptest.Server, text string) *Todo {
	t.Helper()
	resp, err := srv.Client().Post(srv.URL+"/api/v1/todos", "application/json",
		strings.NewReader(fmt.Sprintf(`{"text":%q}`, text)))
	if err != nil {
		t.Fatal(err)
	}
	var todo Todo
	json.NewDecoder(resp.Body).Decode(&todo)
	resp.Body.Close()
	return &todo
}

func TestList(t *testing.T) {
	srv := newServer(t)
	createTodo(t, srv, "one")
	createTodo(t, srv, "two")

	resp, _ := srv.Client().Get(srv.URL + "/api/v1/todos/")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
	var todos []*Todo
	json.NewDecoder(resp.Body).Decode(&todos)
	if len(todos) != 2 {
		t.Fatalf("got %d todos, want 2", len(todos))
	}
}

func TestCreateSetsLocation(t *testing.T) {
	srv := newServer(t)
	resp, _ := srv.Client().Post(srv.URL+"/api/v1/todos", "application/json",
		strings.NewReader(`{"text":"hello"}`))
	resp.Body.Close()

	if loc := resp.Header.Get("Location"); !strings.HasPrefix(loc, "/api/v1/todos/") {
		t.Fatalf("Location: got %q", loc)
	}
}

func TestTodoCtx_GetFromContext(t *testing.T) {
	srv := newServer(t)
	created := createTodo(t, srv, "context test")

	resp, _ := srv.Client().Get(fmt.Sprintf("%s/api/v1/todos/%d/", srv.URL, created.ID))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
	var got Todo
	json.NewDecoder(resp.Body).Decode(&got)
	if got.Text != "context test" {
		t.Fatalf("text: got %q, want %q", got.Text, "context test")
	}
}

func TestTodoCtx_NotFound(t *testing.T) {
	srv := newServer(t)
	resp, _ := srv.Client().Get(srv.URL + "/api/v1/todos/99999/")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("got %d, want 404", resp.StatusCode)
	}
}

func TestMarkDone(t *testing.T) {
	srv := newServer(t)
	created := createTodo(t, srv, "finish chapter")

	resp, _ := srv.Client().Post(
		fmt.Sprintf("%s/api/v1/todos/%d/done", srv.URL, created.ID), "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
	var got Todo
	json.NewDecoder(resp.Body).Decode(&got)
	if !got.Done {
		t.Fatal("expected done=true")
	}
}

func TestAdminSubrouter(t *testing.T) {
	srv := newServer(t)
	createTodo(t, srv, "visible to admin")

	resp, _ := srv.Client().Get(srv.URL + "/admin/todos")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
}

func TestHealthEndpoint(t *testing.T) {
	srv := newServer(t)
	resp, _ := srv.Client().Get(srv.URL + "/health")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Fatalf("status: got %v, want ok", body["status"])
	}
}

func TestNotFound(t *testing.T) {
	srv := newServer(t)
	resp, _ := srv.Client().Get(srv.URL + "/does/not/exist")
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("got %d, want 404", resp.StatusCode)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	srv := newServer(t)
	req, _ := http.NewRequest("DELETE", srv.URL+"/health", nil)
	resp, _ := srv.Client().Do(req)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("got %d, want 405", resp.StatusCode)
	}
}
