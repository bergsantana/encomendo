package postgres

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// mapError turns a GORM or Postgres error into the domain sentinel the API
// maps to a problem code, keeping the original in the chain so callers can
// still read the *pgconn.PgError (for example its constraint name).
//
//   - no row: shared.ErrNotFound;
//   - unique violation, serialization failure, deadlock: shared.ErrConflict
//     (the last two are worth retrying);
//   - foreign key, check or not-null violation: shared.ErrInvalid.
//
// Everything else passes through and becomes a logged 500. That includes a
// row refused by RLS or a missing grant, which mean the code set the wrong
// scope (ADR 0009), and every connection or server failure, which must never
// look like a missing row or a bad token (ADR 0013).
func mapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%w: %w", shared.ErrNotFound, err)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.Code {
	case "23505":
		return fmt.Errorf("%w: %w", shared.ErrConflict, err)
	case "40001", "40P01":
		return fmt.Errorf("retryable %w: %w", shared.ErrConflict, err)
	case "23503":
		return fmt.Errorf("invalid reference: %w: %w", shared.ErrInvalid, err)
	case "23514", "23502":
		return fmt.Errorf("%w: %w", shared.ErrInvalid, err)
	default:
		return err
	}
}
