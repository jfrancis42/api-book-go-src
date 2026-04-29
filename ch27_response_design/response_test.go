package response

import (
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
	srv := httptest.NewServer(BuildRouter())
	t.Cleanup(srv.Close)
	return srv
}

func TestCreate_Returns201WithLocation(t *testing.T) {
	srv := newServer(t)
	resp, err := srv.Client().Post(srv.URL+"/todos", "application/json",
		strings.NewReader(`{"text":"buy milk"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("got %d, want 201", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); !strings.HasPrefix(loc, "/todos/") {
		t.Fatalf("Location: got %q, want /todos/N", loc)
	}
}

func TestCreate_TimestampsPresent(t *testing.T) {
	srv := newServer(t)
	resp, _ := srv.Client().Post(srv.URL+"/todos", "application/json",
		strings.NewReader(`{"text":"check timestamps"}`))
	defer resp.Body.Close()

	var todo Todo
	json.NewDecoder(resp.Body).Decode(&todo)
	if todo.CreatedAt.IsZero() {
		t.Fatal("created_at is zero")
	}
	if todo.UpdatedAt.IsZero() {
		t.Fatal("updated_at is zero")
	}
}

func TestCreate_ValidationError(t *testing.T) {
	srv := newServer(t)
	resp, _ := srv.Client().Post(srv.URL+"/todos", "application/json",
		strings.NewReader(`{"text":""}`))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("got %d, want 422", resp.StatusCode)
	}
	var er ErrorResponse
	json.NewDecoder(resp.Body).Decode(&er)
	if er.Code != ErrCodeValidation {
		t.Fatalf("code: got %q, want %q", er.Code, ErrCodeValidation)
	}
	if len(er.Fields) == 0 {
		t.Fatal("expected field errors")
	}
}

func TestGet_NotFound(t *testing.T) {
	srv := newServer(t)
	resp, _ := srv.Client().Get(srv.URL + "/todos/99")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("got %d, want 404", resp.StatusCode)
	}
	var er ErrorResponse
	json.NewDecoder(resp.Body).Decode(&er)
	if er.Code != ErrCodeNotFound {
		t.Fatalf("code: got %q, want %q", er.Code, ErrCodeNotFound)
	}
}

func TestList_Pagination(t *testing.T) {
	srv := newServer(t)
	for i := range 5 {
		srv.Client().Post(srv.URL+"/todos", "application/json",
			strings.NewReader(fmt.Sprintf(`{"text":"todo %d"}`, i)))
	}

	resp, _ := srv.Client().Get(srv.URL + "/todos?page=1&per_page=2")
	defer resp.Body.Close()

	var page Page[*Todo]
	json.NewDecoder(resp.Body).Decode(&page)
	if len(page.Items) != 2 {
		t.Fatalf("items: got %d, want 2", len(page.Items))
	}
	if page.Total != 5 {
		t.Fatalf("total: got %d, want 5", page.Total)
	}
	if !page.HasMore {
		t.Fatal("expected has_more=true")
	}
}

func TestList_LastPageHasMoreFalse(t *testing.T) {
	srv := newServer(t)
	srv.Client().Post(srv.URL+"/todos", "application/json", strings.NewReader(`{"text":"only one"}`))

	resp, _ := srv.Client().Get(srv.URL + "/todos?page=1&per_page=20")
	defer resp.Body.Close()

	var page Page[*Todo]
	json.NewDecoder(resp.Body).Decode(&page)
	if page.HasMore {
		t.Fatal("expected has_more=false on last page")
	}
}

func TestNewPage(t *testing.T) {
	items := []int{1, 2, 3}
	p := NewPage(items, 10, 1, 3)
	if !p.HasMore {
		t.Fatal("expected HasMore=true when page*perPage < total")
	}
	p2 := NewPage(items, 3, 1, 3)
	if p2.HasMore {
		t.Fatal("expected HasMore=false on last page")
	}
}

func TestContentTypeJSON(t *testing.T) {
	srv := newServer(t)
	resp, _ := srv.Client().Get(srv.URL + "/todos")
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Fatalf("Content-Type: got %q, want application/json", ct)
	}
}
