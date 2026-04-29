package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// verifySignature checks that the X-Hub-Signature-256 header matches the
// HMAC-SHA256 of the payload computed with secret.
func verifySignature(secret, payload []byte, signature string) error {
	if !strings.HasPrefix(signature, "sha256=") {
		return fmt.Errorf("signature missing sha256= prefix")
	}
	got, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	if err != nil {
		return fmt.Errorf("invalid signature hex: %w", err)
	}

	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	want := mac.Sum(nil)

	if !hmac.Equal(got, want) {
		return fmt.Errorf("signature mismatch")
	}
	return nil
}

// PushEvent represents a GitHub push webhook payload (abbreviated).
type PushEvent struct {
	Ref        string `json:"ref"`
	Before     string `json:"before"`
	After      string `json:"after"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	Pusher struct {
		Name string `json:"name"`
	} `json:"pusher"`
}

// PullRequestEvent represents a GitHub pull_request webhook payload (abbreviated).
type PullRequestEvent struct {
	Action      string `json:"action"`
	Number      int    `json:"number"`
	PullRequest struct {
		Title string `json:"title"`
		State string `json:"state"`
		User  struct {
			Login string `json:"login"`
		} `json:"user"`
	} `json:"pull_request"`
}

// WebhookHandler verifies GitHub webhook signatures and dispatches events to
// registered handlers.
type WebhookHandler struct {
	Secret []byte

	// OnPush is called when a push event arrives. May be nil.
	OnPush func(PushEvent) error

	// OnPR is called when a pull_request event arrives. May be nil.
	OnPR func(PullRequestEvent) error
}

func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MB limit
	if err != nil {
		http.Error(w, "cannot read body", http.StatusBadRequest)
		return
	}

	sig := r.Header.Get("X-Hub-Signature-256")
	if err := verifySignature(h.Secret, body, sig); err != nil {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	event := r.Header.Get("X-GitHub-Event")
	switch event {
	case "push":
		if h.OnPush == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var ev PushEvent
		if err := json.Unmarshal(body, &ev); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if err := h.OnPush(ev); err != nil {
			http.Error(w, "handler error", http.StatusInternalServerError)
			return
		}

	case "pull_request":
		if h.OnPR == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var ev PullRequestEvent
		if err := json.Unmarshal(body, &ev); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if err := h.OnPR(ev); err != nil {
			http.Error(w, "handler error", http.StatusInternalServerError)
			return
		}

	case "ping":
		// GitHub sends a ping event when the webhook is first created.
		w.WriteHeader(http.StatusNoContent)
		return

	default:
		// Unknown event type — accept but ignore.
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
