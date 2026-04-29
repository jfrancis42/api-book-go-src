package auth

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var testSecret = []byte("test-secret-key-for-jwt-signing!")

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(BuildRouter(NewUserStore(), testSecret))
	t.Cleanup(srv.Close)
	return srv
}

func register(t *testing.T, srv *httptest.Server, username, password string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	resp, err := srv.Client().Post(srv.URL+"/auth/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("register got %d: %s", resp.StatusCode, b)
	}
	var res map[string]any
	json.NewDecoder(resp.Body).Decode(&res)
	return res["token"].(string)
}

func TestRegister_Success(t *testing.T) {
	srv := newServer(t)
	token := register(t, srv, "alice", "secret123")
	if token == "" {
		t.Fatal("expected non-empty token")
	}
}

func TestRegister_DuplicateUsername(t *testing.T) {
	srv := newServer(t)
	register(t, srv, "alice", "secret123")

	body, _ := json.Marshal(map[string]string{"username": "alice", "password": "another123"})
	resp, _ := srv.Client().Post(srv.URL+"/auth/register", "application/json", bytes.NewReader(body))
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("got %d, want 409", resp.StatusCode)
	}
}

func TestRegister_ShortPassword(t *testing.T) {
	srv := newServer(t)
	body, _ := json.Marshal(map[string]string{"username": "bob", "password": "short"})
	resp, _ := srv.Client().Post(srv.URL+"/auth/register", "application/json", bytes.NewReader(body))
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("got %d, want 422", resp.StatusCode)
	}
}

func TestRegister_ShortUsername(t *testing.T) {
	srv := newServer(t)
	body, _ := json.Marshal(map[string]string{"username": "ab", "password": "validpass"})
	resp, _ := srv.Client().Post(srv.URL+"/auth/register", "application/json", bytes.NewReader(body))
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("got %d, want 422", resp.StatusCode)
	}
}

func TestLogin_Success(t *testing.T) {
	srv := newServer(t)
	register(t, srv, "alice", "secret123")

	body, _ := json.Marshal(map[string]string{"username": "alice", "password": "secret123"})
	resp, _ := srv.Client().Post(srv.URL+"/auth/login", "application/json", bytes.NewReader(body))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
	var res map[string]any
	json.NewDecoder(resp.Body).Decode(&res)
	if res["token"].(string) == "" {
		t.Fatal("expected token in login response")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	srv := newServer(t)
	register(t, srv, "alice", "secret123")

	body, _ := json.Marshal(map[string]string{"username": "alice", "password": "wrongpassword"})
	resp, _ := srv.Client().Post(srv.URL+"/auth/login", "application/json", bytes.NewReader(body))
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", resp.StatusCode)
	}
}

func TestLogin_UnknownUser_SameErrorAsWrongPassword(t *testing.T) {
	srv := newServer(t)

	body, _ := json.Marshal(map[string]string{"username": "nobody", "password": "anything"})
	resp, _ := srv.Client().Post(srv.URL+"/auth/login", "application/json", bytes.NewReader(body))
	resp.Body.Close()

	// Must return 401, not 404 (prevents username enumeration).
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", resp.StatusCode)
	}
}

func TestRequireAuth_WithValidToken(t *testing.T) {
	srv := newServer(t)
	token := register(t, srv, "alice", "secret123")

	req, _ := http.NewRequest("GET", srv.URL+"/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ := srv.Client().Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
}

func TestRequireAuth_NoToken(t *testing.T) {
	srv := newServer(t)
	resp, _ := srv.Client().Get(srv.URL + "/me")
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", resp.StatusCode)
	}
}

func TestRequireAuth_InvalidToken(t *testing.T) {
	srv := newServer(t)
	req, _ := http.NewRequest("GET", srv.URL+"/me", nil)
	req.Header.Set("Authorization", "Bearer notavalidtoken")
	resp, _ := srv.Client().Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", resp.StatusCode)
	}
}

func TestHashAndCheck(t *testing.T) {
	hash, err := hashPassword("my-password")
	if err != nil {
		t.Fatal(err)
	}
	if checkPassword(hash, "my-password") != nil {
		t.Fatal("correct password should pass check")
	}
	if checkPassword(hash, "wrong-password") == nil {
		t.Fatal("wrong password should fail check")
	}
}

func TestGenerateAndValidateToken(t *testing.T) {
	tok, err := generateToken(testSecret, 42)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := validateToken(testSecret, tok)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != 42 {
		t.Fatalf("UserID: got %d, want 42", claims.UserID)
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	tok, _ := generateToken(testSecret, 1)
	_, err := validateToken([]byte("wrong-secret"), tok)
	if err == nil {
		t.Fatal("expected error for wrong secret")
	}
}

func TestLogin_ResponseDoesNotContainPasswordHash(t *testing.T) {
	srv := newServer(t)
	register(t, srv, "alice", "secret123")

	body, _ := json.Marshal(map[string]string{"username": "alice", "password": "secret123"})
	resp, _ := srv.Client().Post(srv.URL+"/auth/login", "application/json", bytes.NewReader(body))
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(raw), "password_hash") || strings.Contains(string(raw), "$2a$") {
		t.Fatalf("password hash leaked in response: %s", raw)
	}
}
