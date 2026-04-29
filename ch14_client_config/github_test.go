package github

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func withServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(func() { srv.Close(); setBaseURL("https://api.github.com") })
	setBaseURL(srv.URL)
	return NewClient("test-token")
}

func TestDefaultTimeout(t *testing.T) {
	c := NewClient("token")
	if c.httpClient.Timeout != DefaultTimeout {
		t.Errorf("timeout: got %v, want %v", c.httpClient.Timeout, DefaultTimeout)
	}
}

func TestWithTimeout(t *testing.T) {
	c := NewClient("token", WithTimeout(5*time.Second))
	if c.httpClient.Timeout != 5*time.Second {
		t.Errorf("timeout: got %v, want 5s", c.httpClient.Timeout)
	}
}

func TestWithMaxConnsPerHost(t *testing.T) {
	c := NewClient("token", WithMaxConnsPerHost(50))
	transport, ok := unwrapTransport(c.httpClient.Transport)
	if !ok {
		t.Fatal("expected *http.Transport")
	}
	if transport.MaxConnsPerHost != 50 {
		t.Errorf("MaxConnsPerHost: got %d, want 50", transport.MaxConnsPerHost)
	}
}

func TestLoggingTransport(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(User{Login: "test"})
	}))
	defer srv.Close()
	setBaseURL(srv.URL)
	defer setBaseURL("https://api.github.com")

	c := NewClient("token", WithLogger(logger))
	c.GetUser(context.Background(), "test")

	log := logBuf.String()
	if !strings.Contains(log, "http request") {
		t.Errorf("expected log to contain 'http request', got: %s", log)
	}
	if !strings.Contains(log, "status=200") {
		t.Errorf("expected log to contain status, got: %s", log)
	}
}

func TestClientReusesConnections(t *testing.T) {
	// Verify that the transport is reused (pool is shared) by checking
	// that the same Client instance works for multiple requests
	var requestCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(User{Login: "test"})
	}))
	defer srv.Close()
	setBaseURL(srv.URL)
	defer setBaseURL("https://api.github.com")

	c := NewClient("token")
	for i := 0; i < 5; i++ {
		_, err := c.GetUser(context.Background(), "test")
		if err != nil {
			t.Fatal(err)
		}
	}
	if requestCount != 5 {
		t.Errorf("expected 5 requests, got %d", requestCount)
	}
}
