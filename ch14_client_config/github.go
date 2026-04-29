package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"
)

var apiBaseURL = "https://api.github.com"

func setBaseURL(u string) { apiBaseURL = u }

const (
	DefaultTimeout       = 30 * time.Second
	DefaultDialTimeout   = 5 * time.Second
	DefaultTLSTimeout    = 10 * time.Second
	DefaultHeaderTimeout = 15 * time.Second
)

// loggingTransport wraps an http.RoundTripper and logs each request.
type loggingTransport struct {
	base   http.RoundTripper
	logger *slog.Logger
}

func (t *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := t.base.RoundTrip(req)
	status := 0
	if resp != nil {
		status = resp.StatusCode
	}
	t.logger.Info("http request",
		"method", req.Method,
		"url", req.URL.String(),
		"status", status,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return resp, err
}

func newHTTPClient(timeout time.Duration, maxConns int, logger *slog.Logger) *http.Client {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   DefaultDialTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   DefaultTLSTimeout,
		ResponseHeaderTimeout: DefaultHeaderTimeout,
		MaxIdleConnsPerHost:   20,
		MaxConnsPerHost:       maxConns,
		IdleConnTimeout:       90 * time.Second,
		Proxy:                 http.ProxyFromEnvironment,
	}

	var rt http.RoundTripper = transport
	if logger != nil {
		rt = &loggingTransport{base: transport, logger: logger}
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: rt,
	}
}

type Option func(*Client)

func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		c.httpClient.Timeout = d
	}
}

func WithMaxConnsPerHost(n int) Option {
	return func(c *Client) {
		if t, ok := unwrapTransport(c.httpClient.Transport); ok {
			t.MaxConnsPerHost = n
		}
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(c *Client) {
		c.logger = logger
		// Rebuild the transport stack with the logger
		if t, ok := unwrapTransport(c.httpClient.Transport); ok {
			c.httpClient.Transport = &loggingTransport{base: t, logger: logger}
		}
	}
}

func unwrapTransport(rt http.RoundTripper) (*http.Transport, bool) {
	if lt, ok := rt.(*loggingTransport); ok {
		rt = lt.base
	}
	t, ok := rt.(*http.Transport)
	return t, ok
}

type Client struct {
	token      string
	httpClient *http.Client
	logger     *slog.Logger
}

func NewClient(token string, opts ...Option) *Client {
	c := &Client{
		token:      token,
		httpClient: newHTTPClient(DefaultTimeout, 0, nil),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) do(ctx context.Context, method, path string, body []byte) ([]byte, *http.Response, error) {
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
		var raw struct{ Message string `json:"message"` }
		json.Unmarshal(respBody, &raw)
		return nil, resp, fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, raw.Message)
	}
	return respBody, resp, nil
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
