package pagination

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
)

type Todo struct {
	ID   int64  `json:"id"`
	Text string `json:"text"`
}

// Page is the offset-pagination response envelope.
type Page[T any] struct {
	Items   []T   `json:"items"`
	Total   int64 `json:"total"`
	Page    int   `json:"page"`
	PerPage int   `json:"per_page"`
	HasMore bool  `json:"has_more"`
}

// CursorPage is the cursor-pagination response envelope.
type CursorPage[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

// Cursor encodes the last-seen ID for stable cursor pagination.
type Cursor struct {
	AfterID int64 `json:"after_id"`
}

func encodeCursor(c Cursor) string {
	b, _ := json.Marshal(c)
	return base64.URLEncoding.EncodeToString(b)
}

func decodeCursor(s string) (Cursor, error) {
	if s == "" {
		return Cursor{}, nil
	}
	b, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, fmt.Errorf("invalid cursor")
	}
	var c Cursor
	return c, json.Unmarshal(b, &c)
}

// buildLinkHeader returns a GitHub-style Link header for offset pagination.
func buildLinkHeader(r *http.Request, page, perPage int, total int64) string {
	base := *r.URL
	var links []string

	lastPage := int((total + int64(perPage) - 1) / int64(perPage))

	addLink := func(p int, rel string) {
		u := base
		q := u.Query()
		q.Set("page", strconv.Itoa(p))
		q.Set("per_page", strconv.Itoa(perPage))
		u.RawQuery = q.Encode()
		links = append(links, fmt.Sprintf(`<%s>; rel="%s"`, u.String(), rel))
	}

	if page < lastPage {
		addLink(page+1, "next")
		addLink(lastPage, "last")
	}
	if page > 1 {
		addLink(page-1, "prev")
		addLink(1, "first")
	}
	return strings.Join(links, ", ")
}

// Store is an in-memory slice of todos with stable ID ordering.
type Store struct {
	mu     sync.RWMutex
	todos  []*Todo
	nextID int64
}

func NewStore() *Store { return &Store{nextID: 1} }

func (s *Store) Add(text string) *Todo {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := &Todo{ID: s.nextID, Text: text}
	s.todos = append(s.todos, t)
	s.nextID++
	return t
}

// List returns a page of todos sorted by ID, plus the total count.
func (s *Store) List(page, perPage int) ([]*Todo, int64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	all := make([]*Todo, len(s.todos))
	copy(all, s.todos)
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })

	total := int64(len(all))
	start := (page - 1) * perPage
	if start >= len(all) {
		return []*Todo{}, total
	}
	end := start + perPage
	if end > len(all) {
		end = len(all)
	}
	return all[start:end], total
}

// ListAfter returns up to limit todos with ID > cursor.AfterID.
func (s *Store) ListAfter(cursor Cursor, limit int) ([]*Todo, Cursor) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Todo
	for _, t := range s.todos {
		if t.ID > cursor.AfterID {
			result = append(result, t)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })

	if len(result) == 0 {
		return result, Cursor{}
	}
	if len(result) > limit {
		result = result[:limit]
	}
	return result, Cursor{AfterID: result[len(result)-1].ID}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// BuildRouter returns a chi router with both offset and cursor pagination endpoints.
func BuildRouter(store *Store) http.Handler {
	r := chi.NewRouter()

	// Seed-only endpoint for tests.
	r.Post("/todos", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Text string `json:"text"` }
		json.NewDecoder(r.Body).Decode(&req)
		writeJSON(w, http.StatusCreated, store.Add(req.Text))
	})

	// Offset pagination: GET /todos?page=1&per_page=20
	r.Get("/todos", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		page, _ := strconv.Atoi(q.Get("page"))
		perPage, _ := strconv.Atoi(q.Get("per_page"))
		if page < 1 {
			page = 1
		}
		if perPage < 1 || perPage > 100 {
			perPage = 20
		}

		items, total := store.List(page, perPage)
		if link := buildLinkHeader(r, page, perPage, total); link != "" {
			w.Header().Set("Link", link)
		}
		writeJSON(w, http.StatusOK, Page[*Todo]{
			Items:   items,
			Total:   total,
			Page:    page,
			PerPage: perPage,
			HasMore: int64(page*perPage) < total,
		})
	})

	// Cursor pagination: GET /todos/cursor?cursor=<token>&limit=20
	r.Get("/todos/cursor", func(w http.ResponseWriter, r *http.Request) {
		cursor, err := decodeCursor(r.URL.Query().Get("cursor"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid cursor")
			return
		}
		limit := 20
		if n, _ := strconv.Atoi(r.URL.Query().Get("limit")); n > 0 && n <= 100 {
			limit = n
		}

		// Fetch limit+1 to detect whether there's a next page.
		items, _ := store.ListAfter(cursor, limit+1)
		hasMore := len(items) > limit
		if hasMore {
			items = items[:limit]
		}

		resp := CursorPage[*Todo]{Items: items, HasMore: hasMore}
		if hasMore && len(items) > 0 {
			resp.NextCursor = encodeCursor(Cursor{AfterID: items[len(items)-1].ID})
		}
		writeJSON(w, http.StatusOK, resp)
	})

	return r
}
