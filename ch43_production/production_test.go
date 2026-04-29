package production

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func newServer(t *testing.T, checks ...func() error) *httptest.Server {
	t.Helper()
	probe := NewReadinessProbe(checks...)
	srv := httptest.NewServer(BuildRouter(probe))
	t.Cleanup(srv.Close)
	return srv
}

func TestSecurityHeaders_Present(t *testing.T) {
	srv := newServer(t)
	resp, _ := srv.Client().Get(srv.URL + "/healthz/live")
	defer resp.Body.Close()

	headers := map[string]string{
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
		"Content-Security-Policy":   "default-src 'self'",
		"Referrer-Policy":           "strict-origin-when-cross-origin",
	}
	for header, want := range headers {
		if got := resp.Header.Get(header); got != want {
			t.Errorf("%s: got %q, want %q", header, got, want)
		}
	}
}

func TestSecurityHeaders_OnErrorResponse(t *testing.T) {
	srv := newServer(t)
	// /notfound triggers a 404 — security headers must still be present
	resp, _ := srv.Client().Get(srv.URL + "/notfound")
	defer resp.Body.Close()

	if resp.Header.Get("X-Frame-Options") != "DENY" {
		t.Fatal("X-Frame-Options missing on 404 response")
	}
}

func TestLiveness_Returns200(t *testing.T) {
	srv := newServer(t)
	resp, _ := srv.Client().Get(srv.URL + "/healthz/live")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
}

func TestReadiness_AllChecksPass(t *testing.T) {
	srv := newServer(t,
		func() error { return nil },
		func() error { return nil },
	)
	resp, _ := srv.Client().Get(srv.URL + "/healthz/ready")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
}

func TestReadiness_CheckFails(t *testing.T) {
	srv := newServer(t, func() error { return errors.New("db down") })
	resp, _ := srv.Client().Get(srv.URL + "/healthz/ready")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503", resp.StatusCode)
	}
}

func TestFromEnv_MissingJWTSecret(t *testing.T) {
	os.Unsetenv("JWT_SECRET")
	os.Unsetenv("DB_CONN")
	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected error when JWT_SECRET is missing")
	}
}

func TestFromEnv_MissingDBConn(t *testing.T) {
	os.Setenv("JWT_SECRET", "s3cr3t")
	defer os.Unsetenv("JWT_SECRET")
	os.Unsetenv("DB_CONN")

	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected error when DB_CONN is missing")
	}
}

func TestFromEnv_TLSPartialConfig(t *testing.T) {
	os.Setenv("JWT_SECRET", "s3cr3t")
	os.Setenv("DB_CONN", "sqlite:///tmp/test.db")
	os.Setenv("TLS_CERT", "/path/to/cert.pem")
	defer func() {
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("DB_CONN")
		os.Unsetenv("TLS_CERT")
	}()
	os.Unsetenv("TLS_KEY")

	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected error when TLS_CERT set but TLS_KEY missing")
	}
}

func TestFromEnv_Defaults(t *testing.T) {
	os.Setenv("JWT_SECRET", "s3cr3t")
	os.Setenv("DB_CONN", "sqlite:///tmp/test.db")
	defer func() {
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("DB_CONN")
		os.Unsetenv("ADDR")
	}()
	os.Unsetenv("ADDR")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Addr != ":8080" {
		t.Fatalf("Addr: got %q, want :8080", cfg.Addr)
	}
}
