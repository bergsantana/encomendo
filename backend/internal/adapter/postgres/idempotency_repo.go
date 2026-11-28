package postgres

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
)

// Idempotency implements appshared.Idempotency on idempotency_keys
// (0014_idempotency), in the caller's transaction and under the store of
// its scope (tenant_isolation). A key lives 24 hours; an expired row is
// ignored by Recall and replaced by Remember, and the M6 worker will delete
// old rows.
type Idempotency struct{}

var _ appshared.Idempotency = (*Idempotency)(nil)

// NewIdempotency returns the Postgres key store.
func NewIdempotency() *Idempotency { return &Idempotency{} }

type idempotencyRow struct {
	RequestHash []byte
	ResourceID  string
}

// Recall follows the appshared.Idempotency contract. The request hash is
// compared in constant time, so a timing difference never tells a caller
// how much of another request it guessed.
func (*Idempotency) Recall(ctx context.Context, k appshared.IdempotencyKey) (string, bool, error) {
	db, err := conn(ctx)
	if err != nil {
		return "", false, err
	}
	var rows []idempotencyRow
	err = db.Raw(`SELECT request_hash, resource_id::text AS resource_id FROM idempotency_keys
		WHERE tenant_id = current_tenant_id() AND key = ? AND expires_at > now()`, k.Key).Scan(&rows).Error
	if err != nil {
		return "", false, fmt.Errorf("recall idempotency key: %w", mapError(err))
	}
	if len(rows) == 0 {
		return "", false, nil
	}
	if subtle.ConstantTimeCompare(rows[0].RequestHash, k.RequestHash[:]) != 1 {
		return "", false, appshared.ErrIdempotencyKeyReused
	}
	return rows[0].ResourceID, true, nil
}

// Remember follows the appshared.Idempotency contract. An expired row with
// the key is deleted first. A live one, or one a concurrent first call
// inserts meanwhile (the insert waits on the primary key until that call
// commits), is ErrIdempotencyKeyInFlight: this transaction then rolls back
// and the retry replays the winner's resource.
func (*Idempotency) Remember(ctx context.Context, k appshared.IdempotencyKey, resource string, ttl time.Duration) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	err = db.Exec(`DELETE FROM idempotency_keys WHERE tenant_id = current_tenant_id() AND key = ? AND expires_at <= now()`, k.Key).Error
	if err != nil {
		return fmt.Errorf("drop an expired idempotency key: %w", mapError(err))
	}
	err = db.Exec(`INSERT INTO idempotency_keys (tenant_id, key, request_hash, resource_id, expires_at)
		VALUES (current_tenant_id(), ?, ?, CAST(? AS uuid), now() + make_interval(secs => ?))`,
		k.Key, k.RequestHash[:], resource, ttl.Seconds()).Error
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "idempotency_keys_pkey" {
		return appshared.ErrIdempotencyKeyInFlight
	}
	return fmt.Errorf("remember idempotency key: %w", mapError(err))
}
