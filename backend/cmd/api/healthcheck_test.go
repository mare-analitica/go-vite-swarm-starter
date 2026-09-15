package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthcheck(t *testing.T) {
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
	}))
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "http://")

	if code := healthcheck(addr); code != 0 {
		t.Errorf("healthy server: exit code = %d", code)
	}
	status = http.StatusServiceUnavailable
	if code := healthcheck(addr); code != 1 {
		t.Errorf("unhealthy server: exit code = %d", code)
	}
	if code := healthcheck("127.0.0.1:1"); code != 1 {
		t.Errorf("closed port: exit code = %d", code)
	}
	if code := healthcheck("not-an-address"); code != 1 {
		t.Errorf("invalid address: exit code = %d", code)
	}
}
