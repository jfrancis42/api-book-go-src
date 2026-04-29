package complete

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

type contextKey int

const (
	keyUserID   contextKey = iota
	keyUsername contextKey = iota
)

func userIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(keyUserID).(int64)
	return id, ok
}

func usernameFromContext(ctx context.Context) string {
	s, _ := ctx.Value(keyUsername).(string)
	return s
}

// RequireAuth validates the Bearer JWT and injects user identity into context.
func RequireAuth(jwtSecret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hdr := r.Header.Get("Authorization")
			if !strings.HasPrefix(hdr, "Bearer ") {
				writeJSON(w, http.StatusUnauthorized, ErrorResponse{
					Error: "missing or invalid authorization header",
					Code:  "UNAUTHORIZED",
				})
				return
			}
			c, err := parseToken(jwtSecret, strings.TrimPrefix(hdr, "Bearer "))
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, ErrorResponse{
					Error: "invalid token",
					Code:  "UNAUTHORIZED",
				})
				return
			}
			ctx := context.WithValue(r.Context(), keyUserID, c.UserID)
			ctx = context.WithValue(ctx, keyUsername, c.Username)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Logger logs method, path, status, and latency using slog.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(rw, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.code,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.code = code
	sw.ResponseWriter.WriteHeader(code)
}

// RateLimiter returns a middleware that applies a global token-bucket rate limit.
func RateLimiter(r rate.Limit, burst int) func(http.Handler) http.Handler {
	lim := rate.NewLimiter(r, burst)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !lim.Allow() {
				w.Header().Set("Retry-After", "1")
				writeJSON(w, http.StatusTooManyRequests, ErrorResponse{
					Error: "rate limit exceeded",
					Code:  "RATE_LIMITED",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders adds basic security headers to every response.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// Recoverer catches panics and returns 500.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recovered", "panic", rec)
				writeJSON(w, http.StatusInternalServerError, ErrorResponse{
					Error: "internal server error",
					Code:  "INTERNAL_ERROR",
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
