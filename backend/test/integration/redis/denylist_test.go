//go:build integration

// Package redis_test runs the Redis adapters against a real Redis in a
// testcontainers container (backend/CLAUDE.md, Testing).
package redis_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/bergsantana/encomendo/backend/internal/adapter/redis"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/platform/config"
	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

var redisURL string

func TestMain(m *testing.M) { os.Exit(run(m)) }

func run(m *testing.M) int {
	ctx := context.Background()
	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "redis:7-alpine",
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor:   wait.ForLog("Ready to accept connections").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "start redis:", err)
		return 1
	}
	defer func() { _ = testcontainers.TerminateContainer(ctr) }() // the reaper removes it if this fails
	host, err := ctr.Host(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	port, err := ctr.MappedPort(ctx, "6379/tcp")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	redisURL = fmt.Sprintf("redis://%s:%s/0", host, port.Port())
	return m.Run()
}

func newDenylist(t *testing.T) *redis.Denylist {
	t.Helper()
	c, err := redis.Open(config.Redis{URL: redisURL})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return redis.NewDenylist(c, slog.New(slog.DiscardHandler))
}

func TestDenylist_DeniedTokensStayDeniedUntilTheyWouldHaveExpired(t *testing.T) {
	t.Parallel()
	d := newDenylist(t)
	ctx := t.Context()
	stopped, other := pgtest.NewID(), pgtest.NewID()

	d.Deny(ctx, []shared.ID{stopped}, time.Now().Add(time.Minute))

	for jti, want := range map[shared.ID]bool{stopped: true, other: false} {
		got, err := d.Denied(ctx, jti)
		if err != nil || got != want {
			t.Errorf("Denied(%v) = %v, %v; want %v", jti, got, err, want)
		}
	}
}

func TestDenylist_ADeniedTokenIsForgottenWhenItsTimeIsUp(t *testing.T) {
	t.Parallel()
	d := newDenylist(t)
	ctx := t.Context()
	jti := pgtest.NewID()

	d.Deny(ctx, []shared.ID{jti}, time.Now().Add(300*time.Millisecond))

	deadline := time.Now().Add(5 * time.Second)
	for {
		denied, err := d.Denied(ctx, jti)
		if err != nil {
			t.Fatal(err)
		}
		if !denied {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the jti is still denied long after its time: the key has no expiry")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestDenylist_DenyWorksAfterTheRequestEnded: logout and password reset deny
// on a context the request's end does not cancel, and a dropped connection
// must not leave a thief's access token alive (ADR 0013).
func TestDenylist_DenyWorksAfterTheRequestEnded(t *testing.T) {
	t.Parallel()
	d := newDenylist(t)
	jti := pgtest.NewID()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	d.Deny(ctx, []shared.ID{jti}, time.Now().Add(time.Minute))

	if denied, err := d.Denied(t.Context(), jti); err != nil || !denied {
		t.Errorf("Denied = %v, %v; want the jti denied although the request's context was canceled", denied, err)
	}
}

func TestDenylist_DeniesEveryJtiOfOneCall(t *testing.T) {
	t.Parallel()
	d := newDenylist(t)
	ctx := t.Context()
	jtis := []shared.ID{pgtest.NewID(), pgtest.NewID(), pgtest.NewID()}

	d.Deny(ctx, jtis, time.Now().Add(time.Minute))

	for _, jti := range jtis {
		if denied, err := d.Denied(ctx, jti); err != nil || !denied {
			t.Errorf("Denied(%v) = %v, %v; want true", jti, denied, err)
		}
	}
}
