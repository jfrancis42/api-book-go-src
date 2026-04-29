package methods

import (
	"bytes"
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
	resp, _ := srv.Client().Post(srv.URL+"/todos", "application/json",
		strings.NewReader(fmt.Sprintf(`{"text":%q}`, text)))
	var todo Todo
	json.NewDecoder(resp.Body).Decode(&todo)
	resp.Body.Close()
	return &todo
}

func TestPOST_LocationHeader(t *testing.T) {
	srv := newServer(t)
	resp, _ := srv.Client().Post(srv.URL+"/todos", "application/json",
		strings.NewReader(`{"text":"first"}`))
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("got %d, want 201", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); !strings.HasPrefix(loc, "/todos/") {
		t.Fatalf("Location: got %q", loc)
	}
}

func TestPUT_ReplacesAllFields(t *testing.T) {
	srv := newServer(t)
	created := createTodo(t, srv, "original text")

	body, _ := json.Marshal(ReplaceTodoRequest{Text: "replaced", Done: true, Priority: "high"})
	req, _ := http.NewRequest("PUT", fmt.Sprintf("%s/todos/%d", srv.URL, created.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := srv.Client().Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
	var got Todo
	json.NewDecoder(resp.Body).Decode(&got)
	if got.Text != "replaced" {
		t.Errorf("Text: got %q, want replaced", got.Text)
	}
	if !got.Done {
		t.Error("Done: expected true")
	}
	if got.Priority != "high" {
		t.Errorf("Priority: got %q, want high", got.Priority)
	}
}

func TestPUT_ZeroesUnsentFields(t *testing.T) {
	srv := newServer(t)
	created := createTodo(t, srv, "initial")

	// First set priority via PATCH.
	patchBody, _ := json.Marshal(UpdateTodoRequest{Priority: strPtr("high")})
	patchReq, _ := http.NewRequest("PATCH", fmt.Sprintf("%s/todos/%d", srv.URL, created.ID), bytes.NewReader(patchBody))
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, _ := srv.Client().Do(patchReq)
	io.Copy(io.Discard, patchResp.Body)
	patchResp.Body.Close()

	// Now PUT without priority — it should become empty string (zero value).
	putBody, _ := json.Marshal(ReplaceTodoRequest{Text: "replaced"})
	putReq, _ := http.NewRequest("PUT", fmt.Sprintf("%s/todos/%d", srv.URL, created.ID), bytes.NewReader(putBody))
	putReq.Header.Set("Content-Type", "application/json")
	resp, _ := srv.Client().Do(putReq)
	defer resp.Body.Close()

	var got Todo
	json.NewDecoder(resp.Body).Decode(&got)
	if got.Priority != "" {
		t.Errorf("Priority should be zeroed by PUT, got %q", got.Priority)
	}
	if got.Done != false {
		t.Error("Done should be false (zeroed) after PUT without done=true")
	}
}

func TestPATCH_OnlyUpdatesSuppliedFields(t *testing.T) {
	srv := newServer(t)
	created := createTodo(t, srv, "unchanged text")

	done := true
	body, _ := json.Marshal(UpdateTodoRequest{Done: &done})
	req, _ := http.NewRequest("PATCH", fmt.Sprintf("%s/todos/%d", srv.URL, created.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := srv.Client().Do(req)
	defer resp.Body.Close()

	var got Todo
	json.NewDecoder(resp.Body).Decode(&got)
	if got.Text != "unchanged text" {
		t.Errorf("Text should be unchanged, got %q", got.Text)
	}
	if !got.Done {
		t.Error("Done: expected true")
	}
}

func TestPATCH_EmptyBodyIsNoop(t *testing.T) {
	srv := newServer(t)
	created := createTodo(t, srv, "stable text")

	req, _ := http.NewRequest("PATCH", fmt.Sprintf("%s/todos/%d", srv.URL, created.ID),
		strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := srv.Client().Do(req)
	defer resp.Body.Close()

	var got Todo
	json.NewDecoder(resp.Body).Decode(&got)
	if got.Text != "stable text" {
		t.Errorf("Text changed on empty PATCH: got %q", got.Text)
	}
}

func TestPUT_NotFound(t *testing.T) {
	srv := newServer(t)
	body, _ := json.Marshal(ReplaceTodoRequest{Text: "x"})
	req, _ := http.NewRequest("PUT", srv.URL+"/todos/99999", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := srv.Client().Do(req)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("got %d, want 404", resp.StatusCode)
	}
}

func strPtr(s string) *string { return &s }
