package openapi

import (
	_ "embed"
	"net/http"

	"github.com/go-chi/chi/v5"
)

//go:embed openapi.yaml
var spec []byte

// swaggerUI serves a minimal Swagger UI page pointing at /openapi.yaml.
const swaggerUI = `<!DOCTYPE html>
<html>
<head>
  <title>Todo API Docs</title>
  <meta charset="utf-8"/>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
<div id="swagger-ui"></div>
<script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
<script>
  SwaggerUIBundle({ url: "/openapi.yaml", dom_id: '#swagger-ui' })
</script>
</body>
</html>`

// BuildRouter returns a chi router that serves the OpenAPI spec and Swagger UI.
func BuildRouter() http.Handler {
	r := chi.NewRouter()

	// Serve the raw OpenAPI spec.
	r.Get("/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		w.WriteHeader(http.StatusOK)
		w.Write(spec)
	})

	// Serve the Swagger UI.
	r.Get("/docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(swaggerUI))
	})

	return r
}
