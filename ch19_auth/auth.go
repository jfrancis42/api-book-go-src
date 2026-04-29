package auth

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// TokenSource provides bearer tokens on demand.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// StaticTokenSource returns the same token every time — suitable for PATs.
type StaticTokenSource struct {
	token string
}

func NewStaticTokenSource(token string) *StaticTokenSource {
	return &StaticTokenSource{token: token}
}

func (s *StaticTokenSource) Token(_ context.Context) (string, error) {
	if s.token == "" {
		return "", fmt.Errorf("no token configured")
	}
	return s.token, nil
}

// FetchFunc fetches a new token and returns it along with its expiry time.
type FetchFunc func(ctx context.Context) (token string, expiry time.Time, err error)

// CachedTokenSource wraps a FetchFunc and caches the result until near-expiry.
type CachedTokenSource struct {
	mu      sync.Mutex
	fetch   FetchFunc
	token   string
	expires time.Time
	skew    time.Duration // refresh this early before expiry
}

func NewCachedTokenSource(fetch FetchFunc) *CachedTokenSource {
	return &CachedTokenSource{
		fetch: fetch,
		skew:  30 * time.Second,
	}
}

func (c *CachedTokenSource) Token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Now().Add(c.skew).Before(c.expires) {
		return c.token, nil
	}

	token, expires, err := c.fetch(ctx)
	if err != nil {
		return "", err
	}
	c.token = token
	c.expires = expires
	return token, nil
}

// authTransport is an http.RoundTripper that injects a Bearer token.
type authTransport struct {
	base   http.RoundTripper
	tokens TokenSource
}

// NewAuthTransport wraps an existing transport (or http.DefaultTransport) with
// token injection.
func NewAuthTransport(base http.RoundTripper, tokens TokenSource) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &authTransport{base: base, tokens: tokens}
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	token, err := t.tokens.Token(req.Context())
	if err != nil {
		return nil, fmt.Errorf("auth: get token: %w", err)
	}

	// Clone the request so we don't mutate the caller's copy.
	r2 := req.Clone(req.Context())
	r2.Header.Set("Authorization", "Bearer "+token)
	return t.base.RoundTrip(r2)
}

// NewAuthClient builds an *http.Client that injects Bearer tokens automatically.
func NewAuthClient(tokens TokenSource) *http.Client {
	return &http.Client{
		Transport: NewAuthTransport(http.DefaultTransport, tokens),
	}
}
