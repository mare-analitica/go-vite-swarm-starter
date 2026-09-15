// Package store implements the notes repository on PostgreSQL.
package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mare-analitica/go-vite-swarm-starter/backend/internal/notes"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// migrationLockID is an arbitrary constant for pg_advisory_lock, so two API
// replicas starting at once never apply migrations concurrently.
const migrationLockID = 727_001

// Postgres is a notes.Repository backed by PostgreSQL.
type Postgres struct {
	pool *pgxpool.Pool
}

// Open connects to PostgreSQL and verifies the connection.
func Open(ctx context.Context, dsn string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Postgres{pool: pool}, nil
}

// Close releases the connection pool.
func (p *Postgres) Close() { p.pool.Close() }

// Ping reports whether the database is reachable.
func (p *Postgres) Ping(ctx context.Context) error { return p.pool.Ping(ctx) }

// Migrate applies embedded migrations in filename order, each in its own
// transaction, recording applied versions in schema_migrations.
func (p *Postgres) Migrate(ctx context.Context) (applied []string, err error) {
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockID); err != nil {
		return nil, fmt.Errorf("advisory lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", migrationLockID)
	}()

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version text PRIMARY KEY,
		applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return nil, fmt.Errorf("create schema_migrations: %w", err)
	}

	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return nil, err
	}
	sort.Strings(names)

	for _, name := range names {
		version := strings.TrimSuffix(strings.TrimPrefix(name, "migrations/"), ".sql")
		var exists bool
		if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)", version).Scan(&exists); err != nil {
			return applied, err
		}
		if exists {
			continue
		}
		sql, err := migrationFiles.ReadFile(name)
		if err != nil {
			return applied, err
		}
		if err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(sql)); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", version)
			return err
		}); err != nil {
			return applied, fmt.Errorf("migration %s: %w", version, err)
		}
		applied = append(applied, version)
	}
	return applied, nil
}

const noteColumns = "id::text, title, body, COALESCE(attachment_key, ''), COALESCE(attachment_name, ''), created_at"

func scanNote(row pgx.Row) (notes.Note, error) {
	var n notes.Note
	err := row.Scan(&n.ID, &n.Title, &n.Body, &n.AttachmentKey, &n.AttachmentName, &n.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return notes.Note{}, notes.ErrNotFound
	}
	return n, err
}

// isInvalidID reports a malformed UUID, which callers treat as "not found".
func isInvalidID(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "22P02"
}

// List returns the most recent notes.
func (p *Postgres) List(ctx context.Context, limit int) ([]notes.Note, error) {
	rows, err := p.pool.Query(ctx, "SELECT "+noteColumns+" FROM notes ORDER BY created_at DESC LIMIT $1", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []notes.Note{}
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// Get returns one note.
func (p *Postgres) Get(ctx context.Context, id string) (notes.Note, error) {
	n, err := scanNote(p.pool.QueryRow(ctx, "SELECT "+noteColumns+" FROM notes WHERE id = $1", id))
	if isInvalidID(err) {
		return notes.Note{}, notes.ErrNotFound
	}
	return n, err
}

// Create inserts a note.
func (p *Postgres) Create(ctx context.Context, in notes.NewNote) (notes.Note, error) {
	return scanNote(p.pool.QueryRow(ctx,
		"INSERT INTO notes (title, body) VALUES ($1, $2) RETURNING "+noteColumns, in.Title, in.Body))
}

// Delete removes a note and returns it, so the caller can clean up its attachment.
func (p *Postgres) Delete(ctx context.Context, id string) (notes.Note, error) {
	n, err := scanNote(p.pool.QueryRow(ctx, "DELETE FROM notes WHERE id = $1 RETURNING "+noteColumns, id))
	if isInvalidID(err) {
		return notes.Note{}, notes.ErrNotFound
	}
	return n, err
}

// SetAttachment records the object key and original filename of a note's attachment.
func (p *Postgres) SetAttachment(ctx context.Context, id, key, name string) error {
	tag, err := p.pool.Exec(ctx, "UPDATE notes SET attachment_key = $2, attachment_name = $3 WHERE id = $1", id, key, name)
	if isInvalidID(err) || (err == nil && tag.RowsAffected() == 0) {
		return notes.ErrNotFound
	}
	return err
}
