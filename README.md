# Source Code — *Go Fetch*
### Crafting REST API Clients and Servers in Go

Source code for every chapter of *Go Fetch* by George Jeffrey Francis
(Ordo Artificum Press, 2026).

**Available on Amazon:**
https://www.amazon.com/gp/product/B0GZF89415/

---

## Organization

Each directory corresponds to a chapter and is **self-contained**.

### Part I — Building REST API Clients in Go (ch04–ch22)

| Directory | Chapter |
|-----------|---------|
| `ch04_first_call/` | Chapter 4: Your First API Call |
| `ch05_json/` | Chapter 5: JSON and Struct Tags |
| `ch06_credentials/` | Chapter 6: Keeping Secrets |
| `ch07_parameters/` | Chapter 7: Query and Path Parameters |
| `ch08_sending_data/` | Chapter 8: Sending Data |
| `ch09_errors/` | Chapter 9: Error Handling |
| `ch10_rate_limits/` | Chapter 10: Rate Limits |
| `ch11_retrying/` | Chapter 11: Retrying with Exponential Backoff |
| `ch12_pagination/` | Chapter 12: Pagination |
| `ch13_http_caching/` | Chapter 13: HTTP Caching — Conditional Requests |
| `ch14_client_config/` | Chapter 14: Client Configuration and Middleware |
| `ch15_goroutines/` | Chapter 15: Goroutines and errgroup |
| `ch18_xml/` | Chapter 18: XML — The Other Data Format |
| `ch19_auth/` | Chapter 19: Authentication Deep Dive |
| `ch21_webhooks/` | Chapter 21: Webhooks |

### Part II — Building REST APIs in Go (ch23–ch44)

| Directory | Chapter |
|-----------|---------|
| `ch23_server_intro/` | Chapter 23: Introduction to chi |
| `ch24_middleware/` | Chapter 24: Middleware |
| `ch25_database/` | Chapter 25: Databases with sqlx |
| `ch26_request_validation/` | Chapter 26: Request Validation |
| `ch27_response_design/` | Chapter 27: Response Design |
| `ch28_routing/` | Chapter 28: Advanced Routing |
| `ch29_post_put_patch/` | Chapter 29: POST, PUT, and PATCH |
| `ch30_server_pagination/` | Chapter 30: Pagination in the Server |
| `ch31_server_errors/` | Chapter 31: Structured Error Responses |
| `ch32_server_auth/` | Chapter 32: Authentication |
| `ch33_rate_limiting/` | Chapter 33: Rate Limiting |
| `ch34_versioning/` | Chapter 34: API Versioning |
| `ch35_server_webhooks/` | Chapter 35: Sending Webhooks |
| `ch36_openapi/` | Chapter 36: OpenAPI |
| `ch37_server_caching/` | Chapter 37: Server-Side Caching |
| `ch38_graceful_shutdown/` | Chapter 38: Graceful Shutdown |
| `ch39_server_testing/` | Chapter 39: Testing HTTP Servers |
| `ch40_observability/` | Chapter 40: Observability |
| `ch41_concurrency_patterns/` | Chapter 41: Concurrency Patterns |
| `ch42_performance/` | Chapter 42: Performance |
| `ch43_production/` | Chapter 43: Production Readiness |
| `ch44_complete_server/` | Chapter 44: The Complete Server |
| `server/` | Full server — final assembled state |

---

## Getting Started

**Requirements:** Go 1.22+

Chapters 4–22 (Part I) call the GitHub API and require a personal access token.
Create a `.env` file in each chapter directory you want to run:

```
GITHUB_TOKEN=your_token_here
```

Run any example:

```bash
cd ch04_first_call/
go run .
```

Part II chapters run a local HTTP server. Start the server, then use `curl` or
any HTTP client to exercise the endpoints — each chapter's directory includes
example `curl` commands in its own `README.md`.
