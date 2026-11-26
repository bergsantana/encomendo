package redis

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/platform/config"
)

// deadRedis is a client for a Redis that is not there.
func deadRedis(t *testing.T) *Client {
	t.Helper()
	c, err := Open(config.Redis{URL: "redis://127.0.0.1:1/0"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

var jti = shared.NewV7(1_700_000_000_000, [10]byte{1})

// TestDenylist_DenyNeverFailsTheCallerAndLogsTheFailure: the revoke in
// Postgres committed already, so a Redis outage is logged, not returned
// (ADR 0013).
func TestDenylist_DenyNeverFailsTheCallerAndLogsTheFailure(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	d := NewDenylist(deadRedis(t), slog.New(slog.NewTextHandler(&logs, nil)))

	d.Deny(context.Background(), []shared.ID{jti}, time.Now().Add(15*time.Minute))

	if !strings.Contains(logs.String(), "denylist write failed") {
		t.Errorf("a failed write was not logged: %q", logs.String())
	}
	if strings.Contains(logs.String(), jti.String()) {
		t.Errorf("the log carries the jti: %q", logs.String())
	}
}

// TestDenylist_DenyOutlivesTheRequestContext: logout and reset call Deny on
// a context the request's end does not cancel; an already canceled one must
// not stop the write from being tried.
func TestDenylist_DenyIgnoresTheEndOfTheRequest(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	d := NewDenylist(deadRedis(t), slog.New(slog.NewTextHandler(&logs, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	d.Deny(ctx, []shared.ID{jti}, time.Now().Add(time.Minute))

	if strings.Contains(logs.String(), "context canceled") {
		t.Errorf("the write was abandoned with the request: %q", logs.String())
	}
}

func TestDenylist_DenyDoesNothingForNothingToStop(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	d := NewDenylist(deadRedis(t), slog.New(slog.NewTextHandler(&logs, nil)))

	d.Deny(context.Background(), nil, time.Now().Add(time.Hour))
	d.Deny(context.Background(), []shared.ID{jti}, time.Now().Add(-time.Second))

	if logs.Len() != 0 {
		t.Errorf("Deny tried Redis for nothing: %q", logs.String())
	}
}

// TestDenylist_DeniedFailsClosed: an unreadable denylist is an error for the
// middleware to answer 503, never "not denied" (ADR 0013).
func TestDenylist_DeniedFailsClosed(t *testing.T) {
	t.Parallel()
	d := NewDenylist(deadRedis(t), slog.New(slog.DiscardHandler))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	denied, err := d.Denied(ctx, jti)

	if err == nil || denied {
		t.Errorf("Denied = %v, %v; want an error, and not a pass", denied, err)
	}
}
