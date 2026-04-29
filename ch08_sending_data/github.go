package github

import (
	"bytes"
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
		return nil, resp, fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, respBody)
	}
	return respBody, resp, nil
}

func (c *Client) get(ctx context.Context, path string) ([]byte, *http.Response, error) {
	return c.do(ctx, "GET", path, nil)
}
func (c *Client) post(ctx context.Context, path string, body []byte) ([]byte, *http.Response, error) {
	return c.do(ctx, "POST", path, body)
}
func (c *Client) patch(ctx context.Context, path string, body []byte) ([]byte, *http.Response, error) {
	return c.do(ctx, "PATCH", path, body)
}
func (c *Client) put(ctx context.Context, path string, body []byte) ([]byte, *http.Response, error) {
	return c.do(ctx, "PUT", path, body)
}
func (c *Client) delete(ctx context.Context, path string) ([]byte, *http.Response, error) {
	return c.do(ctx, "DELETE", path, nil)
}

type Issue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	State     string    `json:"state"`
	Body      string    `json:"body"`
	Locked    bool      `json:"locked"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateIssueRequest struct {
	Title  string   `json:"title"`
	Body   string   `json:"body,omitempty"`
	Labels []string `json:"labels,omitempty"`
}

func (c *Client) CreateIssue(ctx context.Context, owner, repo string, req CreateIssueRequest) (*Issue, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	body, _, err := c.post(ctx, fmt.Sprintf("/repos/%s/%s/issues", owner, repo), payload)
	if err != nil {
		return nil, err
	}
	var issue Issue
	return &issue, json.Unmarshal(body, &issue)
}

type UpdateIssueRequest struct {
	Title *string `json:"title,omitempty"`
	Body  *string `json:"body,omitempty"`
	State *string `json:"state,omitempty"`
}

func (c *Client) UpdateIssue(ctx context.Context, owner, repo string, number int, req UpdateIssueRequest) (*Issue, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	body, _, err := c.patch(ctx, fmt.Sprintf("/repos/%s/%s/issues/%d", owner, repo, number), payload)
	if err != nil {
		return nil, err
	}
	var issue Issue
	return &issue, json.Unmarshal(body, &issue)
}

func strPtr(s string) *string { return &s }

func (c *Client) CloseIssue(ctx context.Context, owner, repo string, number int) (*Issue, error) {
	return c.UpdateIssue(ctx, owner, repo, number, UpdateIssueRequest{State: strPtr("closed")})
}

func (c *Client) StarRepo(ctx context.Context, owner, repo string) error {
	_, _, err := c.put(ctx, fmt.Sprintf("/user/starred/%s/%s", owner, repo), nil)
	return err
}

func (c *Client) UnstarRepo(ctx context.Context, owner, repo string) error {
	_, _, err := c.delete(ctx, fmt.Sprintf("/user/starred/%s/%s", owner, repo))
	return err
}

type IssueComment struct {
	ID   int    `json:"id"`
	Body string `json:"body"`
}

func (c *Client) CreateIssueComment(ctx context.Context, owner, repo string, number int, body string) (*IssueComment, error) {
	payload, err := json.Marshal(map[string]string{"body": body})
	if err != nil {
		return nil, err
	}
	respBody, _, err := c.post(ctx, fmt.Sprintf("/repos/%s/%s/issues/%d/comments", owner, repo, number), payload)
	if err != nil {
		return nil, err
	}
	var comment IssueComment
	return &comment, json.Unmarshal(respBody, &comment)
}

func (c *Client) ListIssues(ctx context.Context, owner, repo string, state string, perPage int) ([]Issue, error) {
	params := url.Values{}
	if state != "" {
		params.Set("state", state)
	}
	if perPage > 0 {
		params.Set("per_page", strconv.Itoa(perPage))
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
