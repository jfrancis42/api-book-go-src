package complete

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

var testSecret = []byte("test-jwt-secret-32-bytes-minimum!")

// newTestServer spins up an httptest.Server backed by an in-memory SQLite DB.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	db, err := openDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	srv := httptest.NewServer(BuildRouter(db, testSecret))
	t.Cleanup(func() {
		srv.Close()
		db.Close()
	})
	return srv
}

// helper: POST JSON, return response
func postJSON(t *testing.T, client *http.Client, url string, body any, token string) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

func decodeBody(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode body: %v", err)
	}
}

func drainClose(resp *http.Response) {
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

// registerAndLogin creates a user and returns its JWT token.
func registerAndLogin(t *testing.T, srv *httptest.Server, username, password string) string {
	t.Helper()
	creds := RegisterRequest{Username: username, Password: password}
	resp := postJSON(t, srv.Client(), srv.URL+"/api/v1/auth/register", creds, "")
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("register got %d: %s", resp.StatusCode, body)
	}
	var tok TokenResponse
	decodeBody(t, resp, &tok)
	return tok.Token
}

// --- Auth tests ---

func TestRegister_Success(t *testing.T) {
	srv := newTestServer(t)
	resp := postJSON(t, srv.Client(), srv.URL+"/api/v1/auth/register",
		RegisterRequest{Username: "alice", Password: "secret123"}, "")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("got %d, want 201", resp.StatusCode)
	}
	var tok TokenResponse
	decodeBody(t, resp, &tok)
	if tok.Token == "" {
		t.Fatal("expected non-empty token")
	}
}

func TestRegister_DuplicateUsername(t *testing.T) {
	srv := newTestServer(t)
	creds := RegisterRequest{Username: "alice", Password: "secret123"}
	drainClose(postJSON(t, srv.Client(), srv.URL+"/api/v1/auth/register", creds, ""))
	resp := postJSON(t, srv.Client(), srv.URL+"/api/v1/auth/register", creds, "")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("got %d, want 409", resp.StatusCode)
	}
	drainClose(resp)
}

func TestRegister_ShortPassword(t *testing.T) {
	srv := newTestServer(t)
	resp := postJSON(t, srv.Client(), srv.URL+"/api/v1/auth/register",
		RegisterRequest{Username: "bob", Password: "short"}, "")
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("got %d, want 422", resp.StatusCode)
	}
	var er ErrorResponse
	decodeBody(t, resp, &er)
	if len(er.Fields) == 0 {
		t.Fatal("expected field errors")
	}
}

func TestLogin_Success(t *testing.T) {
	srv := newTestServer(t)
	registerAndLogin(t, srv, "alice", "secret123") // register first

	resp := postJSON(t, srv.Client(), srv.URL+"/api/v1/auth/login",
		LoginRequest{Username: "alice", Password: "secret123"}, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
	var tok TokenResponse
	decodeBody(t, resp, &tok)
	if tok.Token == "" {
		t.Fatal("expected token")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	srv := newTestServer(t)
	registerAndLogin(t, srv, "alice", "secret123")

	resp := postJSON(t, srv.Client(), srv.URL+"/api/v1/auth/login",
		LoginRequest{Username: "alice", Password: "wrongpassword"}, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", resp.StatusCode)
	}
	drainClose(resp)
}

// --- Todo CRUD tests ---

func TestCreateTodo(t *testing.T) {
	srv := newTestServer(t)
	token := registerAndLogin(t, srv, "alice", "secret123")

	resp := postJSON(t, srv.Client(), srv.URL+"/api/v1/todos",
		CreateTodoRequest{Text: "buy milk"}, token)
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("got %d: %s", resp.StatusCode, body)
	}
	var todo Todo
	decodeBody(t, resp, &todo)
	if todo.Text != "buy milk" {
		t.Fatalf("text: got %q, want %q", todo.Text, "buy milk")
	}
	if todo.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
}

func TestCreateTodo_EmptyText(t *testing.T) {
	srv := newTestServer(t)
	token := registerAndLogin(t, srv, "alice", "secret123")

	resp := postJSON(t, srv.Client(), srv.URL+"/api/v1/todos",
		CreateTodoRequest{Text: "   "}, token)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("got %d, want 422", resp.StatusCode)
	}
	drainClose(resp)
}

func TestGetTodo(t *testing.T) {
	srv := newTestServer(t)
	token := registerAndLogin(t, srv, "alice", "secret123")

	// Create
	cr := postJSON(t, srv.Client(), srv.URL+"/api/v1/todos",
		CreateTodoRequest{Text: "walk dog"}, token)
	var created Todo
	decodeBody(t, cr, &created)

	// Get
	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/todos/%d", srv.URL, created.ID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
	var got Todo
	decodeBody(t, resp, &got)
	if got.Text != "walk dog" {
		t.Fatalf("text: got %q, want %q", got.Text, "walk dog")
	}
}

