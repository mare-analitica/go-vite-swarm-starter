// Package httpapi exposes the notes API over HTTP.
package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/mare-analitica/go-vite-swarm-starter/backend/internal/events"
	"github.com/mare-analitica/go-vite-swarm-starter/backend/internal/notes"
	"github.com/mare-analitica/go-vite-swarm-starter/backend/internal/storage"
)

const (
	maxJSONBody = 64 << 10
	listLimit   = 100
)

// Storage is the object storage used for attachments.
type Storage interface {
	Ping(ctx context.Context) error
	PresignUpload(ctx context.Context, key string) (storage.Upload, error)
	PresignDownload(ctx context.Context, key, filename string) (string, error)
	Delete(ctx context.Context, key string) error
}

// Pinger reports dependency health.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Deps are the collaborators of the API.
type Deps struct {
	Notes      notes.Repository
	Database   Pinger
	Storage    Storage
	Events     events.Publisher
	Log        *slog.Logger
	CORSOrigin string
}

// Handler builds the HTTP handler with routes and middleware.
func Handler(d Deps) http.Handler {
	s := &server{Deps: d}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /readyz", s.readyz)
	mux.HandleFunc("GET /api/notes", s.listNotes)
	mux.HandleFunc("POST /api/notes", s.createNote)
	mux.HandleFunc("GET /api/notes/{id}", s.getNote)
	mux.HandleFunc("DELETE /api/notes/{id}", s.deleteNote)
	mux.HandleFunc("POST /api/notes/{id}/attachment", s.presignAttachmentUpload)
	mux.HandleFunc("GET /api/notes/{id}/attachment", s.presignAttachmentDownload)

	return s.recoverer(s.logRequests(s.cors(securityHeaders(mux))))
}

type server struct {
	Deps
}

func (s *server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	checks := map[string]string{"database": "ok", "storage": "ok"}
	status := http.StatusOK
	if err := s.Database.Ping(ctx); err != nil {
		checks["database"] = "unavailable"
		status = http.StatusServiceUnavailable
	}
	if err := s.Storage.Ping(ctx); err != nil {
		checks["storage"] = "unavailable"
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, checks)
}

func (s *server) listNotes(w http.ResponseWriter, r *http.Request) {
	list, err := s.Notes.List(r.Context(), listLimit)
	if err != nil {
		s.internalError(w, r, "list notes", err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *server) createNote(w http.ResponseWriter, r *http.Request) {
	var in notes.NewNote
	if !decodeJSON(w, r, &in) {
		return
	}
	if msg := in.Normalize(); msg != "" {
		writeError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	note, err := s.Notes.Create(r.Context(), in)
	if err != nil {
		s.internalError(w, r, "create note", err)
		return
	}
	s.Events.Publish("note.created", map[string]string{"id": note.ID, "title": note.Title})
	writeJSON(w, http.StatusCreated, note)
}

func (s *server) getNote(w http.ResponseWriter, r *http.Request) {
	note, err := s.Notes.Get(r.Context(), r.PathValue("id"))
	if s.handleLookupError(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, note)
}

func (s *server) deleteNote(w http.ResponseWriter, r *http.Request) {
	note, err := s.Notes.Delete(r.Context(), r.PathValue("id"))
	if s.handleLookupError(w, r, err) {
		return
	}
	if note.AttachmentKey != "" {
		if err := s.Storage.Delete(r.Context(), note.AttachmentKey); err != nil {
			// The note is gone; an orphan object is logged, not surfaced.
			s.Log.Warn("delete attachment", "key", note.AttachmentKey, "error", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

type attachmentRequest struct {
	Filename string `json:"filename"`
}

var unsafeFilenameChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func (s *server) presignAttachmentUpload(w http.ResponseWriter, r *http.Request) {
	var in attachmentRequest
	if !decodeJSON(w, r, &in) {
		return
	}
	name := strings.TrimSpace(path.Base(strings.ReplaceAll(in.Filename, `\`, "/")))
	if name == "" || name == "." || name == "/" || len(name) > 200 {
		writeError(w, http.StatusUnprocessableEntity, "a filename up to 200 characters is required")
		return
	}

	id := r.PathValue("id")
	if _, err := s.Notes.Get(r.Context(), id); s.handleLookupError(w, r, err) {
		return
	}

	key := "notes/" + id + "/" + randomHex(8) + "-" + unsafeFilenameChars.ReplaceAllString(name, "_")
	upload, err := s.Storage.PresignUpload(r.Context(), key)
	if err != nil {
		s.internalError(w, r, "presign upload", err)
		return
	}
	if err := s.Notes.SetAttachment(r.Context(), id, key, name); s.handleLookupError(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, upload)
}

func (s *server) presignAttachmentDownload(w http.ResponseWriter, r *http.Request) {
	note, err := s.Notes.Get(r.Context(), r.PathValue("id"))
	if s.handleLookupError(w, r, err) {
		return
	}
	if note.AttachmentKey == "" {
		writeError(w, http.StatusNotFound, "note has no attachment")
		return
	}
	u, err := s.Storage.PresignDownload(r.Context(), note.AttachmentKey, note.AttachmentName)
	if err != nil {
		s.internalError(w, r, "presign download", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": u})
}

// handleLookupError writes 404 or 500 and reports whether err was handled.
func (s *server) handleLookupError(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, notes.ErrNotFound):
		writeError(w, http.StatusNotFound, "note not found")
	default:
		s.internalError(w, r, "lookup note", err)
	}
	return true
}

func (s *server) internalError(w http.ResponseWriter, r *http.Request, op string, err error) {
	s.Log.Error(op, "error", err, "request_id", requestID(r.Context()))
	writeError(w, http.StatusInternalServerError, "internal error")
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
