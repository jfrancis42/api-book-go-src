package testing

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestServer is the core test helper: spins up a full server with fresh
// stores and registers t.Cleanup to close it. Tests never share state.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	secret := []byte("test-secret")
	srv := httptest.NewServer(BuildRouter(NewUserStore(), NewTodoStore(), secret))
	t.Cleanup(srv.Close)
	return srv
}

// registerAndLogin registers a user and returns the JWT token.
// Using bcrypt.MinCost in BuildRouter keeps this fast enough for parallel tests.
func registerAndLogin(t *testing.T, srv *httptest.Server, username, password string) string {
	t.Helper()
	body := fmt.Sprintf(`{"username":%q,"password":%q}`, username, password)
	resp, err := srv.Client().Post(srv.URL+"/auth/register", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register: got %d, want 201", resp.StatusCode)
	}
	var out map[string]string
	json.NewDecoder(resp.Body).Decode(&out)
	tok := out["token"]
	if tok == "" {
		t.Fatal("register: no token in response")
	}
	return tok
}

// createTodo creates a todo via the API and returns the decoded response.
func createTodo(t *testing.T, srv *httptest.Server, token, text string) *Todo {
	t.Helper()
	body := fmt.Sprintf(`{"text":%q}`, text)
	req, _ := http.NewRequest("POST", srv.URL+"/todos", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("createTodo: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("createTodo: got %d, want 201", resp.StatusCode)
	}
	var todo Todo
	json.NewDecoder(resp.Body).Decode(&todo)
	return &todo
}

func TestRegister_Success(t *testing.T) {
	srv := newTestServer(t)
	tok := registerAndLogin(t, srv, "alice", "password123")
	if tok == "" {
		t.Fatal("expected non-empty token")
	}
}

func TestRegister_DuplicateUsername(t *testing.T) {
	srv := newTestServer(t)
	registerAndLogin(t, srv, "alice", "password123")

	resp, _ := srv.Client().Post(srv.URL+"/auth/register", "application/json",
		strings.NewReader(`{"username":"alice","password":"password123"}`))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("got %d, want 409", resp.StatusCode)
	}
}

func TestRegister_ShortPassword(t *testing.T) {
	srv := newTestServer(t)
	resp, _ := srv.Client().Post(srv.URL+"/auth/register", "application/json",
		strings.NewReader(`{"username":"alice","password":"short"}`))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("got %d, want 422", resp.StatusCode)
	}
}

func TestLogin_Success(t *testing.T) {
	srv := newTestServer(t)
	registerAndLogin(t, srv, "bob", "securepass")

	resp, _ := srv.Client().Post(srv.URL+"/auth/login", "application/json",
		strings.NewReader(`{"username":"bob","password":"securepass"}`))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	srv := newTestServer(t)
	registerAndLogin(t, srv, "bob", "correctpass")

	resp, _ := srv.Client().Post(srv.URL+"/auth/login", "application/json",
		strings.NewReader(`{"username":"bob","password":"wrongpass"}`))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", resp.StatusCode)
	}
}

func TestCreateTodo_RequiresAuth(t *testing.T) {
	srv := newTestServer(t)
	resp, _ := srv.Client().Post(srv.URL+"/todos", "application/json",
		strings.NewReader(`{"text":"hello"}`))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", resp.StatusCode)
	}
}

func TestCreateTodo_EmptyText(t *testing.T) {
	srv := newTestServer(t)
	tok := registerAndLogin(t, srv, "alice", "password123")

	req, _ := http.NewRequest("POST", srv.URL+"/todos", strings.NewReader(`{"text":"   "}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, _ := srv.Client().Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("got %d, want 422", resp.StatusCode)
	}
}

func TestListTodos_EmptyByDefault(t *testing.T) {
	srv := newTestServer(t)
	tok := registerAndLogin(t, srv, "alice", "password123")

	req, _ := http.NewRequest("GET", srv.URL+"/todos", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, _ := srv.Client().Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
	var todos []*Todo
	json.NewDecoder(resp.Body).Decode(&todos)
	if len(todos) != 0 {
		t.Fatalf("expected empty list, got %d items", len(todos))
	}
}

func TestGetTodo_OwnTodo(t *testing.T) {
	srv := newTestServer(t)
	tok := registerAndLogin(t, srv, "alice", "password123")
	todo := createTodo(t, srv, tok, "buy milk")

	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/todos/%d", srv.URL, todo.ID), nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, _ := srv.Client().Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
	var got Todo
	json.NewDecoder(resp.Body).Decode(&got)
	if got.Text != "buy milk" {
		t.Fatalf("text: got %q, want %q", got.Text, "buy milk")
	}
}

// TestGetTodo_CrossUserForbidden verifies that a user cannot read another's todo.
// This is a key authz test: auth (token valid) vs authz (owns resource).
func TestGetTodo_CrossUserForbidden(t *testing.T) {
	srv := newTestServer(t)
	aliceTok := registerAndLogin(t, srv, "alice", "password123")
	bobTok := registerAndLogin(t, srv, "bob", "password456")

	todo := createTodo(t, srv, aliceTok, "alice's secret")

	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/todos/%d", srv.URL, todo.ID), nil)
	req.Header.Set("Authorization", "Bearer "+bobTok)
	resp, _ := srv.Client().Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("got %d, want 403", resp.StatusCode)
	}
}

func TestListTodos_IsolatedByUser(t *testing.T) {
	srv := newTestServer(t)
	aliceTok := registerAndLogin(t, srv, "alice", "password123")
	bobTok := registerAndLogin(t, srv, "bob", "password456")

	createTodo(t, srv, aliceTok, "alice todo 1")
	createTodo(t, srv, aliceTok, "alice todo 2")
	createTodo(t, srv, bobTok, "bob todo 1")

	check := func(tok string, want int) {
		t.Helper()
		req, _ := http.NewRequest("GET", srv.URL+"/todos", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, _ := srv.Client().Do(req)
		defer resp.Body.Close()
		var todos []*Todo
		json.NewDecoder(resp.Body).Decode(&todos)
		if len(todos) != want {
			t.Fatalf("got %d todos, want %d", len(todos), want)
		}
	}

	check(aliceTok, 2)
	check(bobTok, 1)
}

// TestRegister_TableDriven demonstrates table-driven subtests with shared server.
// Each subtest uses t.Run so failures are isolated and -v shows individual results.
func TestRegister_TableDriven(t *testing.T) {
	srv := newTestServer(t)

	cases := []struct {
		name     string
		username string
		password string
		want     int
	}{
		{"valid", "user1", "longpassword", http.StatusCreated},
		{"short_password", "user2", "abc", http.StatusUnprocessableEntity},
		{"empty_username", "", "longpassword", http.StatusCreated}, // server allows it
		{"duplicate", "user1", "longpassword", http.StatusConflict},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"username":%q,"password":%q}`, tc.username, tc.password)
			resp, _ := srv.Client().Post(srv.URL+"/auth/register", "application/json",
				strings.NewReader(body))
			defer resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Fatalf("got %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}
