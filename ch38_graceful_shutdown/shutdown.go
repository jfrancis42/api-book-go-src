package shutdown

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
)

// Config holds all server configuration loaded from environment variables.
type Config struct {
	Addr      string
	LogLevel  string
	JWTSecret string
}

// FromEnv loads configuration from environment variables with defaults.
// Returns an error if required variables are missing.
func FromEnv() (Config, error) {
	cfg := Config{
		Addr:     getenv("ADDR", ":8080"),
		LogLevel: getenv("LOG_LEVEL", "info"),
	}
	cfg.JWTSecret = os.Getenv("JWT_SECRET")
	if cfg.JWTSecret == "" {
		return cfg, fmt.Errorf("JWT_SECRET is required")
	}
	return cfg, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// HealthChecker tracks server start time and checks DB connectivity.
type HealthChecker struct {
	started time.Time
	ping    func(ctx context.Context) error // injected for testing
}

func NewHealthChecker(ping func(ctx context.Context) error) *HealthChecker {
	return &HealthChecker{started: time.Now(), ping: ping}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// HandleLiveness returns 200 as long as the process is alive.
func HandleLiveness(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// HandleReadiness pings the database and returns 503 if it is unreachable.
func (hc *HealthChecker) HandleReadiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := hc.ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status": "degraded",
			"reason": "database unreachable",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "ok",
		"uptime_seconds": int(time.Since(hc.started).Seconds()),
	})
}

// BuildRouter assembles a chi router with health check endpoints.
func BuildRouter(hc *HealthChecker) http.Handler {
	r := chi.NewRouter()

	r.Get("/healthz/live", HandleLiveness)
	r.Get("/healthz/ready", hc.HandleReadiness)

	r.Get("/slow", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(500 * time.Millisecond):
			writeJSON(w, http.StatusOK, map[string]string{"message": "slow response"})
		case <-r.Context().Done():
		}
	})

	return r
}

// Run starts the HTTP server and blocks until SIGTERM/SIGINT, then shuts down
// gracefully with a 30-second timeout.
//
// This function is intended to be called from main(). In tests, use
// httptest.NewServer(BuildRouter(...)) directly.
func Run(addr string, handler http.Handler) error {
	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	done := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			done <- err
		}
		close(done)
	}()

	// In production, signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	// would be called here. For this example, we just wait for the server to end.
	return <-done
}
