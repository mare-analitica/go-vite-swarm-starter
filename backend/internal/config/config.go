// Package config loads runtime configuration from environment variables.
//
// Secrets support the *_FILE convention: when DB_PASSWORD_FILE is set, the
// value is read from that file (a mounted Docker Secret) instead of the
// environment, so secret values never appear in `docker inspect`.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the complete runtime configuration of the API.
type Config struct {
	HTTPAddr          string
	CORSAllowedOrigin string
	ShutdownTimeout   time.Duration

	DB DB
	S3 S3

	N8NWebhookURL string
}

// DB holds PostgreSQL connection settings.
type DB struct {
	Host     string
	Port     int
	Name     string
	User     string
	Password string
	SSLMode  string
}

// S3 holds object storage settings. Endpoint is used by the API itself;
// PublicEndpoint is the URL browsers use, and presigned requests are signed for it.
type S3 struct {
	Endpoint       string
	UseSSL         bool
	PublicEndpoint *url.URL
	Region         string
	Bucket         string
	AccessKey      string
	SecretKey      string
	MaxUploadBytes int64
	URLExpiry      time.Duration
}

// DSN returns a pgx connection string.
func (d DB) DSN() string {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(d.User, d.Password),
		Host:     fmt.Sprintf("%s:%d", d.Host, d.Port),
		Path:     "/" + d.Name,
		RawQuery: "sslmode=" + url.QueryEscape(d.SSLMode),
	}
	return u.String()
}

// Load reads configuration from the environment using getenv (os.Getenv in
// production, a map in tests) and readFile for *_FILE secrets.
func Load(getenv func(string) string, readFile func(string) ([]byte, error)) (Config, error) {
	l := loader{getenv: getenv, readFile: readFile}

	cfg := Config{
		HTTPAddr:          l.str("HTTP_ADDR", ":8080"),
		CORSAllowedOrigin: l.str("CORS_ALLOWED_ORIGIN", ""),
		ShutdownTimeout:   l.duration("SHUTDOWN_TIMEOUT", 15*time.Second),
		DB: DB{
			Host:     l.required("DB_HOST"),
			Port:     l.integer("DB_PORT", 5432),
			Name:     l.required("DB_NAME"),
			User:     l.required("DB_USER"),
			Password: l.secret("DB_PASSWORD"),
			SSLMode:  l.str("DB_SSLMODE", "disable"),
		},
		S3: S3{
			Endpoint:       l.required("S3_ENDPOINT"),
			UseSSL:         l.boolean("S3_USE_SSL", false),
			Region:         l.str("S3_REGION", "us-east-1"),
			Bucket:         l.required("S3_BUCKET"),
			AccessKey:      l.secret("S3_ACCESS_KEY"),
			SecretKey:      l.secret("S3_SECRET_KEY"),
			MaxUploadBytes: int64(l.integer("S3_MAX_UPLOAD_BYTES", 10<<20)),
			URLExpiry:      l.duration("S3_URL_EXPIRY", 15*time.Minute),
		},
		N8NWebhookURL: l.str("N8N_WEBHOOK_URL", ""),
	}

	if raw := l.required("S3_PUBLIC_ENDPOINT"); raw != "" {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" || u.Host == "" {
			l.errs = append(l.errs, fmt.Errorf("S3_PUBLIC_ENDPOINT must be an absolute URL, got %q", raw))
		} else {
			cfg.S3.PublicEndpoint = u
		}
	}

	return cfg, errors.Join(l.errs...)
}

type loader struct {
	getenv   func(string) string
	readFile func(string) ([]byte, error)
	errs     []error
}

func (l *loader) str(key, fallback string) string {
	if v := strings.TrimSpace(l.getenv(key)); v != "" {
		return v
	}
	return fallback
}

func (l *loader) required(key string) string {
	v := strings.TrimSpace(l.getenv(key))
	if v == "" {
		l.errs = append(l.errs, fmt.Errorf("%s is required", key))
	}
	return v
}

// secret reads KEY_FILE when set, otherwise KEY. One of them is required.
func (l *loader) secret(key string) string {
	if path := strings.TrimSpace(l.getenv(key + "_FILE")); path != "" {
		b, err := l.readFile(path)
		if err != nil {
			l.errs = append(l.errs, fmt.Errorf("%s_FILE: %w", key, err))
			return ""
		}
		return strings.TrimRight(string(b), "\r\n")
	}
	return l.required(key)
}

func (l *loader) integer(key string, fallback int) int {
	raw := strings.TrimSpace(l.getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		l.errs = append(l.errs, fmt.Errorf("%s must be a positive integer, got %q", key, raw))
		return fallback
	}
	return v
}

func (l *loader) boolean(key string, fallback bool) bool {
	raw := strings.TrimSpace(l.getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("%s must be a boolean, got %q", key, raw))
		return fallback
	}
	return v
}

func (l *loader) duration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(l.getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := time.ParseDuration(raw)
	if err != nil || v <= 0 {
		l.errs = append(l.errs, fmt.Errorf("%s must be a positive duration, got %q", key, raw))
		return fallback
	}
	return v
}

// FromEnvironment loads configuration from the process environment.
func FromEnvironment() (Config, error) {
	return Load(os.Getenv, os.ReadFile)
}
