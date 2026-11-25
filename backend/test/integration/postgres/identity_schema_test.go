//go:build integration

package postgres_test

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

// TestRefreshTokens_KeysKeepFamiliesApart checks the keys ADR 0013 relies on:
// a token's audience is its family's, a client token never has a store, and
// a token can only be replaced by one of its own family.
func TestRefreshTokens_KeysKeepFamiliesApart(t *testing.T) {
	t.Parallel()
	s := env.Seeder(t)
	db := s.DB()
	ctx := t.Context()
	user := s.User()
	store := s.Tenant(user)
	account := s.ClientAccount()

	staff, client, other := pgtest.NewID().String(), pgtest.NewID().String(), pgtest.NewID().String()
	for _, f := range []struct{ id, audience, owner, column string }{
		{staff, "backoffice", user.String(), "user_id"},
		{other, "backoffice", user.String(), "user_id"},
		{client, "client", account.String(), "client_account_id"},
	} {
		if _, err := db.ExecContext(ctx, `INSERT INTO refresh_families (id, audience, `+f.column+`) VALUES ($1, $2, $3)`,
			f.id, f.audience, f.owner); err != nil {
			t.Fatal(err)
		}
	}
	insert := func(id, family, audience string, tenant any) error {
		_, err := db.ExecContext(ctx, `INSERT INTO refresh_tokens
			(id, family_id, audience, tenant_id, token_hash, access_jti, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			id, family, audience, tenant, pgtest.RandomHash(), pgtest.NewID().String(), time.Now().Add(time.Hour))
		return err
	}
	first, foreign := pgtest.NewID().String(), pgtest.NewID().String()
	if err := insert(first, staff, "backoffice", store.String()); err != nil {
		t.Fatalf("a staff token with its store: %v", err)
	}
	if err := insert(foreign, other, "backoffice", nil); err != nil {
		t.Fatal(err)
	}

	cases := map[string]struct {
		err  error
		code string
	}{
		"client token with a store":  {insert(pgtest.NewID().String(), client, "client", store.String()), "23514"},
		"audience unlike its family": {insert(pgtest.NewID().String(), staff, "client", nil), "23503"},
		"successor from another family": {func() error {
			_, err := db.ExecContext(ctx, `UPDATE refresh_tokens SET used_at = now(), replaced_by = $1 WHERE id = $2`, foreign, first)
			return err
		}(), "23503"},
	}
	for name, c := range cases {
		var pgErr *pgconn.PgError
		if !errors.As(c.err, &pgErr) || pgErr.Code != c.code {
			t.Errorf("%s: err = %v, want SQLSTATE %s", name, c.err, c.code)
		}
	}
}
