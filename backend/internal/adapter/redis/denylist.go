package redis

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// denyKeyPrefix starts every denylist key. These keys hold no tenant data,
// only the jti of an access token that must stop, so they use the global
// prefix g: of docs/architecture.md section 7 instead of a tenant's t:{id}:
// a jti is looked up before any tenant is known.
const denyKeyPrefix = "g:auth:deny:"

// denyTimeout bounds each denylist call. On a write the revoke in Postgres
// has committed already, so a slow Redis must not hold up the answer; on a
// read it turns a hung Redis into a quick 503 (ADR 0013).
const denyTimeout = 2 * time.Second

// Denylist stops access tokens before they expire, keeping the jtis of
// revoked sessions until their tokens would have expired anyway (ADR 0013).
// It implements appidentity.Denylist for the use cases, and Denied for the
// authn middleware.
type Denylist struct {
	rdb *goredis.Client
	log *slog.Logger
	now func() time.Time
}

// NewDenylist returns the denylist on c.
func NewDenylist(c *Client, log *slog.Logger) *Denylist {
	return &Denylist{rdb: c.rdb, log: log, now: time.Now}
}

// Deny stops the access tokens with these jtis until until. It never fails
// the caller: a Redis error is logged. It runs on a context the request's
// end does not cancel, and adds only its own short timeout.
func (d *Denylist) Deny(ctx context.Context, jtis []shared.ID, until time.Time) {
	ttl := until.Sub(d.now())
	if len(jtis) == 0 || ttl <= 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), denyTimeout)
	defer cancel()
	_, err := d.rdb.Pipelined(ctx, func(p goredis.Pipeliner) error {
		for _, jti := range jtis {
			p.Set(ctx, denyKeyPrefix+jti.String(), 1, ttl)
		}
		return nil
	})
	if err != nil {
		d.log.ErrorContext(ctx, "denylist write failed; the revoked access tokens work until they expire",
			"count", len(jtis), "err", err)
	}
}

// Denied reports whether the access token with this jti was stopped. The
// authn middleware calls it on every request and answers 503 when it
// returns an error: a denylist that cannot be read fails closed, never
// open and never as a 401 (ADR 0013).
func (d *Denylist) Denied(ctx context.Context, jti shared.ID) (bool, error) {
	// Bounded like a write, so a hung Redis answers 503 quickly instead of
	// holding every request for the socket timeout.
	ctx, cancel := context.WithTimeout(ctx, denyTimeout)
	defer cancel()
	n, err := d.rdb.Exists(ctx, denyKeyPrefix+jti.String()).Result()
	if err != nil {
		return false, fmt.Errorf("read denylist: %w", err)
	}
	return n > 0, nil
}
