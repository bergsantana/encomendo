// Package redis is the Redis adapter: the client, and later the tenant-scoped
// cache, the rate limiter and Idempotency-Key storage.
//
// Redis is never the source of truth. A Redis outage degrades reads to
// Postgres; it must never fail a request.
package redis
