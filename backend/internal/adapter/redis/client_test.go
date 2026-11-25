package redis

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/platform/config"
)

func TestOpen_RejectsInvalidURL(t *testing.T) {
	t.Parallel()

	if _, err := Open(config.Redis{URL: "http://not-redis"}); err == nil {
		t.Fatal("Open accepted a non-redis URL")
	}
}

func TestPing_ReportsUnreachableServer(t *testing.T) {
	t.Parallel()

	// Port 1 on localhost refuses connections, so Ping must fail fast.
	c, err := Open(config.Redis{URL: "redis://127.0.0.1:1/0"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Ping(ctx); err == nil || !strings.Contains(err.Error(), "ping redis") {
		t.Errorf("Ping error = %v, want a wrapped connection error", err)
	}
}
