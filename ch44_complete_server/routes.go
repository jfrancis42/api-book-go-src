package complete

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jmoiron/sqlx"
	"golang.org/x/time/rate"
)

// BuildRouter wires together all handlers and middleware and returns an http.Handler
// ready to be passed to http.Server.
func BuildRouter(db *sqlx.DB, jwtSecret []byte) http.Handler {
	todos := NewTodoRepo(db)
	users := NewUserRepo(db)

	r := chi.NewRouter()

	// Global middleware — order matters.
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(SecurityHeaders)
	r.Use(Recoverer)
	r.Use(Logger)
	r.Use(RateLimiter(rate.Limit(100), 50))

	// Health probes — no auth required.
	r.Get("/healthz/live", handleLiveness())
	r.Get("/healthz/ready", handleReadiness(db))

	// Authentication — no JWT required, but apply a tighter rate limit.
	loginRL := RateLimiter(rate.Every(1), 5)
	r.With(loginRL).Post("/api/v1/auth/register", handleRegister(users, jwtSecret))
	r.With(loginRL).Post("/api/v1/auth/login", handleLogin(users, jwtSecret))

	// Protected routes.
	auth := RequireAuth(jwtSecret)
	r.Group(func(r chi.Router) {
		r.Use(auth)

		r.Route("/api/v1/todos", func(r chi.Router) {
			r.Get("/", handleListTodos(todos))
			r.Post("/", handleCreateTodo(todos))
			r.Get("/{id}", handleGetTodo(todos))
			r.Patch("/{id}", handleUpdateTodo(todos))
			r.Delete("/{id}", handleDeleteTodo(todos))
		})
	})

	// Standard not-found / method-not-allowed responses.
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not found", Code: "NOT_FOUND"})
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed", Code: "METHOD_NOT_ALLOWED"})
	})

	return r
}
