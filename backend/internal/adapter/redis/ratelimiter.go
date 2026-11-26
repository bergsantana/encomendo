package redis

import (
	"context"
	"crypto/rand"
	_ "embed" // the rate-limit script
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
)

//go:embed ratelimit.lua
var rateLimitLua string

// limitTimeout bounds one count. A Redis that hangs instead of refusing
// would otherwise hold each limited request for the client's read timeout,
// twice on routes with two limits, and use up the /auth deadline: the
// request would fail instead of passing (ADR 0021).
const limitTimeout = 250 * time.Millisecond

// RateLimiter counts attempts in sliding windows, one sorted set per key,
// with one script call per attempt (ADR 0021). It implements
// adapter/http/ratelimit.Limiter.
type RateLimiter struct {
	rdb    *goredis.Client
	script *goredis.Script
	clock  appshared.Clock
	log    *slog.Logger
}

// NewRateLimiter returns the limiter on c, taking the time from clock so
// that every API instance and every test counts on the same timeline.
func NewRateLimiter(c *Client, clock appshared.Clock, log *slog.Logger) *RateLimiter {
	return &RateLimiter{rdb: c.rdb, script: goredis.NewScript(rateLimitLua), clock: clock, log: log}
}

// Allow counts one attempt under key and reports whether it is within limit
// attempts per window, and if not, how long until it would be. A refused
// attempt is not counted.
//
// It never fails the caller. When Redis cannot be reached, or does not
// answer within limitTimeout, the attempt is allowed and the failure logged
// at error, as the cache does (ADR 0021). A request that ended first is
// allowed without a log line; its own handler stops it.
func (l *RateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, time.Duration) {
	ok, wait, err := l.count(ctx, key, limit, window)
	if err != nil {
		if ctx.Err() == nil {
			// The key's value is a client's IP or an email's HMAC: only the
			// rule goes to the log.
			l.log.ErrorContext(ctx, "rate limit not checked; the attempt is allowed", "rule", ruleOf(key), "err", err)
		}
		return true, 0
	}
	return ok, wait
}

func (l *RateLimiter) count(ctx context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	if limit < 1 || window < time.Millisecond {
		return false, 0, fmt.Errorf("rate limit: %d per %v is not a limit", limit, window)
	}
	member, err := attemptID()
	if err != nil {
		return false, 0, err
	}
	ctx, cancel := context.WithTimeout(ctx, limitTimeout)
	defer cancel()
	now := l.clock.Now().UnixMilli()
	res, err := l.script.Run(ctx, l.rdb, []string{key},
		now, window.Milliseconds(), limit, strconv.FormatInt(now, 10)+"-"+member).Int64Slice()
	if err != nil {
		return false, 0, fmt.Errorf("rate limit script: %w", err)
	}
	if len(res) != 2 {
		return false, 0, errors.New("rate limit script: unexpected answer")
	}
	return res[0] == 1, time.Duration(res[1]) * time.Millisecond, nil
}

// ruleOf returns the rule and kind of a key, g:rl:{rule}:{kind}:{value},
// without its value.
func ruleOf(key string) string {
	parts := strings.SplitN(key, ":", 5)
	if len(parts) < 4 {
		return "unknown"
	}
	return parts[2] + ":" + parts[3]
}

// attemptID makes each attempt its own member of the sorted set, so two
// attempts in the same millisecond both count.
func attemptID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rate limit attempt id: %w", err)
	}
	return hex.EncodeToString(b), nil
}