func TestGetTodo_NotFound(t *testing.T) {
	srv := newTestServer(t)
	token := registerAndLogin(t, srv, "alice", "secret123")

	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/todos/99999", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ := srv.Client().Do(req)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("got %d, want 404", resp.StatusCode)
	}
	drainClose(resp)
}

func TestUpdateTodo(t *testing.T) {
	srv := newTestServer(t)
	token := registerAndLogin(t, srv, "alice", "secret123")

	cr := postJSON(t, srv.Client(), srv.URL+"/api/v1/todos",
		CreateTodoRequest{Text: "study Go"}, token)
	var created Todo
	decodeBody(t, cr, &created)

	done := true
	b, _ := json.Marshal(UpdateTodoRequest{Done: &done})
	req, _ := http.NewRequest("PATCH", fmt.Sprintf("%s/api/v1/todos/%d", srv.URL, created.ID), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ := srv.Client().Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
	var updated Todo
	decodeBody(t, resp, &updated)
	if !updated.Done {
		t.Fatal("expected done=true")
	}
}

func TestDeleteTodo(t *testing.T) {
	srv := newTestServer(t)
	token := registerAndLogin(t, srv, "alice", "secret123")

	cr := postJSON(t, srv.Client(), srv.URL+"/api/v1/todos",
		CreateTodoRequest{Text: "clean desk"}, token)
	var created Todo
	decodeBody(t, cr, &created)

	req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/api/v1/todos/%d", srv.URL, created.ID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ := srv.Client().Do(req)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("got %d, want 204", resp.StatusCode)
	}
	drainClose(resp)

	// Second delete should 404
	resp2, _ := srv.Client().Do(req)
	if resp2.StatusCode != http.StatusNotFound {
		t.Fatalf("second delete: got %d, want 404", resp2.StatusCode)
	}
	drainClose(resp2)
}

func TestListTodos_Pagination(t *testing.T) {
	srv := newTestServer(t)
	token := registerAndLogin(t, srv, "alice", "secret123")

	// Create 5 todos
	for i := range 5 {
		resp := postJSON(t, srv.Client(), srv.URL+"/api/v1/todos",
			CreateTodoRequest{Text: fmt.Sprintf("todo %d", i)}, token)
		drainClose(resp)
	}

	// First page: limit=2
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/todos?limit=2&offset=0", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ := srv.Client().Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
	var page Page[*Todo]
	decodeBody(t, resp, &page)
	if len(page.Items) != 2 {
		t.Fatalf("items: got %d, want 2", len(page.Items))
	}
	if page.Total != 5 {
		t.Fatalf("total: got %d, want 5", page.Total)
	}
}

// --- Auth enforcement tests ---

func TestRequiresAuth(t *testing.T) {
	srv := newTestServer(t)

	// No token
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/todos", nil)
	resp, _ := srv.Client().Do(req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", resp.StatusCode)
	}
	drainClose(resp)
}

func TestForbiddenCrossUser(t *testing.T) {
	srv := newTestServer(t)
	tokenA := registerAndLogin(t, srv, "alice", "secret123")
	tokenB := registerAndLogin(t, srv, "bob", "secret456")

	// Alice creates a todo
	cr := postJSON(t, srv.Client(), srv.URL+"/api/v1/todos",
		CreateTodoRequest{Text: "alice's secret"}, tokenA)
	var created Todo
	decodeBody(t, cr, &created)

	// Bob tries to get it
	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/todos/%d", srv.URL, created.ID), nil)
	req.Header.Set("Authorization", "Bearer "+tokenB)
	resp, _ := srv.Client().Do(req)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("got %d, want 403", resp.StatusCode)
	}
	drainClose(resp)
}

// --- Health checks ---

func TestHealthLive(t *testing.T) {
	srv := newTestServer(t)
	resp, err := srv.Client().Get(srv.URL + "/healthz/live")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
	drainClose(resp)
}

func TestHealthReady(t *testing.T) {
	srv := newTestServer(t)
	resp, err := srv.Client().Get(srv.URL + "/healthz/ready")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
	drainClose(resp)
}

// --- Security headers ---

func TestSecurityHeaders(t *testing.T) {
	srv := newTestServer(t)
	resp, _ := srv.Client().Get(srv.URL + "/healthz/live")
	drainClose(resp)

	if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing X-Content-Type-Options: nosniff")
	}
	if resp.Header.Get("X-Frame-Options") != "DENY" {
		t.Error("missing X-Frame-Options: DENY")
	}
}

// --- Not found / method not allowed ---

func TestNotFound(t *testing.T) {
	srv := newTestServer(t)
	resp, _ := srv.Client().Get(srv.URL + "/does/not/exist")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("got %d, want 404", resp.StatusCode)
	}
	drainClose(resp)
}
