package ratelimit

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/time/rate"
)

type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter tracks a per-key (IP or user ID) token-bucket limiter.
type RateLimiter struct {
	mu      sync.Mutex
	clients map[string]*clientLimiter
	limit   rate.Limit
	burst   int
}

// NewRateLimiter creates a per-key rate limiter with periodic stale-entry cleanup.
func NewRateLimiter(limit rate.Limit, burst int) *RateLimiter {
	rl := &RateLimiter{
		clients: make(map[string]*clientLimiter),
		limit:   limit,
		burst:   burst,
	}
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) getLimiter(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	cl, ok := rl.clients[key]
	if !ok {
		cl = &clientLimiter{limiter: rate.NewLimiter(rl.limit, rl.burst)}
		rl.clients[key] = cl
	}
	cl.lastSeen = time.Now()
	return cl.limiter
}

func (rl *RateLimiter) cleanup() {
	for range time.Tick(time.Minute) {
		rl.mu.Lock()
		for k, cl := range rl.clients {
			if time.Since(cl.lastSeen) > 3*time.Minute {
				delete(rl.clients, k)
			}
		}
		rl.mu.Unlock()
	}
}

func extractIP(r *http.Request) string {
	ip := r.RemoteAddr
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	return ip
}

// Middleware implements the http.Handler wrapper with X-RateLimit-* headers.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := extractIP(r)
		lim := rl.getLimiter(key)

		// Reserve checks capacity without consuming a token.
		r2 := lim.Reserve()
		if !r2.OK() || r2.Delay() > 0 {
			// Delay > 0 means we'd have to wait — cancel and reject.
			r2.Cancel()
			remaining := int(lim.Tokens())
			if remaining < 0 {
				remaining = 0
			}
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.burst))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			w.Header().Set("Retry-After", "1")
			writeJSON(w, http.StatusTooManyRequests, map[string]string{
				"error": "rate limit exceeded",
				"code":  "RATE_LIMITED",
			})
			return
		}

		remaining := int(lim.Tokens())
		if remaining < 0 {
			remaining = 0
		}
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.burst))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))

		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// BuildRouter returns a chi router with a per-IP rate limiter applied.
func BuildRouter(globalRL *RateLimiter) http.Handler {
	r := chi.NewRouter()
	r.Use(globalRL.Middleware)

	r.Get("/hello", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"message": "hello"})
	})

	// Tighter limit on a specific endpoint.
	loginRL := NewRateLimiter(rate.Every(time.Second), 3)
	r.With(loginRL.Middleware).Post("/auth/login", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"message": "logged in"})
	})

	return r
}
