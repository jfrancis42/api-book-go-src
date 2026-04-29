package pagination

import (
	"encoding/json"
	"fmt"
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

func addTodos(t *testing.T, srv *httptest.Server, n int) {
	t.Helper()
	for i := range n {
		resp, _ := srv.Client().Post(srv.URL+"/todos", "application/json",
			strings.NewReader(fmt.Sprintf(`{"text":"todo %d"}`, i)))
		resp.Body.Close()
	}
}

func TestOffsetPagination_FirstPage(t *testing.T) {
	srv := newServer(t)
	addTodos(t, srv, 10)

	resp, _ := srv.Client().Get(srv.URL + "/todos?page=1&per_page=3")
	defer resp.Body.Close()

	var page Page[*Todo]
	json.NewDecoder(resp.Body).Decode(&page)

	if len(page.Items) != 3 {
		t.Fatalf("items: got %d, want 3", len(page.Items))
	}
	if page.Total != 10 {
		t.Fatalf("total: got %d, want 10", page.Total)
	}
	if !page.HasMore {
		t.Fatal("expected has_more=true")
	}
}

func TestOffsetPagination_LastPage(t *testing.T) {
	srv := newServer(t)
	addTodos(t, srv, 5)

	resp, _ := srv.Client().Get(srv.URL + "/todos?page=2&per_page=3")
	defer resp.Body.Close()

	var page Page[*Todo]
	json.NewDecoder(resp.Body).Decode(&page)

	if len(page.Items) != 2 {
		t.Fatalf("items: got %d, want 2 (10-(3*1))", len(page.Items))
	}
	if page.HasMore {
		t.Fatal("expected has_more=false on last page")
	}
}

func TestLinkHeader_FirstPage(t *testing.T) {
	srv := newServer(t)
	addTodos(t, srv, 50)

	resp, _ := srv.Client().Get(srv.URL + "/todos?page=1&per_page=20")
	resp.Body.Close()

	link := resp.Header.Get("Link")
	if !strings.Contains(link, `rel="next"`) {
		t.Fatalf("Link missing next: %s", link)
	}
	if !strings.Contains(link, `rel="last"`) {
		t.Fatalf("Link missing last: %s", link)
	}
}

func TestLinkHeader_MiddlePage(t *testing.T) {
	srv := newServer(t)
	addTodos(t, srv, 50)

	resp, _ := srv.Client().Get(srv.URL + "/todos?page=2&per_page=20")
	resp.Body.Close()

	link := resp.Header.Get("Link")
	for _, rel := range []string{"next", "prev", "first", "last"} {
		if !strings.Contains(link, `rel="`+rel+`"`) {
			t.Errorf("Link missing %s: %s", rel, link)
		}
	}
}

func TestLinkHeader_LastPage(t *testing.T) {
	srv := newServer(t)
	addTodos(t, srv, 5)

	resp, _ := srv.Client().Get(srv.URL + "/todos?page=2&per_page=3")
	resp.Body.Close()

	link := resp.Header.Get("Link")
	if strings.Contains(link, `rel="next"`) {
		t.Fatalf("last page should not have next link: %s", link)
	}
	if !strings.Contains(link, `rel="prev"`) {
		t.Fatalf("last page should have prev link: %s", link)
	}
}

func TestCursorPagination_FirstPage(t *testing.T) {
	srv := newServer(t)
	addTodos(t, srv, 10)

	resp, _ := srv.Client().Get(srv.URL + "/todos/cursor?limit=3")
	defer resp.Body.Close()

	var page CursorPage[*Todo]
	json.NewDecoder(resp.Body).Decode(&page)

	if len(page.Items) != 3 {
		t.Fatalf("items: got %d, want 3", len(page.Items))
	}
	if !page.HasMore {
		t.Fatal("expected has_more=true")
	}
	if page.NextCursor == "" {
		t.Fatal("expected non-empty next_cursor")
	}
}

func TestCursorPagination_WalkAllPages(t *testing.T) {
	srv := newServer(t)
	addTodos(t, srv, 10)

	var cursor string
	var seen int
	for {
		url := srv.URL + "/todos/cursor?limit=3"
		if cursor != "" {
			url += "&cursor=" + cursor
		}
		resp, _ := srv.Client().Get(url)
		var page CursorPage[*Todo]
		json.NewDecoder(resp.Body).Decode(&page)
		resp.Body.Close()

		seen += len(page.Items)
		if !page.HasMore {
			break
		}
		cursor = page.NextCursor
	}
	if seen != 10 {
		t.Fatalf("walked %d items, want 10", seen)
	}
}

func TestCursorPagination_InvalidCursor(t *testing.T) {
	srv := newServer(t)
	resp, _ := srv.Client().Get(srv.URL + "/todos/cursor?cursor=notbase64!!!")
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", resp.StatusCode)
	}
}
