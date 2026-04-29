package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newServer(t *testing.T) (*httptest.Server, *EventQueue, *SubscriptionStore) {
	t.Helper()
	queue := NewEventQueue()
	subs := NewSubscriptionStore()
	todos := NewTodoStore()
	srv := httptest.NewServer(BuildRouter(todos, queue, subs))
	t.Cleanup(srv.Close)
	return srv, queue, subs
}

// newSubscriberServer creates a test HTTP server that records received webhooks.
func newSubscriberServer(t *testing.T) (*httptest.Server, *atomic.Int64, chan []byte) {
	t.Helper()
	var count atomic.Int64
	received := make(chan []byte, 10)

	sub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body.Close()
		count.Add(1)
		received <- body
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(sub.Close)
	return sub, &count, received
}

func TestEnqueueEvent_OnTodoCreate(t *testing.T) {
	srv, queue, _ := newServer(t)

	resp, _ := srv.Client().Post(srv.URL+"/todos", "application/json",
		strings.NewReader(`{"text":"test todo"}`))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("got %d, want 201", resp.StatusCode)
	}

	pending := queue.Pending()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending event, got %d", len(pending))
	}
	if pending[0].EventType != "todo.created" {
		t.Fatalf("event type: got %q, want todo.created", pending[0].EventType)
	}
}

func TestWorker_DeliversToSubscriber(t *testing.T) {
	queue := NewEventQueue()
	subs := NewSubscriptionStore()
	sub, count, received := newSubscriberServer(t)

	subs.Create(sub.URL, "mysecret", []string{"todo.created"})

	// Enqueue an event and process it.
	queue.Enqueue("todo.created", map[string]string{"text": "hello"})
	worker := NewWebhookWorker(queue, subs)
	worker.ProcessOnce(t.Context())

	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for webhook delivery")
	}

	if count.Load() != 1 {
		t.Fatalf("delivery count: got %d, want 1", count.Load())
	}
	if len(queue.Pending()) != 0 {
		t.Fatal("expected no pending events after delivery")
	}
}

func TestWorker_PayloadMatchesTodo(t *testing.T) {
	queue := NewEventQueue()
	subs := NewSubscriptionStore()
	sub, _, received := newSubscriberServer(t)

	subs.Create(sub.URL, "secret", []string{"todo.created"})

	todo := map[string]any{"id": 1, "text": "buy milk"}
	queue.Enqueue("todo.created", todo)
	worker := NewWebhookWorker(queue, subs)
	worker.ProcessOnce(t.Context())

	select {
	case body := <-received:
		var got map[string]any
		json.Unmarshal(body, &got)
		if got["text"] != "buy milk" {
			t.Fatalf("payload text: got %v", got["text"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

func TestSign_HMACCorrect(t *testing.T) {
	secret := "test-secret"
	payload := []byte(`{"id":1,"text":"hello"}`)

	sig := sign(secret, payload)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if sig != want {
		t.Fatalf("signature mismatch: got %q, want %q", sig, want)
	}
}

func TestWorker_SignatureHeaderPresent(t *testing.T) {
	queue := NewEventQueue()
	subs := NewSubscriptionStore()

	var gotSig string
	subSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-Hub-Signature-256")
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(subSrv.Close)

	subs.Create(subSrv.URL, "my-webhook-secret", []string{"todo.created"})
	queue.Enqueue("todo.created", map[string]string{"event": "test"})

	worker := NewWebhookWorker(queue, subs)
	worker.ProcessOnce(t.Context())

	time.Sleep(10 * time.Millisecond)
	if !strings.HasPrefix(gotSig, "sha256=") {
		t.Fatalf("X-Hub-Signature-256: got %q, want sha256=...", gotSig)
	}
}

func TestWorker_EventTypeHeaderPresent(t *testing.T) {
	queue := NewEventQueue()
	subs := NewSubscriptionStore()

	var gotEventType string
	subSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotEventType = r.Header.Get("X-Webhook-Event")
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(subSrv.Close)

	subs.Create(subSrv.URL, "secret", []string{"todo.deleted"})
	queue.Enqueue("todo.deleted", map[string]int{"id": 42})

	worker := NewWebhookWorker(queue, subs)
	worker.ProcessOnce(t.Context())

	time.Sleep(10 * time.Millisecond)
	if gotEventType != "todo.deleted" {
		t.Fatalf("X-Webhook-Event: got %q, want todo.deleted", gotEventType)
	}
}

func TestWorker_SubscriberError_IncrementsAttempts(t *testing.T) {
	queue := NewEventQueue()
	subs := NewSubscriptionStore()

	// Subscriber that always fails.
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(failSrv.Close)

	subs.Create(failSrv.URL, "secret", []string{"todo.created"})
	event, _ := queue.Enqueue("todo.created", map[string]string{"text": "test"})

	worker := NewWebhookWorker(queue, subs)
	worker.ProcessOnce(t.Context())

	if event.Attempts != 1 {
		t.Fatalf("attempts: got %d, want 1", event.Attempts)
	}
	// Event should still be pending after a failed delivery.
	if event.Delivered {
		t.Fatal("event should not be marked delivered after error")
	}
}

func TestSubscriptionAPI_Create(t *testing.T) {
	srv, _, _ := newServer(t)

	resp, _ := srv.Client().Post(srv.URL+"/webhooks/subscriptions", "application/json",
		strings.NewReader(`{"url":"http://example.com/hook","secret":"s3cr3t","events":["todo.created"]}`))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("got %d, want 201", resp.StatusCode)
	}
	var sub Subscription
	json.NewDecoder(resp.Body).Decode(&sub)
	if sub.ID == 0 {
		t.Fatal("expected non-zero subscription ID")
	}
}

func TestSubscriptionAPI_InvalidURL(t *testing.T) {
	srv, _, _ := newServer(t)

	resp, _ := srv.Client().Post(srv.URL+"/webhooks/subscriptions", "application/json",
		strings.NewReader(`{"url":"not-a-url","secret":"s3cr3t","events":["todo.created"]}`))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", resp.StatusCode)
	}
}

func TestWorker_WildcardSubscription(t *testing.T) {
	queue := NewEventQueue()
	subs := NewSubscriptionStore()
	sub, count, received := newSubscriberServer(t)

	subs.Create(sub.URL, "secret", []string{"*"})
	queue.Enqueue("todo.created", map[string]string{"a": "1"})
	queue.Enqueue("todo.deleted", map[string]string{"b": "2"})

	worker := NewWebhookWorker(queue, subs)
	worker.ProcessOnce(t.Context())

	timeout := time.After(time.Second)
	for range 2 {
		select {
		case <-received:
		case <-timeout:
			t.Fatal("timed out waiting for deliveries")
		}
	}
	if count.Load() != 2 {
		t.Fatalf("delivery count: got %d, want 2", count.Load())
	}
}
