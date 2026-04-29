package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

var apiBaseURL = "https://api.github.com"

func setBaseURL(u string) { apiBaseURL = u }

// RateLimit holds the most recently observed rate limit state.
type RateLimit struct {
	Limit     int
	Remaining int
	Reset     time.Time
	Used      int
}

func parseRateLimit(h http.Header) RateLimit {
	parseInt := func(key string) int {
		n, _ := strconv.Atoi(h.Get(key))
		return n
	}
	resetUnix, _ := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64)
	return RateLimit{
		Limit:     parseInt("X-RateLimit-Limit"),
		Remaining: parseInt("X-RateLimit-Remaining"),
		Reset:     time.Unix(resetUnix, 0),
		Used:      parseInt("X-RateLimit-Used"),
	}
}

type Client struct {
	token      string
	httpClient *http.Client
	mu         sync.Mutex
	rateLimit  RateLimit
}

func NewClient(token string) *Client {
	return &Client{token: token, httpClient: &http.Client{}}
}

func (c *Client) updateRateLimit(h http.Header) {
	rl := parseRateLimit(h)
	if rl.Limit == 0 {
		return
	}
	c.mu.Lock()
	c.rateLimit = rl
	c.mu.Unlock()
}

// RateLimit returns the most recently observed rate limit.
func (c *Client) RateLimit() RateLimit {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rateLimit
}

func (c *Client) checkRateLimit(ctx context.Context) error {
	c.mu.Lock()
	rl := c.rateLimit
	c.mu.Unlock()

	if rl.Limit == 0 || rl.Remaining > 0 {
		return nil
	}
	waitUntil := time.Until(rl.Reset)
	if waitUntil <= 0 {
		return nil
	}
	select {
	case <-time.After(waitUntil):
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func sleepForRateLimit(ctx context.Context, resp *http.Response) {
	if s := resp.Header.Get("Retry-After"); s != "" {
		if secs, err := strconv.Atoi(s); err == nil {
			select {
			case <-time.After(time.Duration(secs) * time.Second):
			case <-ctx.Done():
			}
			return
		}
	}
	rl := parseRateLimit(resp.Header)
	if wait := time.Until(rl.Reset); wait > 0 {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
		}
	}
}

var (
	ErrNotFound    = fmt.Errorf("not found")
	ErrUnauthorized = fmt.Errorf("unauthorized")
	ErrRateLimited = fmt.Errorf("rate limited")
	ErrServerError = fmt.Errorf("server error")
)

type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("GitHub API error %d: %s", e.StatusCode, e.Message)
}

func parseAPIError(statusCode int, body []byte) error {
	apiErr := &APIError{StatusCode: statusCode}
	var raw struct{ Message string `json:"message"` }
	if json.Unmarshal(body, &raw) == nil {
		apiErr.Message = raw.Message
	}
	switch statusCode {
	case http.StatusNotFound:
		return fmt.Errorf("%w: %w", ErrNotFound, apiErr)
	case http.StatusUnauthorized:
		return fmt.Errorf("%w: %w", ErrUnauthorized, apiErr)
	case http.StatusTooManyRequests:
		return fmt.Errorf("%w: %w", ErrRateLimited, apiErr)
	default:
		if statusCode >= 500 {
			return fmt.Errorf("%w: %w", ErrServerError, apiErr)
		}
		return apiErr
	}
}

func (c *Client) do(ctx context.Context, method, path string, body []byte) ([]byte, *http.Response, error) {
	if err := c.checkRateLimit(ctx); err != nil {
		return nil, nil, err
	}

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

	c.updateRateLimit(resp.Header)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp, err
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		sleepForRateLimit(ctx, resp)
		return nil, resp, parseAPIError(resp.StatusCode, respBody)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp, parseAPIError(resp.StatusCode, respBody)
	}
	return respBody, resp, nil
}

func (c *Client) get(ctx context.Context, path string) ([]byte, *http.Response, error) {
	return c.do(ctx, "GET", path, nil)
}

type User struct {
	Login string `json:"login"`
	Name  string `json:"name"`
}

func (c *Client) GetUser(ctx context.Context, username string) (*User, error) {
	body, _, err := c.get(ctx, "/users/"+username)
	if err != nil {
		return nil, err
	}
	var u User
	return &u, json.Unmarshal(body, &u)
}
