package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
		return nil, resp, parseAPIError(resp.StatusCode, respBody)
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

type User struct {
	Login     string    `json:"login"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

func (c *Client) GetUser(ctx context.Context, username string) (*User, error) {
	body, _, err := c.get(ctx, "/users/"+username)
	if err != nil {
		return nil, fmt.Errorf("GetUser(%q): %w", username, err)
	}
	var u User
	return &u, json.Unmarshal(body, &u)
}

type Issue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
}

type CreateIssueRequest struct {
	Title string `json:"title"`
	Body  string `json:"body,omitempty"`
}

func (c *Client) CreateIssue(ctx context.Context, owner, repo string, req CreateIssueRequest) (*Issue, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	body, _, err := c.post(ctx, fmt.Sprintf("/repos/%s/%s/issues", owner, repo), payload)
	if err != nil {
		return nil, fmt.Errorf("CreateIssue: %w", err)
	}
	var issue Issue
	return &issue, json.Unmarshal(body, &issue)
}
