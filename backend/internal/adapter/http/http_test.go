package http

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/middleware"
	"github.com/bergsantana/encomendo/backend/internal/platform/config"
)

func testRouter() http.Handler {
	log := slog.New(slog.DiscardHandler)
	health := handler.NewHealth(log, time.Second, handler.Check{Name: "postgres", Run: func(context.Context) error { return nil }})
	return NewRouter(log, Handlers{Health: health}, RouterOptions{HSTS: true})
}

func TestRouter_Routes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		method, path string
		wantStatus   int
		wantType     string
	}{
		{http.MethodGet, "/healthz", http.StatusOK, "application/json; charset=utf-8"},
		{http.MethodGet, "/readyz", http.StatusOK, "application/json; charset=utf-8"},
		{http.MethodGet, "/nowhere", http.StatusNotFound, "application/problem+json"},
		{http.MethodPost, "/healthz", http.StatusMethodNotAllowed, "application/problem+json"},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			testRouter().ServeHTTP(w, httptest.NewRequest(tt.method, tt.path, nil))

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if tt.wantType != "" && w.Header().Get("Content-Type") != tt.wantType {
				t.Errorf("Content-Type = %q, want %q", w.Header().Get("Content-Type"), tt.wantType)
			}
		})
	}
}

func TestRouter_MethodNotAllowedListsAllowedMethods(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	testRouter().ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/readyz", nil))

	if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "GET" {
		t.Errorf("status = %d, Allow = %q; want 405 and GET", w.Code, w.Header().Get("Allow"))
	}
	if !strings.Contains(w.Body.String(), `"code":"method_not_allowed"`) {
		t.Errorf("body = %s", w.Body.String())
	}
}

func TestRouter_AppliesMiddleware(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	testRouter().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if w.Header().Get(middleware.HeaderRequestID) == "" {
		t.Error("missing request ID header")
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing security headers")
	}
	if w.Header().Get("Strict-Transport-Security") == "" {
		t.Error("missing HSTS when enabled")
	}
}

func TestServer_ServesUntilContextIsCancelled(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.HTTP{
		ReadHeaderTimeout: time.Second, ReadTimeout: time.Second,
		WriteTimeout: time.Second, IdleTimeout: time.Second, ShutdownTimeout: time.Second,
	}
	srv := NewServer(cfg, testRouter(), slog.New(slog.DiscardHandler))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx, ln) }()

	resp, err := http.Get("http://" + ln.Addr().String() + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"status":"ok"`) {
		t.Errorf("GET /healthz = %d %s", resp.StatusCode, body)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve returned %v after cancel, want nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not return after cancel")
	}
}

func TestServer_RunReportsListenErrors(t *testing.T) {
	t.Parallel()

	srv := NewServer(config.HTTP{Addr: "256.0.0.1:99999", ShutdownTimeout: time.Second},
		http.NotFoundHandler(), slog.New(slog.DiscardHandler))

	if err := srv.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "listen on") {
		t.Errorf("Run error = %v, want listen error", err)
	}
}
