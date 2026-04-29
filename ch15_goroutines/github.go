package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/sync/errgroup"
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
	Login     string `json:"login"`
	Name      string `json:"name"`
	Followers int    `json:"followers"`
}

func (c *Client) GetUser(ctx context.Context, username string) (*User, error) {
	body, _, err := c.get(ctx, "/users/"+username)
	if err != nil {
		return nil, err
	}
	var u User
	return &u, json.Unmarshal(body, &u)
}

// FetchUsers fetches multiple users concurrently, bounded to maxConcurrent.
// Results are returned in the same order as the input usernames.
func (c *Client) FetchUsers(ctx context.Context, usernames []string, maxConcurrent int) ([]*User, error) {
	users := make([]*User, len(usernames))

	g, ctx := errgroup.WithContext(ctx)
	if maxConcurrent > 0 {
		g.SetLimit(maxConcurrent)
	}

	for i, username := range usernames {
		i, username := i, username
		g.Go(func() error {
			user, err := c.GetUser(ctx, username)
			if err != nil {
				return fmt.Errorf("GetUser(%q): %w", username, err)
			}
			users[i] = user
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}
	return users, nil
}

type Repo struct {
	FullName string `json:"full_name"`
	Language string `json:"language"`
}

// FetchRepos fetches multiple repos concurrently.
func (c *Client) FetchRepos(ctx context.Context, pairs [][2]string, maxConcurrent int) ([]*Repo, error) {
	repos := make([]*Repo, len(pairs))

	g, ctx := errgroup.WithContext(ctx)
	if maxConcurrent > 0 {
		g.SetLimit(maxConcurrent)
	}

	for i, pair := range pairs {
		i, owner, repo := i, pair[0], pair[1]
		g.Go(func() error {
			body, _, err := c.get(ctx, "/repos/"+owner+"/"+repo)
			if err != nil {
				return err
			}
			var r Repo
			if err := json.Unmarshal(body, &r); err != nil {
				return err
			}
			repos[i] = &r
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}
	return repos, nil
}
