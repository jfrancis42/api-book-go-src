package production

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
)

// Config holds all runtime configuration loaded from the environment.
// Every field that is required causes FromEnv to return an error when absent.
type Config struct {
	Addr      string
	JWTSecret string
	DBConn    string
	LogLevel  string
	TLSCert   string
	TLSKey    string
}

// FromEnv loads Config from environment variables with safe defaults.
func FromEnv() (Config, error) {
	cfg := Config{
		Addr:     getenv("ADDR", ":8080"),
		LogLevel: getenv("LOG_LEVEL", "info"),
		TLSCert:  os.Getenv("TLS_CERT"),
		TLSKey:   os.Getenv("TLS_KEY"),
	}
	if cfg.JWTSecret = os.Getenv("JWT_SECRET"); cfg.JWTSecret == "" {
		return cfg, fmt.Errorf("JWT_SECRET is required")
	}
	if cfg.DBConn = os.Getenv("DB_CONN"); cfg.DBConn == "" {
		return cfg, fmt.Errorf("DB_CONN is required")
	}
	if (cfg.TLSCert == "") != (cfg.TLSKey == "") {
		return cfg, fmt.Errorf("TLS_CERT and TLS_KEY must both be set or both be empty")
	}
	return cfg, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// SecurityHeaders adds OWASP-recommended HTTP security headers to every response.
// Call this middleware first so headers are present even on error responses.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		// Prevent MIME-type sniffing.
		h.Set("X-Content-Type-Options", "nosniff")
		// Block rendering in frames (clickjacking protection).
		h.Set("X-Frame-Options", "DENY")
		// Tell browsers to enforce HTTPS for one year.
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		// Restrict what browsers can load (CSP baseline).
		h.Set("Content-Security-Policy", "default-src 'self'")
		// Limit Referer header leakage.
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		// Disable access to sensitive browser features.
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		next.ServeHTTP(w, r)
	})
}

// ReadinessProbe tracks the server start time for uptime reporting.
type ReadinessProbe struct {
	started time.Time
	checks  []func() error
}

func NewReadinessProbe(checks ...func() error) *ReadinessProbe {
	return &ReadinessProbe{started: time.Now(), checks: checks}
}

func (p *ReadinessProbe) Handle(w http.ResponseWriter, r *http.Request) {
	for _, check := range p.checks {
		if err := check(); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"status": "degraded",
				"reason": err.Error(),
			})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "ok",
		"uptime_seconds": int(time.Since(p.started).Seconds()),
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// BuildRouter assembles a production-ready router with security middleware.
func BuildRouter(probe *ReadinessProbe) http.Handler {
	r := chi.NewRouter()
	r.Use(SecurityHeaders)

	r.Get("/healthz/live", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/healthz/ready", probe.Handle)

	r.Get("/todos", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, []any{})
	})

	return r
}
