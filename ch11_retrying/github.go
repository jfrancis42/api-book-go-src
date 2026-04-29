package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

var apiBaseURL = "https://api.github.com"

func setBaseURL(u string) { apiBaseURL = u }

var (
	ErrNotFound    = errors.New("not found")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden   = errors.New("forbidden")
	ErrRateLimited = errors.New("rate limited")
	ErrServerError = errors.New("server error")
)

type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("GitHub API error %d: %s", e.StatusCode, e.Message)
}

func IsRetryable(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.StatusCode {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}
	return false
}

func parseAPIError(code int, body []byte) error {
	apiErr := &APIError{StatusCode: code}
	var raw struct{ Message string `json:"message"` }
	if json.Unmarshal(body, &raw) == nil {
		apiErr.Message = raw.Message
	}
	switch code {
	case http.StatusNotFound:
		return fmt.Errorf("%w: %w", ErrNotFound, apiErr)
	case http.StatusUnauthorized:
		return fmt.Errorf("%w: %w", ErrUnauthorized, apiErr)
	case http.StatusForbidden:
		return fmt.Errorf("%w: %w", ErrForbidden, apiErr)
	case http.StatusTooManyRequests:
		return fmt.Errorf("%w: %w", ErrRateLimited, apiErr)
	default:
		if code >= 500 {
			return fmt.Errorf("%w: %w", ErrServerError, apiErr)
		}
		return apiErr
	}
}

// BackoffFunc computes the wait duration for the given retry attempt (0-based).
var BackoffFunc = defaultBackoff

func defaultBackoff(attempt int) time.Duration {
	base := time.Second
	max := 60 * time.Second
	exp := time.Duration(math.Pow(2, float64(attempt))) * base
	if exp > max {
		exp = max
	}
	jitter := time.Duration(rand.Int63n(int64(exp / 5)))
	return exp + jitter
}

type Option func(*Client)

func WithMaxRetries(n int) Option {
	return func(c *Client) { c.maxRetries = n }
}

type Client struct {
	token      string
	httpClient *http.Client
	maxRetries int
	mu         sync.Mutex
}

func NewClient(token string, opts ...Option) *Client {
	c := &Client{
		token:      token,
		httpClient: &http.Client{},
		maxRetries: 3,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) doOnce(ctx context.Context, method, path string, body []byte) ([]byte, *http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, apiBaseURL+path, bodyReader)
	if err != nil {
		return nil, nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp, parseAPIError(resp.StatusCode, respBody)
	}
	return respBody, resp, nil
}

func (c *Client) do(ctx context.Context, method, path string, body []byte) ([]byte, *http.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			wait := BackoffFunc(attempt - 1)
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			}
		}

		respBody, resp, err := c.doOnce(ctx, method, path, body)
		if err == nil {
			return respBody, resp, nil
		}
		lastErr = err
		if !IsRetryable(err) {
			return nil, resp, err
		}
	}
	return nil, nil, fmt.Errorf("after %d retries: %w", c.maxRetries, lastErr)
}

func (c *Client) get(ctx context.Context, path string) ([]byte, *http.Response, error) {
	return c.do(ctx, "GET", path, nil)
}

type User struct {
	Login string `json:"login"`
}

func (c *Client) GetUser(ctx context.Context, username string) (*User, error) {
	body, _, err := c.get(ctx, "/users/"+username)
	if err != nil {
		return nil, err
	}
	var u User
	return &u, json.Unmarshal(body, &u)
}
