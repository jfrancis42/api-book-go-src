package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
)

var apiBaseURL = "https://api.github.com"

func setBaseURL(u string) { apiBaseURL = u }

type Client struct {
	token      string
	httpClient *http.Client
}

// NewClient returns a Client authenticated with the given token.
func NewClient(token string) *Client {
	return &Client{token: token, httpClient: &http.Client{}}
}

// NewClientFromEnv creates a Client from the GITHUB_TOKEN environment variable.
// Loads .env if present.
func NewClientFromEnv() (*Client, error) {
	_ = godotenv.Load()
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN environment variable not set")
	}
	return NewClient(token), nil
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
	Login       string    `json:"login"`
	Name        string    `json:"name"`
	Email       string    `json:"email"`
	Bio         string    `json:"bio"`
	PublicRepos int       `json:"public_repos"`
	Followers   int       `json:"followers"`
	Following   int       `json:"following"`
	CreatedAt   time.Time `json:"created_at"`
}

func (c *Client) GetUser(ctx context.Context, username string) (*User, error) {
	body, _, err := c.get(ctx, "/users/"+username)
	if err != nil {
		return nil, err
	}
	var u User
	return &u, json.Unmarshal(body, &u)
}

func (c *Client) GetAuthenticatedUser(ctx context.Context) (*User, error) {
	body, _, err := c.get(ctx, "/user")
	if err != nil {
		return nil, err
	}
	var u User
	return &u, json.Unmarshal(body, &u)
}
