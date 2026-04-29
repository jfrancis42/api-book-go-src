package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func sign(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func post(t *testing.T, handler http.Handler, event string, secret, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", event)
	req.Header.Set("X-Hub-Signature-256", sign(secret, body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func postBadSig(t *testing.T, handler http.Handler, event string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", event)
	req.Header.Set("X-Hub-Signature-256", "sha256=deadbeef")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func TestVerifySignature_Valid(t *testing.T) {
	secret := []byte("my-secret")
	payload := []byte(`{"ref":"refs/heads/main"}`)
	sig := sign(secret, payload)

	if err := verifySignature(secret, payload, sig); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifySignature_Invalid(t *testing.T) {
	secret := []byte("my-secret")
	payload := []byte(`{"ref":"refs/heads/main"}`)

	if err := verifySignature(secret, payload, "sha256=deadbeef00"); err == nil {
		t.Fatal("expected error for bad signature")
	}
}

func TestVerifySignature_MissingPrefix(t *testing.T) {
	secret := []byte("my-secret")
	payload := []byte(`{}`)

	if err := verifySignature(secret, payload, "nodprefix"); err == nil {
		t.Fatal("expected error for missing prefix")
	}
}

func TestWebhookHandler_Push(t *testing.T) {
	secret := []byte("webhook-secret")

	var received PushEvent
	h := &WebhookHandler{
		Secret: secret,
		OnPush: func(ev PushEvent) error {
			received = ev
			return nil
		},
	}

	payload, _ := json.Marshal(PushEvent{
		Ref:   "refs/heads/main",
		After: "abc123",
	})

	rr := post(t, h, "push", secret, payload)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("got %d, want %d", rr.Code, http.StatusNoContent)
	}
	if received.Ref != "refs/heads/main" {
		t.Fatalf("Ref: got %q, want %q", received.Ref, "refs/heads/main")
	}
}

func TestWebhookHandler_PullRequest(t *testing.T) {
	secret := []byte("webhook-secret")

	var received PullRequestEvent
	h := &WebhookHandler{
		Secret: secret,
		OnPR: func(ev PullRequestEvent) error {
			received = ev
			return nil
		},
	}

	ev := PullRequestEvent{Action: "opened", Number: 42}
	payload, _ := json.Marshal(ev)

	rr := post(t, h, "pull_request", secret, payload)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("got %d, want %d", rr.Code, http.StatusNoContent)
	}
	if received.Number != 42 {
		t.Fatalf("Number: got %d, want 42", received.Number)
	}
}

func TestWebhookHandler_InvalidSignature(t *testing.T) {
	h := &WebhookHandler{Secret: []byte("secret")}
	rr := postBadSig(t, h, "push", []byte(`{}`))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestWebhookHandler_Ping(t *testing.T) {
	secret := []byte("s")
	h := &WebhookHandler{Secret: secret}
	rr := post(t, h, "ping", secret, []byte(`{"zen":"Design for failure."}`))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("got %d, want %d", rr.Code, http.StatusNoContent)
	}
}

func TestWebhookHandler_UnknownEvent(t *testing.T) {
	secret := []byte("s")
	h := &WebhookHandler{Secret: secret}
	rr := post(t, h, "star", secret, []byte(`{"action":"created"}`))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("got %d, want %d", rr.Code, http.StatusNoContent)
	}
}
