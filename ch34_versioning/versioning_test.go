package versioning

import (
	"encoding/json"
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

func TestV1_ResponseShape(t *testing.T) {
	srv := newServer(t)
	srv.Client().Post(srv.URL+"/api/v1/todos", "application/json",
		strings.NewReader(`{"text":"hello"}`))

	resp, _ := srv.Client().Get(srv.URL + "/api/v1/todos")
	defer resp.Body.Close()

	var items []map[string]any
	json.NewDecoder(resp.Body).Decode(&items)

	if len(items) == 0 {
		t.Fatal("expected items")
	}
	// V1 must not include created_at or priority.
	if _, ok := items[0]["created_at"]; ok {
		t.Error("V1 response should not include created_at")
	}
	if _, ok := items[0]["priority"]; ok {
		t.Error("V1 response should not include priority")
	}
}

func TestV2_ResponseShape(t *testing.T) {
	srv := newServer(t)
	srv.Client().Post(srv.URL+"/api/v2/todos", "application/json",
		strings.NewReader(`{"text":"hello","priority":"high"}`))

	resp, _ := srv.Client().Get(srv.URL + "/api/v2/todos")
	defer resp.Body.Close()

	var items []map[string]any
	json.NewDecoder(resp.Body).Decode(&items)

	if len(items) == 0 {
		t.Fatal("expected items")
	}
	// V2 must include created_at and priority.
	if _, ok := items[0]["created_at"]; !ok {
		t.Error("V2 response should include created_at")
	}
	if _, ok := items[0]["priority"]; !ok {
		t.Error("V2 response should include priority")
	}
}

func TestV1_HasDeprecationHeaders(t *testing.T) {
	srv := newServer(t)
	resp, _ := srv.Client().Get(srv.URL + "/api/v1/todos")
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.Header.Get("Deprecation") != "true" {
		t.Error("V1 should have Deprecation: true header")
	}
	if resp.Header.Get("Sunset") == "" {
		t.Error("V1 should have Sunset header")
	}
	if !strings.Contains(resp.Header.Get("Link"), `rel="successor-version"`) {
		t.Error("V1 should have successor-version Link header")
	}
}

func TestV2_NoDeprecationHeaders(t *testing.T) {
	srv := newServer(t)
	resp, _ := srv.Client().Get(srv.URL + "/api/v2/todos")
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.Header.Get("Deprecation") != "" {
		t.Error("V2 should not have Deprecation header")
	}
}

func TestGone_RetiredV0(t *testing.T) {
	srv := newServer(t)
	resp, _ := srv.Client().Get(srv.URL + "/api/v0/todos")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusGone {
		t.Fatalf("got %d, want 410", resp.StatusCode)
	}
	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["code"] != "API_VERSION_REMOVED" {
		t.Fatalf("code: got %q, want API_VERSION_REMOVED", body["code"])
	}
}

func TestSharedStore_BothVersionsSeeData(t *testing.T) {
	srv := newServer(t)
	// Create via v1.
	srv.Client().Post(srv.URL+"/api/v1/todos", "application/json",
		strings.NewReader(`{"text":"shared"}`))
	// Retrieve via v2 — same store.
	resp, _ := srv.Client().Get(srv.URL + "/api/v2/todos")
	defer resp.Body.Close()

	var items []V2Todo
	json.NewDecoder(resp.Body).Decode(&items)
	if len(items) == 0 || items[0].Text != "shared" {
		t.Fatal("v2 should see todos created via v1")
	}
}
