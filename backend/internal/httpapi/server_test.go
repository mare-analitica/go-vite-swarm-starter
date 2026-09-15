package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mare-analitica/go-vite-swarm-starter/backend/internal/notes"
	"github.com/mare-analitica/go-vite-swarm-starter/backend/internal/storage"
)

type fakeRepo struct {
	mu    sync.Mutex
	notes map[string]notes.Note
	err   error
}

func newFakeRepo() *fakeRepo { return &fakeRepo{notes: map[string]notes.Note{}} }

func (f *fakeRepo) List(context.Context, int) ([]notes.Note, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []notes.Note{}
	for _, n := range f.notes {
		out = append(out, n)
	}
	return out, f.err
}

func (f *fakeRepo) Get(_ context.Context, id string) (notes.Note, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.notes[id]
	if !ok {
		return notes.Note{}, notes.ErrNotFound
	}
	return n, nil
}

func (f *fakeRepo) Create(_ context.Context, in notes.NewNote) (notes.Note, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := notes.Note{ID: "n1", Title: in.Title, Body: in.Body, CreatedAt: time.Now()}
	f.notes[n.ID] = n
	return n, nil
}

func (f *fakeRepo) Delete(_ context.Context, id string) (notes.Note, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.notes[id]
	if !ok {
		return notes.Note{}, notes.ErrNotFound
	}
	delete(f.notes, id)
	return n, nil
}

func (f *fakeRepo) SetAttachment(_ context.Context, id, key, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.notes[id]
	if !ok {
		return notes.ErrNotFound
	}
	n.AttachmentKey, n.AttachmentName = key, name
	f.notes[id] = n
	return nil
}

type fakeStorage struct {
	pingErr error
	deleted []string
	objects map[string]bool
}

func (f *fakeStorage) Ping(context.Context) error { return f.pingErr }
func (f *fakeStorage) PresignUpload(_ context.Context, key string) (storage.Upload, error) {
	return storage.Upload{URL: "https://s3.example.com/app", Fields: map[string]string{"key": key}, Key: key}, nil
}
func (f *fakeStorage) PresignDownload(_ context.Context, key, _ string) (string, error) {
	return "https://s3.example.com/app/" + key, nil
}
func (f *fakeStorage) Exists(_ context.Context, key string) (bool, error) {
	return f.objects[key], nil
}
func (f *fakeStorage) Delete(_ context.Context, key string) error {
	f.deleted = append(f.deleted, key)
	return nil
}

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

type fakeEvents struct{ published []string }

func (f *fakeEvents) Publish(event string, _ any) { f.published = append(f.published, event) }

type fixture struct {
	repo    *fakeRepo
	storage *fakeStorage
	events  *fakeEvents
	handler http.Handler
}

func newFixture(dbErr error) *fixture {
	f := &fixture{repo: newFakeRepo(), storage: &fakeStorage{objects: map[string]bool{}}, events: &fakeEvents{}}
	f.handler = Handler(Deps{
		Notes:      f.repo,
		Database:   fakePinger{err: dbErr},
		Storage:    f.storage,
		Events:     f.events,
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		CORSOrigin: "https://app.example.com",
	})
	return f
}

func (f *fixture) do(method, target, body string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), method, target, strings.NewReader(body))
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

