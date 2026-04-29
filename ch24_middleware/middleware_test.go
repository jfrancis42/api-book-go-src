package middleware

import (
	"bytes"
	"log/slog"
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

func TestLogger_SetsStatusInLog(t *testing.T) {
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))

	srv := newServer(t)
	resp, err := srv.Client().Get(srv.URL + "/hello")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
	log := buf.String()
	if !strings.Contains(log, `"status":200`) {
		t.Fatalf("log missing status:200, got: %s", log)
	}
}

func TestRequestID_GeneratedWhenAbsent(t *testing.T) {
	srv := newServer(t)
	resp, _ := srv.Client().Get(srv.URL + "/hello")
	resp.Body.Close()

	if id := resp.Header.Get("X-Request-ID"); id == "" {
		t.Fatal("expected X-Request-ID header in response")
	}
}

func TestRequestID_EchosClientHeader(t *testing.T) {
	srv := newServer(t)
	req, _ := http.NewRequest("GET", srv.URL+"/hello", nil)
	req.Header.Set("X-Request-ID", "my-trace-id")
	resp, _ := srv.Client().Do(req)
	resp.Body.Close()

	if got := resp.Header.Get("X-Request-ID"); got != "my-trace-id" {
		t.Fatalf("got %q, want %q", got, "my-trace-id")
	}
}

func TestRequestID_InResponseBody(t *testing.T) {
	srv := newServer(t)
	req, _ := http.NewRequest("GET", srv.URL+"/hello", nil)
	req.Header.Set("X-Request-ID", "abc-123")
	resp, _ := srv.Client().Do(req)
	defer resp.Body.Close()

	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	if !strings.Contains(buf.String(), "abc-123") {
		t.Fatalf("response body missing request_id, got: %s", buf.String())
	}
}

func TestRecoverer_CatchesPanic(t *testing.T) {
	srv := newServer(t)
	resp, err := srv.Client().Get(srv.URL + "/panic")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("got %d, want 500", resp.StatusCode)
	}
}

func TestCORS_AllowedOrigin(t *testing.T) {
	srv := newServer(t)
	req, _ := http.NewRequest("GET", srv.URL+"/hello", nil)
	req.Header.Set("Origin", "https://example.com")
	resp, _ := srv.Client().Do(req)
	resp.Body.Close()

	got := resp.Header.Get("Access-Control-Allow-Origin")
	if got != "https://example.com" {
		t.Fatalf("ACAO: got %q, want %q", got, "https://example.com")
	}
}

func TestCORS_Preflight(t *testing.T) {
	srv := newServer(t)
	req, _ := http.NewRequest(http.MethodOptions, srv.URL+"/hello", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	resp, _ := srv.Client().Do(req)
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("preflight: got %d, want 204", resp.StatusCode)
	}
}

func TestCORS_DisallowedOrigin(t *testing.T) {
	srv := newServer(t)
	req, _ := http.NewRequest("GET", srv.URL+"/hello", nil)
	req.Header.Set("Origin", "https://evil.com")
	resp, _ := srv.Client().Do(req)
	resp.Body.Close()

	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("ACAO should be absent for disallowed origin, got %q", got)
	}
}

func TestCORSAllStar(t *testing.T) {
	h := CORS([]string{"*"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "https://anything.com")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("expected * for wildcard CORS, got %q", rr.Header().Get("Access-Control-Allow-Origin"))
	}
}
