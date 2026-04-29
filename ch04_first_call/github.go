package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

var apiBaseURL = "https://api.github.com"

// setBaseURL overrides the base URL; used only in tests.
func setBaseURL(u string) { apiBaseURL = u }

// Client is a GitHub API client.
type Client struct {
	token      string
	httpClient *http.Client
}

// NewClient returns a Client. Pass an empty string for unauthenticated access.
func NewClient(token string) *Client {
	return &Client{
		token:      token,
		httpClient: &http.Client{},
	}
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

// GetZen returns a random piece of GitHub's design philosophy.
func (c *Client) GetZen(ctx context.Context) (string, error) {
	body, _, err := c.get(ctx, "/zen")
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// User represents a GitHub user.
type User struct {
	Login     string `json:"login"`
	Name      string `json:"name"`
	Bio       string `json:"bio"`
	Followers int    `json:"followers"`
	Following int    `json:"following"`
}

// GetUser returns the GitHub user with the given username.
func (c *Client) GetUser(ctx context.Context, username string) (*User, error) {
	body, _, err := c.get(ctx, "/users/"+username)
	if err != nil {
		return nil, err
	}
	var user User
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// GetAuthenticatedUser returns the currently authenticated user.
func (c *Client) GetAuthenticatedUser(ctx context.Context) (*User, error) {
	body, _, err := c.get(ctx, "/user")
	if err != nil {
		return nil, err
	}
	var user User
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// Repo represents a GitHub repository.
type Repo struct {
	FullName        string `json:"full_name"`
	Description     string `json:"description"`
	StargazersCount int    `json:"stargazers_count"`
	ForksCount      int    `json:"forks_count"`
	Language        string `json:"language"`
}

// GetRepo returns the repository owner/repo.
func (c *Client) GetRepo(ctx context.Context, owner, repo string) (*Repo, error) {
	body, _, err := c.get(ctx, "/repos/"+owner+"/"+repo)
	if err != nil {
		return nil, err
	}
	var r Repo
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	return &r, nil
}
