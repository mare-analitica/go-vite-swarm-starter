package events

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWebhookPostsEvent(t *testing.T) {
	received := make(chan Event, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content type = %q", r.Header.Get("Content-Type"))
		}
		var ev Event
		if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
			t.Errorf("decode: %v", err)
		}
		received <- ev
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wh := NewWebhook(srv.URL, slog.New(slog.NewTextHandler(io.Discard, nil)))
	wh.Publish("note.created", map[string]string{"id": "42"})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	wh.Wait(ctx)

	select {
	case ev := <-received:
		if ev.Type != "note.created" {
			t.Errorf("type = %q", ev.Type)
		}
	default:
		t.Fatal("webhook was not called")
	}
}

func TestWebhookDisabledWithoutURL(t *testing.T) {
	wh := NewWebhook("", slog.New(slog.NewTextHandler(io.Discard, nil)))
	wh.Publish("note.created", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	wh.Wait(ctx)
	if ctx.Err() != nil {
		t.Fatal("Wait must return immediately when nothing is pending")
	}
}
