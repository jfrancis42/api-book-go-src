package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	store := NewStore()
	srv := httptest.NewServer(BuildRouter(store))
	t.Cleanup(srv.Close)
	return srv
}

func TestListTodos_Empty(t *testing.T) {
	srv := newTestServer(t)
	resp, err := http.Get(srv.URL + "/todos")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}

	var todos []*Todo
	json.NewDecoder(resp.Body).Decode(&todos)
	if len(todos) != 0 {
		t.Errorf("expected empty list, got %d items", len(todos))
	}
}

func TestCreateTodo(t *testing.T) {
	srv := newTestServer(t)

	body := bytes.NewBufferString(`{"text":"write tests"}`)
	resp, err := http.Post(srv.URL+"/todos", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status: got %d, want 201", resp.StatusCode)
	}

	var todo Todo
	json.NewDecoder(resp.Body).Decode(&todo)
	if todo.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if todo.Text != "write tests" {
		t.Errorf("text: got %q, want %q", todo.Text, "write tests")
	}
	if todo.Done {
		t.Error("new todo should not be done")
	}
}

func TestCreateTodo_EmptyText(t *testing.T) {
	srv := newTestServer(t)

	body := bytes.NewBufferString(`{"text":""}`)
	resp, err := http.Post(srv.URL+"/todos", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400", resp.StatusCode)
	}
}

func TestGetTodo_NotFound(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/todos/999")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", resp.StatusCode)
	}
}

func TestUpdateTodo(t *testing.T) {
	srv := newTestServer(t)

	// Create
	body := bytes.NewBufferString(`{"text":"finish book"}`)
	resp, _ := http.Post(srv.URL+"/todos", "application/json", body)
	resp.Body.Close()

	// Get the created todo
	resp, err := http.Get(srv.URL + "/todos/1")
	if err != nil {
		t.Fatal(err)
	}
	var todo Todo
	json.NewDecoder(resp.Body).Decode(&todo)
	resp.Body.Close()

	// Update
	patch := bytes.NewBufferString(`{"done":true}`)
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/todos/1", patch)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	var updated Todo
	json.NewDecoder(resp.Body).Decode(&updated)
	if !updated.Done {
		t.Error("expected done=true")
	}
}

func TestDeleteTodo(t *testing.T) {
	srv := newTestServer(t)

	// Create
	body := bytes.NewBufferString(`{"text":"delete me"}`)
	http.Post(srv.URL+"/todos", "application/json", body)

	// Delete
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/todos/1", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status: got %d, want 204", resp.StatusCode)
	}

	// Verify gone
	resp, _ = http.Get(srv.URL + "/todos/1")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("after delete: status: got %d, want 404", resp.StatusCode)
	}
}
