// Package events publishes domain events to an n8n webhook.
//
// Publishing is best effort: it runs in the background with a timeout and
// never fails or delays the HTTP request that produced the event.
package events

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Publisher sends events.
type Publisher interface {
	Publish(event string, data any)
}

// Event is the JSON payload posted to the webhook.
type Event struct {
	Type       string    `json:"type"`
	OccurredAt time.Time `json:"occurredAt"`
	Data       any       `json:"data"`
}

// Webhook posts events to a URL. A zero URL disables publishing.
type Webhook struct {
	url     string
	client  *http.Client
	log     *slog.Logger
	pending sync.WaitGroup
	now     func() time.Time
}

// NewWebhook creates a publisher. When url is empty, Publish is a no-op.
func NewWebhook(url string, log *slog.Logger) *Webhook {
	return &Webhook{
		url:    url,
		client: &http.Client{Timeout: 5 * time.Second},
		log:    log,
		now:    time.Now,
	}
}

// Publish sends the event in the background.
func (w *Webhook) Publish(event string, data any) {
	if w.url == "" {
		return
	}
	payload, err := json.Marshal(Event{Type: event, OccurredAt: w.now().UTC(), Data: data})
	if err != nil {
		w.log.Error("encode event", "event", event, "error", err)
		return
	}
	w.pending.Add(1)
	go func() {
		defer w.pending.Done()
		if err := w.send(payload); err != nil {
			w.log.Warn("publish event failed", "event", event, "error", err)
		}
	}()
}

func (w *Webhook) send(payload []byte) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, w.url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook responded %d", resp.StatusCode)
	}
	return nil
}

// Wait blocks until in-flight events are sent or ctx is done.
func (w *Webhook) Wait(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		w.pending.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}
