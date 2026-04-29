package testing

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// This module demonstrates the test helper patterns from chapter 39.
// The "server" here is the thing being tested; the real chapter content
// is in server_test.go.

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")

type User struct {
	ID           int64
	Username     string
	PasswordHash string
}

type Todo struct {
	ID     int64  `json:"id"`
	UserID int64  `json:"user_id"`
	Text   string `json:"text"`
	Done   bool   `json:"done"`
}

type UserStore struct {
	mu     sync.Mutex
	users  map[string]*User
	nextID int64
}

func NewUserStore() *UserStore {
	return &UserStore{users: make(map[string]*User), nextID: 1}
}

func (s *UserStore) Create(username, hash string) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[username]; ok {
		return nil, ErrConflict
	}
	u := &User{ID: s.nextID, Username: username, PasswordHash: hash}
	s.users[username] = u
	s.nextID++
	return u, nil
}

func (s *UserStore) GetByUsername(username string) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[username]
	if !ok {
		return nil, ErrNotFound
	}
	return u, nil
}

type TodoStore struct {
	mu     sync.Mutex
	todos  map[int64]*Todo
	nextID int64
}

func NewTodoStore() *TodoStore {
	return &TodoStore{todos: make(map[int64]*Todo), nextID: 1}
}

func (s *TodoStore) Create(userID int64, text string) *Todo {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := &Todo{ID: s.nextID, UserID: userID, Text: text}
	s.todos[s.nextID] = t
	s.nextID++
	return t
}

func (s *TodoStore) Get(id int64) (*Todo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.todos[id]
	if !ok {
		return nil, ErrNotFound
	}
	return t, nil
}

func (s *TodoStore) ListByUser(userID int64) []*Todo {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Todo
	for _, t := range s.todos {
		if t.UserID == userID {
			out = append(out, t)
		}
	}
	return out
}

type claims struct {
	UserID int64 `json:"uid"`
	jwt.RegisteredClaims
}

func issueToken(secret []byte, userID int64) (string, error) {
	c := claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(secret)
}

func parseToken(secret []byte, tok string) (*claims, error) {
	t, err := jwt.ParseWithClaims(tok, &claims{}, func(t *jwt.Token) (any, error) {
		return secret, nil
	})
	if err != nil {
		return nil, err
	}
	c, ok := t.Claims.(*claims)
	if !ok || !t.Valid {
		return nil, errors.New("invalid token")
	}
	return c, nil
}

type contextKey string

const userKey contextKey = "uid"

func requireAuth(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			c, err := parseToken(secret, auth[7:])
			if err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), userKey, c.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func currentUserID(ctx context.Context) int64 {
	id, _ := ctx.Value(userKey).(int64)
	return id
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// BuildRouter returns the server to be tested.
func BuildRouter(users *UserStore, todos *TodoStore, secret []byte) http.Handler {
	r := chi.NewRouter()

	r.Post("/auth/register", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if len(req.Password) < 8 {
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}
		hash, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.MinCost)
		user, err := users.Create(req.Username, string(hash))
		if err != nil {
			if errors.Is(err, ErrConflict) {
				w.WriteHeader(http.StatusConflict)
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		token, _ := issueToken(secret, user.ID)
		writeJSON(w, http.StatusCreated, map[string]string{"token": token})
	})

	r.Post("/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		user, err := users.GetByUsername(req.Username)
		if err != nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		token, _ := issueToken(secret, user.ID)
		writeJSON(w, http.StatusOK, map[string]string{"token": token})
	})

	r.Group(func(r chi.Router) {
		r.Use(requireAuth(secret))

		r.Get("/todos", func(w http.ResponseWriter, r *http.Request) {
			uid := currentUserID(r.Context())
			items := todos.ListByUser(uid)
			if items == nil {
				items = []*Todo{}
			}
			writeJSON(w, http.StatusOK, items)
		})

		r.Post("/todos", func(w http.ResponseWriter, r *http.Request) {
			uid := currentUserID(r.Context())
			var req struct{ Text string `json:"text"` }
			json.NewDecoder(r.Body).Decode(&req)
			if strings.TrimSpace(req.Text) == "" {
				w.WriteHeader(http.StatusUnprocessableEntity)
				return
			}
			writeJSON(w, http.StatusCreated, todos.Create(uid, req.Text))
		})

		r.Get("/todos/{id}", func(w http.ResponseWriter, r *http.Request) {
			uid := currentUserID(r.Context())
			var id int64
			for _, c := range chi.URLParam(r, "id") {
				id = id*10 + int64(c-'0')
			}
			todo, err := todos.Get(id)
			if err != nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if todo.UserID != uid {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			writeJSON(w, http.StatusOK, todo)
		})
	})

	return r
}
