package config

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"time"
)

func baseEnv() map[string]string {
	return map[string]string{
		"DB_HOST":            "postgres",
		"DB_NAME":            "app",
		"DB_USER":            "app",
		"DB_PASSWORD_FILE":   "/run/secrets/db_password",
		"S3_ENDPOINT":        "minio:9000",
		"S3_PUBLIC_ENDPOINT": "https://s3.example.com",
		"S3_BUCKET":          "app",
		"S3_ACCESS_KEY_FILE": "/run/secrets/s3_access_key",
		"S3_SECRET_KEY_FILE": "/run/secrets/s3_secret_key",
	}
}

func fakeFiles(files map[string]string) func(string) ([]byte, error) {
	return func(path string) ([]byte, error) {
		v, ok := files[path]
		if !ok {
			return nil, fs.ErrNotExist
		}
		return []byte(v), nil
	}
}

func TestLoadReadsSecretsFromFiles(t *testing.T) {
	env := baseEnv()
	files := fakeFiles(map[string]string{
		"/run/secrets/db_password":   "db-secret\n",
		"/run/secrets/s3_access_key": "access",
		"/run/secrets/s3_secret_key": "s3-secret",
	})

	cfg, err := Load(func(k string) string { return env[k] }, files)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.DB.Password != "db-secret" {
		t.Errorf("trailing newline must be trimmed, got %q", cfg.DB.Password)
	}
	if cfg.S3.PublicEndpoint.Host != "s3.example.com" {
		t.Errorf("public endpoint host = %q", cfg.S3.PublicEndpoint.Host)
	}
	if cfg.HTTPAddr != ":8080" || cfg.S3.URLExpiry != 15*time.Minute || cfg.S3.MaxUploadBytes != 10<<20 {
		t.Errorf("defaults not applied: %+v", cfg)
	}
	if !strings.Contains(cfg.DB.DSN(), "sslmode=disable") {
		t.Errorf("DSN missing sslmode: %s", cfg.DB.DSN())
	}
}

func TestLoadReportsEveryProblem(t *testing.T) {
	env := map[string]string{
		"DB_PORT":            "not-a-number",
		"S3_PUBLIC_ENDPOINT": "s3.example.com",
		"DB_PASSWORD_FILE":   "/missing",
	}

	_, err := Load(func(k string) string { return env[k] }, fakeFiles(nil))
	if err == nil {
		t.Fatal("Load() must fail")
	}
	for _, want := range []string{"DB_HOST is required", "DB_PORT must be a positive integer", "S3_PUBLIC_ENDPOINT must be an absolute URL", "DB_PASSWORD_FILE"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing secret file must wrap fs.ErrNotExist")
	}
}

func TestDSNEscapesCredentials(t *testing.T) {
	d := DB{Host: "db", Port: 5432, Name: "app", User: "app", Password: "p@ss/word", SSLMode: "disable"}
	if strings.Contains(d.DSN(), "p@ss/word") {
		t.Errorf("password must be escaped in DSN")
	}
}
