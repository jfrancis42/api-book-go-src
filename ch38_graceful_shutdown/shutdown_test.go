package shutdown

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func newServer(t *testing.T, ping func(ctx context.Context) error) *httptest.Server {
	t.Helper()
	hc := NewHealthChecker(ping)
	srv := httptest.NewServer(BuildRouter(hc))
	t.Cleanup(srv.Close)
	return srv
}

func TestLiveness_ReturnsOK(t *testing.T) {
	srv := newServer(t, func(ctx context.Context) error { return nil })
	resp, _ := srv.Client().Get(srv.URL + "/healthz/live")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Fatalf("status: got %q, want ok", body["status"])
	}
}

func TestReadiness_DatabaseOK(t *testing.T) {
	srv := newServer(t, func(ctx context.Context) error { return nil })
	resp, _ := srv.Client().Get(srv.URL + "/healthz/ready")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
}

func TestReadiness_DatabaseDown(t *testing.T) {
	srv := newServer(t, func(ctx context.Context) error {
		return fmt.Errorf("connection refused")
	})
	resp, _ := srv.Client().Get(srv.URL + "/healthz/ready")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503", resp.StatusCode)
	}
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "degraded" {
		t.Fatalf("status: got %v, want degraded", body["status"])
	}
}

func TestReadiness_UptimePresent(t *testing.T) {
	srv := newServer(t, func(ctx context.Context) error { return nil })
	resp, _ := srv.Client().Get(srv.URL + "/healthz/ready")
	defer resp.Body.Close()

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if _, ok := body["uptime_seconds"]; !ok {
		t.Fatal("expected uptime_seconds in readiness response")
	}
}

func TestSlowEndpoint_CompletesNormally(t *testing.T) {
	srv := newServer(t, func(ctx context.Context) error { return nil })
	resp, _ := srv.Client().Get(srv.URL + "/slow")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
}

func TestFromEnv_RequiresJWTSecret(t *testing.T) {
	os.Unsetenv("JWT_SECRET")
	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected error when JWT_SECRET is not set")
	}
}

func TestFromEnv_Defaults(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret")
	defer os.Unsetenv("JWT_SECRET")
	os.Unsetenv("ADDR")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":8080" {
		t.Fatalf("default addr: got %q, want :8080", cfg.Addr)
	}
}

func TestContentType_JSON(t *testing.T) {
	srv := newServer(t, func(ctx context.Context) error { return nil })
	resp, _ := srv.Client().Get(srv.URL + "/healthz/live")
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type: got %q", ct)
	}
}
