package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// User is the stored user record.
type User struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
}

// Claims is the JWT payload.
type Claims struct {
	UserID int64 `json:"user_id"`
	jwt.RegisteredClaims
}

var ErrConflict = errors.New("username already taken")

// hashPassword returns a bcrypt hash of the password.
func hashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

// checkPassword returns nil if password matches hash.
func checkPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// generateToken signs a JWT for the given user.
func generateToken(secret []byte, userID int64) (string, error) {
	c := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(secret)
}

// validateToken parses and validates the JWT, returning the claims.
func validateToken(secret []byte, tokenStr string) (*Claims, error) {
	tok, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	})
	if err != nil {
		return nil, err
	}
	c, ok := tok.Claims.(*Claims)
	if !ok || !tok.Valid {
		return nil, errors.New("invalid token")
	}
	return c, nil
}

type contextKey string

const userIDKey contextKey = "user_id"

// RequireAuth validates the Bearer JWT and injects the user ID into context.
func RequireAuth(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				writeJSON(w, http.StatusUnauthorized, map[string]string{
					"error": "authorization required",
					"code":  "UNAUTHORIZED",
				})
				return
			}
			claims, err := validateToken(secret, auth[7:])
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{
					"error": "invalid or expired token",
					"code":  "INVALID_TOKEN",
				})
				return
			}
			ctx := context.WithValue(r.Context(), userIDKey, claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// CurrentUserID retrieves the authenticated user's ID from context.
func CurrentUserID(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(userIDKey).(int64)
	return id, ok
}

// UserStore is an in-memory user store.
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
	if _, exists := s.users[username]; exists {
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
		return nil, fmt.Errorf("user not found")
	}
	return u, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// BuildRouter returns a chi router demonstrating JWT authentication.
func BuildRouter(store *UserStore, secret []byte) http.Handler {
	r := chi.NewRouter()

	// POST /auth/register — create a new user, return JWT.
	r.Post("/auth/register", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if len(req.Username) < 3 || len(req.Username) > 30 {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
				"error": "username must be 3–30 characters",
			})
			return
		}
		if len(req.Password) < 8 {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
				"error": "password must be at least 8 characters",
			})
			return
		}
		hash, err := hashPassword(req.Password)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server error"})
			return
		}
		user, err := store.Create(req.Username, hash)
		if err != nil {
			if errors.Is(err, ErrConflict) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "username already taken"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server error"})
			return
		}
		token, _ := generateToken(secret, user.ID)
		writeJSON(w, http.StatusCreated, map[string]any{"user": user, "token": token})
	})

	// POST /auth/login — verify credentials, return JWT.
	r.Post("/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		user, err := store.GetByUsername(req.Username)
		// Return the same error for "user not found" and "wrong password" — prevents enumeration.
		if err != nil || checkPassword(user.PasswordHash, req.Password) != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid username or password"})
			return
		}
		token, _ := generateToken(secret, user.ID)
		writeJSON(w, http.StatusOK, map[string]any{"user": user, "token": token})
	})

	// GET /me — protected: requires valid JWT.
	r.With(RequireAuth(secret)).Get("/me", func(w http.ResponseWriter, r *http.Request) {
		userID, _ := CurrentUserID(r.Context())
		writeJSON(w, http.StatusOK, map[string]int64{"user_id": userID})
	})

	// GET /todos — protected endpoint illustrating per-user authorization.
	r.With(RequireAuth(secret)).Get("/todos", func(w http.ResponseWriter, r *http.Request) {
		userID, _ := CurrentUserID(r.Context())
		writeJSON(w, http.StatusOK, map[string]any{
			"user_id": userID,
			"todos":   []any{},
		})
	})

	return r
}
