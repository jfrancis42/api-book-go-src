package github

import (
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

type License struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	SPDX string `json:"spdx_id"`
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
	UpdatedAt   time.Time `json:"updated_at"`
}

type Repo struct {
	ID              int       `json:"id"`
	FullName        string    `json:"full_name"`
	Description     string    `json:"description"`
	Private         bool      `json:"private"`
	Fork            bool      `json:"fork"`
	StargazersCount int       `json:"stargazers_count"`
	ForksCount      int       `json:"forks_count"`
	Language        string    `json:"language"`
	Topics          []string  `json:"topics"`
	DefaultBranch   string    `json:"default_branch"`
	Owner           *User     `json:"owner"`
	License         *License  `json:"license"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	PushedAt        time.Time `json:"pushed_at"`
}

func (c *Client) GetUser(ctx context.Context, username string) (*User, error) {
	body, _, err := c.get(ctx, "/users/"+username)
	if err != nil {
		return nil, err
	}
	var u User
	return &u, json.Unmarshal(body, &u)
}

func (c *Client) GetRepo(ctx context.Context, owner, repo string) (*Repo, error) {
	body, _, err := c.get(ctx, "/repos/"+owner+"/"+repo)
	if err != nil {
		return nil, err
	}
	var r Repo
	return &r, json.Unmarshal(body, &r)
}

func (c *Client) ListUserRepos(ctx context.Context, username string) ([]Repo, error) {
	body, _, err := c.get(ctx, "/users/"+username+"/repos?per_page=100")
	if err != nil {
		return nil, err
	}
	var repos []Repo
	return repos, json.Unmarshal(body, &repos)
}
