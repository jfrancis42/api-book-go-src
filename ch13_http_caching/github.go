package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var apiBaseURL = "https://api.github.com"

func setBaseURL(u string) { apiBaseURL = u }

type cacheEntry struct {
	etag      string
	body      []byte
	expiresAt time.Time
}

// CacheStats holds cache performance counters.
type CacheStats struct {
	Entries      int
	Hits         int64
	Revalidations int64
	Misses       int64
}

type Client struct {
	token      string
	httpClient *http.Client

	mu     sync.RWMutex
	cache  map[string]cacheEntry

	hits         atomic.Int64
	revalidations atomic.Int64
	misses       atomic.Int64
}

func NewClient(token string) *Client {
	return &Client{
		token:      token,
		httpClient: &http.Client{},
		cache:      make(map[string]cacheEntry),
	}
}

func (c *Client) CacheStats() CacheStats {
	c.mu.RLock()
	entries := len(c.cache)
	c.mu.RUnlock()
	return CacheStats{
		Entries:      entries,
		Hits:         c.hits.Load(),
		Revalidations: c.revalidations.Load(),
		Misses:       c.misses.Load(),
	}
}

func (c *Client) ClearCache() {
	c.mu.Lock()
	c.cache = make(map[string]cacheEntry)
	c.mu.Unlock()
}

func parseMaxAge(cc string) int {
	for _, directive := range strings.Split(cc, ",") {
		directive = strings.TrimSpace(directive)
		if strings.HasPrefix(directive, "max-age=") {
			n, err := strconv.Atoi(strings.TrimPrefix(directive, "max-age="))
			if err == nil {
				return n
			}
		}
	}
	return 0
}

func parseAPIError(code int, body []byte) error {
	var raw struct{ Message string `json:"message"` }
	json.Unmarshal(body, &raw)
	return fmt.Errorf("GitHub API error %d: %s", code, raw.Message)
}

func (c *Client) get(ctx context.Context, path string) ([]byte, *http.Response, error) {
	url := apiBaseURL + path

	c.mu.RLock()
	entry, cached := c.cache[url]
	c.mu.RUnlock()

	if cached && time.Now().Before(entry.expiresAt) {
		c.hits.Add(1)
		return entry.body, nil, nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if cached && entry.etag != "" {
		req.Header.Set("If-None-Match", entry.etag)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		c.revalidations.Add(1)
		return entry.body, resp, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp, parseAPIError(resp.StatusCode, body)
	}

	c.misses.Add(1)
	newEntry := cacheEntry{
		etag: resp.Header.Get("ETag"),
		body: body,
	}
	if maxAge := parseMaxAge(resp.Header.Get("Cache-Control")); maxAge > 0 {
		newEntry.expiresAt = time.Now().Add(time.Duration(maxAge) * time.Second)
	}
	c.mu.Lock()
	c.cache[url] = newEntry
	c.mu.Unlock()

	return body, resp, nil
}

type Repo struct {
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Language    string `json:"language"`
}

func (c *Client) GetRepo(ctx context.Context, owner, repo string) (*Repo, error) {
	body, _, err := c.get(ctx, "/repos/"+owner+"/"+repo)
	if err != nil {
		return nil, err
	}
	var r Repo
	return &r, json.Unmarshal(body, &r)
}
