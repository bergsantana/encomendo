package redis

import (
	"context"
	"fmt"

	goredis "github.com/redis/go-redis/v9"

	"github.com/bergsantana/encomendo/backend/internal/platform/config"
)

// Client wraps the go-redis client.
type Client struct {
	rdb *goredis.Client
}

// Open parses the Redis URL and returns a client. It does not connect: the
// API starts even when Redis is down, and readiness reports it.
func Open(cfg config.Redis) (*Client, error) {
	opts, err := goredis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse REDIS_URL: %w", err)
	}
	return &Client{rdb: goredis.NewClient(opts)}, nil
}

// Ping checks that Redis answers. It backs the readiness probe.
func (c *Client) Ping(ctx context.Context) error {
	if err := c.rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping redis: %w", err)
	}
	return nil
}

// Close closes the connection pool.
func (c *Client) Close() error {
	if err := c.rdb.Close(); err != nil {
		return fmt.Errorf("close redis: %w", err)
	}
	return nil
}
