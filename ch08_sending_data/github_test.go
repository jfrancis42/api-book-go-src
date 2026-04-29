package github

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func withServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(func() { srv.Close(); setBaseURL("https://api.github.com") })
	setBaseURL(srv.URL)
	return NewClient("test-token")
}

func TestCreateIssue(t *testing.T) {
	want := Issue{Number: 42, Title: "Test bug", State: "open"}
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method: got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type: got %s", r.Header.Get("Content-Type"))
		}
		var req CreateIssueRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Title != "Test bug" {
			t.Errorf("title: got %q", req.Title)
		}
		w.WriteHeader(http.StatusCreated)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(want)
	})
	got, err := c.CreateIssue(context.Background(), "owner", "repo", CreateIssueRequest{Title: "Test bug"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Number != 42 {
		t.Errorf("number: got %d, want 42", got.Number)
	}
}

func TestUpdateIssue_PartialBody(t *testing.T) {
	var reqBody map[string]interface{}
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			t.Errorf("method: got %s", r.Method)
		}
		json.NewDecoder(r.Body).Decode(&reqBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Issue{Number: 1, State: "closed"})
	})
	closed := "closed"
	c.UpdateIssue(context.Background(), "o", "r", 1, UpdateIssueRequest{State: &closed})

	if _, ok := reqBody["title"]; ok {
		t.Error("title should not be in body when not set")
	}
	if reqBody["state"] != "closed" {
		t.Errorf("state: got %v", reqBody["state"])
	}
}

func TestCloseIssue(t *testing.T) {
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["state"] != "closed" {
			t.Errorf("state: got %q", body["state"])
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Issue{Number: 5, State: "closed"})
	})
	_, err := c.CloseIssue(context.Background(), "o", "r", 5)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDeleteReturns204(t *testing.T) {
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("method: got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	err := c.UnstarRepo(context.Background(), "owner", "repo")
	if err != nil {
		t.Fatal(err)
	}
}

func TestCreateIssueComment(t *testing.T) {
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var m map[string]string
		json.Unmarshal(body, &m)
		if m["body"] != "hello" {
			t.Errorf("body: got %q", m["body"])
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(IssueComment{ID: 99, Body: "hello"})
	})
	comment, err := c.CreateIssueComment(context.Background(), "o", "r", 1, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if comment.ID != 99 {
		t.Errorf("comment ID: got %d", comment.ID)
	}
}
