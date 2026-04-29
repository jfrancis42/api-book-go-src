package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var apiBaseURL = "https://api.github.com"

func setBaseURL(u string) { apiBaseURL = u }

var linkRe = regexp.MustCompile(`<([^>]+)>;\s*rel="([^"]+)"`)

// ParseLinkHeader extracts the URL for each relation from a Link header.
func ParseLinkHeader(header string) map[string]string {
	result := make(map[string]string)
	for _, part := range strings.Split(header, ",") {
		matches := linkRe.FindStringSubmatch(strings.TrimSpace(part))
		if len(matches) == 3 {
			result[matches[2]] = matches[1]
		}
	}
	return result
}

type Client struct {
	token      string
	httpClient *http.Client
}

func NewClient(token string) *Client {
	return &Client{token: token, httpClient: &http.Client{}}
}

func (c *Client) do(ctx context.Context, method, path string, body []byte) ([]byte, *http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, path, bodyReader)
	if err != nil {
		return nil, nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

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
		return nil, resp, fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, respBody)
	}
	return respBody, resp, nil
}

func (c *Client) getURL(ctx context.Context, url string) ([]byte, *http.Response, error) {
	return c.do(ctx, "GET", url, nil)
}

func (c *Client) get(ctx context.Context, path string) ([]byte, *http.Response, error) {
	return c.getURL(ctx, apiBaseURL+path)
}

type Issue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
}

type Repo struct {
	FullName string `json:"full_name"`
	Language string `json:"language"`
}

// Pages returns an iterator over pages of issues.
func (c *Client) Pages(ctx context.Context, firstURL string) iter.Seq2[[]Issue, error] {
	return func(yield func([]Issue, error) bool) {
		url := firstURL
		for url != "" {
			body, resp, err := c.getURL(ctx, url)
			if err != nil {
				yield(nil, err)
				return
			}
			var page []Issue
			if err := json.Unmarshal(body, &page); err != nil {
				yield(nil, err)
				return
			}
			if !yield(page, nil) {
				return
			}
			links := ParseLinkHeader(resp.Header.Get("Link"))
			url = links["next"]
		}
	}
}

// CollectAll drains an iterator and returns all items.
func CollectAll[T any](seq iter.Seq2[[]T, error]) ([]T, error) {
	var all []T
	for page, err := range seq {
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
	}
	return all, nil
}

// First collects at most n items from the iterator, stopping early.
func First[T any](n int, seq iter.Seq2[[]T, error]) ([]T, error) {
	var all []T
	for page, err := range seq {
		if err != nil {
			return nil, err
		}
		for _, item := range page {
			all = append(all, item)
			if len(all) >= n {
				return all, nil
			}
		}
	}
	return all, nil
}

func (c *Client) ListAllIssues(ctx context.Context, owner, repo string) ([]Issue, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/issues?per_page=100&state=all", apiBaseURL, owner, repo)
	return CollectAll(c.Pages(ctx, url))
}
