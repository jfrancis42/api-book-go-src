package middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// BuildRouter assembles a chi router with the chapter's middleware stack applied.
func BuildRouter() http.Handler {
	r := chi.NewRouter()

	// Outermost: Recoverer so it catches panics in all other middleware.
	r.Use(Recoverer)
	r.Use(RequestID)
	r.Use(Logger)
	r.Use(CORS([]string{"https://example.com"}))

	r.Get("/hello", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"message":    "hello",
			"request_id": GetRequestID(r.Context()),
		})
	})

	r.Get("/panic", func(w http.ResponseWriter, r *http.Request) {
		panic("intentional panic for testing")
	})

	return r
}
