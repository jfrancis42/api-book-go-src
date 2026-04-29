package github

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"time"
)

var apiBaseURL = "https://api.github.com"
var feedBaseURL = "https://github.com"

func setBaseURL(u string) {
	apiBaseURL = u
	feedBaseURL = u
}

type Client struct {
	token      string
	httpClient *http.Client
}

func NewClient(token string) *Client {
	return &Client{token: token, httpClient: &http.Client{}}
}

type ReleaseFeed struct {
	XMLName xml.Name       `xml:"feed"`
	Title   string         `xml:"title"`
	Entries []ReleaseEntry `xml:"entry"`
}

type ReleaseEntry struct {
	ID      string        `xml:"id"`
	Title   string        `xml:"title"`
	Updated time.Time     `xml:"updated"`
	Links   []ReleaseLink `xml:"link"`
}

type ReleaseLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

func (e *ReleaseEntry) URL() string {
	for _, l := range e.Links {
		if l.Rel == "alternate" {
			return l.Href
		}
	}
	return ""
}

func (c *Client) GetReleases(ctx context.Context, owner, repo string) (*ReleaseFeed, error) {
	url := feedBaseURL + fmt.Sprintf("/%s/%s/releases.atom", owner, repo)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/atom+xml")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GetReleases: status %d", resp.StatusCode)
	}

	var feed ReleaseFeed
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, fmt.Errorf("GetReleases: decode: %w", err)
	}
	return &feed, nil
}
