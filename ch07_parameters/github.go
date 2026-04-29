package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

var apiBaseURL = "https://api.github.com"

func setBaseURL(u string) { apiBaseURL = u }

type Client struct {
	token      string
	httpClient *http.Client
}

func NewClient(token string) *Client {
	return &Client{token: token, httpClient: &http.Client{}}
}

func (c *Client) get(ctx context.Context, path string) ([]byte, *http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", apiBaseURL+path, nil)
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp, fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, body)
	}
	return body, resp, nil
}

type User struct {
	Login     string    `json:"login"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type Issue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	State     string    `json:"state"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type IssueOptions struct {
	State   string
	Sort    string
	PerPage int
	Page    int
}

func (c *Client) ListIssues(ctx context.Context, owner, repo string, opts IssueOptions) ([]Issue, error) {
	params := url.Values{}
	if opts.State != "" {
		params.Set("state", opts.State)
	}
	if opts.Sort != "" {
		params.Set("sort", opts.Sort)
	}
	if opts.PerPage > 0 {
		params.Set("per_page", strconv.Itoa(opts.PerPage))
	}
	if opts.Page > 0 {
		params.Set("page", strconv.Itoa(opts.Page))
	}

	path := fmt.Sprintf("/repos/%s/%s/issues", owner, repo)
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	body, _, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}
	var issues []Issue
	return issues, json.Unmarshal(body, &issues)
}

type SearchResult struct {
	TotalCount        int    `json:"total_count"`
	IncompleteResults bool   `json:"incomplete_results"`
	Items             []Repo `json:"items"`
}

type Repo struct {
	FullName        string `json:"full_name"`
	Description     string `json:"description"`
	StargazersCount int    `json:"stargazers_count"`
	Language        string `json:"language"`
}

type SearchOptions struct {
	Sort    string
	Order   string
	PerPage int
}

func (c *Client) SearchRepos(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error) {
	params := url.Values{}
	params.Set("q", query)
	if opts.Sort != "" {
		params.Set("sort", opts.Sort)
	}
	if opts.Order != "" {
		params.Set("order", opts.Order)
	}
	if opts.PerPage > 0 {
		params.Set("per_page", strconv.Itoa(opts.PerPage))
	}
	body, _, err := c.get(ctx, "/search/repositories?"+params.Encode())
	if err != nil {
		return nil, err
	}
	var result SearchResult
	return &result, json.Unmarshal(body, &result)
}
