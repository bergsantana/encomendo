package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func ok(context.Context) error { return nil }

func TestHealth_LiveIgnoresDependencies(t *testing.T) {
	t.Parallel()

	h := NewHealth(slog.New(slog.DiscardHandler), time.Second,
		Check{Name: "postgres", Run: func(context.Context) error { return errors.New("down") }})

	w := httptest.NewRecorder()
	h.Live(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestHealth_ReadyWhenAllChecksPass(t *testing.T) {
	t.Parallel()

	h := NewHealth(slog.New(slog.DiscardHandler), time.Second,
		Check{Name: "postgres", Run: ok}, Check{Name: "redis", Run: ok})

	w := httptest.NewRecorder()
	h.Ready(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body statusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Checks["postgres"] != "ok" || body.Checks["redis"] != "ok" {
		t.Errorf("checks = %v", body.Checks)
	}
}

func TestHealth_NotReadyNamesFailedChecksOnly(t *testing.T) {
	t.Parallel()

	h := NewHealth(slog.New(slog.DiscardHandler), time.Second,
		Check{Name: "postgres", Run: ok},
		Check{Name: "redis", Run: func(context.Context) error { return errors.New("dial tcp 10.0.0.7:6379: refused") }})

	w := httptest.NewRecorder()
	h.Ready(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"code":"not_ready"`) || !strings.Contains(body, "redis") {
		t.Errorf("body = %s", body)
	}
	if strings.Contains(body, "10.0.0.7") {
		t.Errorf("internal address leaked: %s", body)
	}
}

func TestHealth_ReadyTimesOutSlowChecks(t *testing.T) {
	t.Parallel()

	slow := func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}
	h := NewHealth(slog.New(slog.DiscardHandler), 20*time.Millisecond, Check{Name: "postgres", Run: slow})

	start := time.Now()
	w := httptest.NewRecorder()
	h.Ready(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("Ready took %v, want it bounded by the timeout", elapsed)
	}
}