func TestCreateNotePublishesEvent(t *testing.T) {
	f := newFixture(nil)
	rec := f.do(http.MethodPost, "/api/notes", `{"title":"  Hello  ","body":"world"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	var got notes.Note
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Title != "Hello" {
		t.Errorf("title must be trimmed, got %q", got.Title)
	}
	if len(f.events.published) != 1 || f.events.published[0] != "note.created" {
		t.Errorf("published = %v", f.events.published)
	}
}

func TestCreateNoteValidation(t *testing.T) {
	f := newFixture(nil)
	cases := map[string]struct {
		body string
		code int
	}{
		"missing title": {`{"title":" "}`, http.StatusUnprocessableEntity},
		"unknown field": {`{"title":"x","admin":true}`, http.StatusBadRequest},
		"invalid json":  {`{`, http.StatusBadRequest},
		"too long":      {`{"title":"` + strings.Repeat("a", notes.MaxTitleLength+1) + `"}`, http.StatusUnprocessableEntity},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if rec := f.do(http.MethodPost, "/api/notes", tc.body); rec.Code != tc.code {
				t.Errorf("status = %d, want %d", rec.Code, tc.code)
			}
		})
	}
	if len(f.events.published) != 0 {
		t.Errorf("invalid input must not publish events")
	}
}

func TestGetAndDeleteNotFound(t *testing.T) {
	f := newFixture(nil)
	if rec := f.do(http.MethodGet, "/api/notes/missing", ""); rec.Code != http.StatusNotFound {
		t.Errorf("get status = %d", rec.Code)
	}
	if rec := f.do(http.MethodDelete, "/api/notes/missing", ""); rec.Code != http.StatusNotFound {
		t.Errorf("delete status = %d", rec.Code)
	}
}

func TestAttachmentLifecycle(t *testing.T) {
	f := newFixture(nil)
	f.do(http.MethodPost, "/api/notes", `{"title":"with file"}`)

	rec := f.do(http.MethodPost, "/api/notes/n1/attachment", `{"filename":"..\\..\\Report Q3.pdf"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("presign status = %d, body = %s", rec.Code, rec.Body)
	}
	var up storage.Upload
	_ = json.Unmarshal(rec.Body.Bytes(), &up)
	key := up.Key
	if !strings.HasPrefix(key, "notes/n1/") || strings.Contains(key, "..") || strings.Contains(key, " ") {
		t.Errorf("unsafe object key %q", key)
	}
	if f.repo.notes["n1"].AttachmentKey != "" {
		t.Fatal("presigning must not record the attachment before the upload is confirmed")
	}

	confirm := `{"key":"` + key + `","filename":"Report Q3.pdf"}`
	if rec := f.do(http.MethodPost, "/api/notes/n1/attachment/confirm", confirm); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("confirm without uploaded object: status = %d", rec.Code)
	}
	f.storage.objects[key] = true
	if rec := f.do(http.MethodPost, "/api/notes/n1/attachment/confirm", confirm); rec.Code != http.StatusOK {
		t.Fatalf("confirm status = %d, body = %s", rec.Code, rec.Body)
	}
	if name := f.repo.notes["n1"].AttachmentName; name != "Report Q3.pdf" {
		t.Errorf("attachment name = %q", name)
	}

	if rec := f.do(http.MethodGet, "/api/notes/n1/attachment", ""); rec.Code != http.StatusOK {
		t.Errorf("download status = %d", rec.Code)
	}
	if rec := f.do(http.MethodDelete, "/api/notes/n1", ""); rec.Code != http.StatusNoContent {
		t.Errorf("delete status = %d", rec.Code)
	}
	if len(f.storage.deleted) != 1 || f.storage.deleted[0] != key {
		t.Errorf("attachment object must be deleted, got %v", f.storage.deleted)
	}
}

func TestConfirmRejectsForeignKeysAndReplacesPrevious(t *testing.T) {
	f := newFixture(nil)
	f.do(http.MethodPost, "/api/notes", `{"title":"x"}`)
	f.storage.objects["notes/other/abc-a.txt"] = true
	if rec := f.do(http.MethodPost, "/api/notes/n1/attachment/confirm", `{"key":"notes/other/abc-a.txt","filename":"a.txt"}`); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("foreign key status = %d", rec.Code)
	}

	f.storage.objects["notes/n1/old-a.txt"] = true
	f.storage.objects["notes/n1/new-b.txt"] = true
	f.do(http.MethodPost, "/api/notes/n1/attachment/confirm", `{"key":"notes/n1/old-a.txt","filename":"a.txt"}`)
	if rec := f.do(http.MethodPost, "/api/notes/n1/attachment/confirm", `{"key":"notes/n1/new-b.txt","filename":"b.txt"}`); rec.Code != http.StatusOK {
		t.Fatalf("replace status = %d", rec.Code)
	}
	if len(f.storage.deleted) != 1 || f.storage.deleted[0] != "notes/n1/old-a.txt" {
		t.Errorf("previous attachment must be deleted, got %v", f.storage.deleted)
	}
}

func TestAttachmentRequiresValidFilenameAndNote(t *testing.T) {
	f := newFixture(nil)
	if rec := f.do(http.MethodPost, "/api/notes/missing/attachment", `{"filename":"a.txt"}`); rec.Code != http.StatusNotFound {
		t.Errorf("missing note status = %d", rec.Code)
	}
	f.do(http.MethodPost, "/api/notes", `{"title":"x"}`)
	if rec := f.do(http.MethodPost, "/api/notes/n1/attachment", `{"filename":""}`); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("empty filename status = %d", rec.Code)
	}
	if rec := f.do(http.MethodGet, "/api/notes/n1/attachment", ""); rec.Code != http.StatusNotFound {
		t.Errorf("download without attachment status = %d", rec.Code)
	}
}

func TestReadiness(t *testing.T) {
	if rec := newFixture(nil).do(http.MethodGet, "/readyz", ""); rec.Code != http.StatusOK {
		t.Errorf("ready status = %d", rec.Code)
	}
	if rec := newFixture(errors.New("down")).do(http.MethodGet, "/readyz", ""); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("not ready status = %d", rec.Code)
	}
}

func TestCORSAllowsOnlyConfiguredOrigin(t *testing.T) {
	f := newFixture(nil)
	rec := f.do(http.MethodOptions, "/api/notes", "", "Origin", "https://app.example.com")
	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" {
		t.Errorf("allowed origin: status=%d header=%q", rec.Code, rec.Header().Get("Access-Control-Allow-Origin"))
	}
	rec = f.do(http.MethodGet, "/api/notes", "", "Origin", "https://app.example.com.evil.io")
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("foreign origin must not receive CORS headers")
	}
}
