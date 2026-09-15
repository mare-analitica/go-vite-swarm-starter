// Package notes holds the domain model shared by storage and HTTP layers.
package notes

import (
	"context"
	"errors"
	"strings"
	"time"
)

// MaxTitleLength and MaxBodyLength bound user input.
const (
	MaxTitleLength = 200
	MaxBodyLength  = 10_000
)

// ErrNotFound is returned when a note does not exist.
var ErrNotFound = errors.New("note not found")

// Note is a text note with an optional attachment stored in object storage.
type Note struct {
	ID             string    `json:"id"`
	Title          string    `json:"title"`
	Body           string    `json:"body"`
	AttachmentKey  string    `json:"-"`
	AttachmentName string    `json:"attachmentName,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
}

// NewNote is the validated input for creating a note.
type NewNote struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// Normalize trims input and returns a validation message, or "" when valid.
func (n *NewNote) Normalize() string {
	n.Title = strings.TrimSpace(n.Title)
	n.Body = strings.TrimSpace(n.Body)
	switch {
	case n.Title == "":
		return "title is required"
	case len([]rune(n.Title)) > MaxTitleLength:
		return "title is too long"
	case len([]rune(n.Body)) > MaxBodyLength:
		return "body is too long"
	}
	return ""
}

// Repository persists notes.
type Repository interface {
	List(ctx context.Context, limit int) ([]Note, error)
	Get(ctx context.Context, id string) (Note, error)
	Create(ctx context.Context, in NewNote) (Note, error)
	Delete(ctx context.Context, id string) (Note, error)
	SetAttachment(ctx context.Context, id, key, name string) error
}
