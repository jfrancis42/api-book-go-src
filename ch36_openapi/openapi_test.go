package openapi

import (
	"io"
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

func TestSpecEndpoint_Returns200(t *testing.T) {
	srv := newServer(t)
	resp, err := srv.Client().Get(srv.URL + "/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "yaml") {
		t.Fatalf("Content-Type: got %q, want yaml", ct)
	}
}

func TestSpecEndpoint_ContainsValidYAML(t *testing.T) {
	srv := newServer(t)
	resp, _ := srv.Client().Get(srv.URL + "/openapi.yaml")
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "openapi: 3.1.0") {
		t.Fatal("spec missing openapi version declaration")
	}
	if !strings.Contains(string(body), "/todos") {
		t.Fatal("spec missing /todos path")
	}
}

func TestDocsEndpoint_Returns200(t *testing.T) {
	srv := newServer(t)
	resp, err := srv.Client().Get(srv.URL + "/docs")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("Content-Type: got %q, want text/html", ct)
	}
}

func TestDocsEndpoint_ContainsSwaggerUI(t *testing.T) {
	srv := newServer(t)
	resp, _ := srv.Client().Get(srv.URL + "/docs")
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "swagger-ui") {
		t.Fatal("docs page missing swagger-ui reference")
	}
	if !strings.Contains(string(body), "/openapi.yaml") {
		t.Fatal("docs page missing link to openapi.yaml")
	}
}

func TestSpecEmbedded_NonEmpty(t *testing.T) {
	if len(spec) == 0 {
		t.Fatal("embedded spec is empty")
	}
}
