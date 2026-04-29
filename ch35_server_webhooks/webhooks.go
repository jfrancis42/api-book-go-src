package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
)

// WebhookEvent is an outbound event queued for delivery.
type WebhookEvent struct {
	ID        int64
	EventType string
	Payload   []byte
	Attempts  int
	CreatedAt time.Time
	Delivered bool
}

// Subscription is a registered listener for webhook events.
type Subscription struct {
	ID     int64    `json:"id"`
	URL    string   `json:"url"`
	Secret string   `json:"secret,omitempty"`
	Events []string `json:"events"`
	Active bool     `json:"active"`
}

// EventQueue is a thread-safe in-memory event store.
// In production this would be a database table written transactionally
// alongside the business operation that triggered the event.
type EventQueue struct {
	mu     sync.Mutex
	events []*WebhookEvent
	nextID int64
}

func NewEventQueue() *EventQueue {
	return &EventQueue{nextID: 1}
}

// Enqueue adds an event. In production this runs inside the same DB transaction
// as the write that triggered the event, ensuring at-least-once delivery.
func (q *EventQueue) Enqueue(eventType string, payload any) (*WebhookEvent, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	q.mu.Lock()
	e := &WebhookEvent{
		ID:        q.nextID,
		EventType: eventType,
		Payload:   b,
		CreatedAt: time.Now(),
	}
	q.events = append(q.events, e)
	q.nextID++
	q.mu.Unlock()
	return e, nil
}

func (q *EventQueue) Pending() []*WebhookEvent {
	q.mu.Lock()
	defer q.mu.Unlock()
	var out []*WebhookEvent
	for _, e := range q.events {
		if !e.Delivered {
			out = append(out, e)
		}
	}
	return out
}

func (q *EventQueue) markDelivered(id int64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, e := range q.events {
		if e.ID == id {
			e.Delivered = true
			return
		}
	}
}

func (q *EventQueue) incrementAttempts(id int64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, e := range q.events {
		if e.ID == id {
			e.Attempts++
			return
		}
	}
}

// SubscriptionStore manages webhook subscriptions.
type SubscriptionStore struct {
	mu     sync.RWMutex
	subs   []*Subscription
	nextID int64
}

func NewSubscriptionStore() *SubscriptionStore {
	return &SubscriptionStore{nextID: 1}
}

func (s *SubscriptionStore) Create(url, secret string, events []string) *Subscription {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub := &Subscription{
		ID:     s.nextID,
		URL:    url,
		Secret: secret,
		Events: events,
		Active: true,
	}
	s.subs = append(s.subs, sub)
	s.nextID++
	return sub
}

func (s *SubscriptionStore) ForEvent(eventType string) []*Subscription {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Subscription
	for _, sub := range s.subs {
		if !sub.Active {
			continue
		}
		for _, e := range sub.Events {
			if e == eventType || e == "*" {
				out = append(out, sub)
				break
			}
		}
	}
	return out
}

// sign computes X-Hub-Signature-256 for a payload using HMAC-SHA256.
func sign(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// DeliveryAttempts tracks delivery attempt count per event for tests.
var DeliveryAttempts atomic.Int64

// WebhookWorker polls the event queue and delivers to subscribers.
type WebhookWorker struct {
	queue  *EventQueue
	subs   *SubscriptionStore
	client *http.Client
	done   chan struct{}
}

func NewWebhookWorker(queue *EventQueue, subs *SubscriptionStore) *WebhookWorker {
	return &WebhookWorker{
		queue:  queue,
		subs:   subs,
		client: &http.Client{Timeout: 5 * time.Second},
		done:   make(chan struct{}),
	}
}

// Start launches the background delivery loop.
func (w *WebhookWorker) Start(interval time.Duration) {
	go w.run(interval)
}

// Stop signals the worker to exit.
func (w *WebhookWorker) Stop() {
	close(w.done)
}

// ProcessOnce delivers all pending events immediately (useful in tests).
func (w *WebhookWorker) ProcessOnce(ctx context.Context) {
	for _, event := range w.queue.Pending() {
		w.deliver(ctx, event)
	}
}

func (w *WebhookWorker) run(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-w.done:
			return
		case <-ticker.C:
			w.ProcessOnce(context.Background())
		}
	}
}

func (w *WebhookWorker) deliver(ctx context.Context, event *WebhookEvent) {
	subs := w.subs.ForEvent(event.EventType)
	for _, sub := range subs {
		DeliveryAttempts.Add(1)
		if err := w.send(ctx, sub, event); err != nil {
			w.queue.incrementAttempts(event.ID)
		} else {
			w.queue.markDelivered(event.ID)
		}
	}
}

func (w *WebhookWorker) send(ctx context.Context, sub *Subscription, event *WebhookEvent) error {
	req, err := http.NewRequestWithContext(ctx, "POST", sub.URL, bytes.NewReader(event.Payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Event", event.EventType)
	req.Header.Set("X-Hub-Signature-256", sign(sub.Secret, event.Payload))
	req.Header.Set("X-Delivery-ID", fmt.Sprintf("%d", event.ID))

	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("subscriber returned %d", resp.StatusCode)
	}
	return nil
}

// Todo is used in handler examples.
type Todo struct {
	ID   int64  `json:"id"`
	Text string `json:"text"`
}

type TodoStore struct {
	mu     sync.Mutex
	todos  map[int64]*Todo
	nextID int64
}

func NewTodoStore() *TodoStore {
	return &TodoStore{todos: make(map[int64]*Todo), nextID: 1}
}

func (s *TodoStore) Create(text string) *Todo {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := &Todo{ID: s.nextID, Text: text}
	s.todos[s.nextID] = t
	s.nextID++
	return t
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// BuildRouter wires up the application routes with webhook event emission.
func BuildRouter(todos *TodoStore, queue *EventQueue, subs *SubscriptionStore) http.Handler {
	r := chi.NewRouter()

	r.Post("/todos", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Text string `json:"text"` }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Text) == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		todo := todos.Create(req.Text)
		// Emit event alongside the write — in production this is a single DB transaction.
		queue.Enqueue("todo.created", todo)
		writeJSON(w, http.StatusCreated, todo)
	})

	r.Post("/webhooks/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			URL    string   `json:"url"`
			Secret string   `json:"secret"`
			Events []string `json:"events"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if !strings.HasPrefix(req.URL, "http") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid url"})
			return
		}
		sub := subs.Create(req.URL, req.Secret, req.Events)
		writeJSON(w, http.StatusCreated, sub)
	})

	return r
}
