package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func TestMapError_TurnsPostgresCodesIntoSentinels(t *testing.T) {
	sentinels := []error{shared.ErrNotFound, shared.ErrConflict, shared.ErrInvalid, shared.ErrForbidden}
	cases := []struct {
		name string
		in   error
		want error // nil: matches no sentinel
	}{
		{"missing row", gorm.ErrRecordNotFound, shared.ErrNotFound},
		{"unique violation", &pgconn.PgError{Code: "23505"}, shared.ErrConflict},
		{"serialization failure", &pgconn.PgError{Code: "40001"}, shared.ErrConflict},
		{"deadlock", &pgconn.PgError{Code: "40P01"}, shared.ErrConflict},
		{"foreign key violation", &pgconn.PgError{Code: "23503"}, shared.ErrInvalid},
		// The domain validates first, so these mean a bug: a logged 500,
		// never a 400 that would end a session on /auth/refresh (ADR 0013).
		{"check violation", &pgconn.PgError{Code: "23514"}, nil},
		{"not null violation", &pgconn.PgError{Code: "23502"}, nil},
		{"wrapped unique violation", fmt.Errorf("insert user: %w", &pgconn.PgError{Code: "23505"}), shared.ErrConflict},
		// A row refused by a policy, or a missing grant, means the code above
		// set the wrong scope: a bug, so it stays a logged 500 (ADR 0009).
		{"row level security", &pgconn.PgError{Code: "42501"}, nil},
		// A failing database is never "not found" or "unauthenticated": the
		// API must answer 5xx, so the apps keep their sessions (ADR 0013).
		{"connection failure", errors.New("dial tcp: connection refused"), nil},
		{"canceled", context.Canceled, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := mapError(c.in)
			for _, s := range sentinels {
				if errors.Is(got, s) != errors.Is(s, c.want) {
					t.Errorf("mapError(%v) = %v; errors.Is(%v) = %v", c.in, got, s, errors.Is(got, s))
				}
			}
			if !errors.Is(got, c.in) {
				t.Errorf("mapError(%v) = %v, which no longer wraps the original", c.in, got)
			}
		})
	}
}

func TestMapError_KeepsNil(t *testing.T) {
	if err := mapError(nil); err != nil {
		t.Errorf("mapError(nil) = %v, want nil", err)
	}
}

func TestMapError_KeepsThePostgresDetail(t *testing.T) {
	in := &pgconn.PgError{Code: "23505", ConstraintName: "users_email_key"}
	var pgErr *pgconn.PgError
	if !errors.As(mapError(in), &pgErr) || pgErr.ConstraintName != "users_email_key" {
		t.Error("the mapped error lost the *pgconn.PgError, which repositories read to tell constraints apart")
	}
}
