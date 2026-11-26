//go:build integration

package postgres_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres"
	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/test/contract/identityports"
	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

// The lock protocol of ADR 0013, against the real locks. Fakes cannot show
// these: they need two transactions in flight at once.

// waitForLockWaiter returns once another session waits for a lock in a
// statement that mentions table, so a test knows its second transaction has
// reached the lock. The package's tests share one database, so the filter
// keeps another test's waiter from releasing this one early.
func waitForLockWaiter(t *testing.T, s *pgtest.Seeder, table string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		if err := s.DB().QueryRowContext(t.Context(),
			`SELECT count(*) FROM pg_stat_activity
			 WHERE datname = current_database() AND pid <> pg_backend_pid()
			   AND wait_event_type = 'Lock' AND query ILIKE '%' || $1 || '%'`, table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no transaction ever waited for a lock on %s", table)
}

// TestRevokeFamily_DuringARotationAlsoDenylistsTheRotatedAccessToken: a
// revoke that waits for the family lock reads the jtis in a statement that
// starts after the rotation committed. A single statement would read with
// the snapshot it took before waiting, and miss the new token's jti.
func TestRevokeFamily_DuringARotationAlsoDenylistsTheRotatedAccessToken(t *testing.T) {
	t.Parallel()
	h := refreshHarness(t)
	s := env.Seeder(t)
	now := h.Now
	fam, first := login(t, h, s.User(), now.Add(-time.Minute))
	next := newToken(t, h, fam.ID, now)

	rotated := make(chan struct{})
	revoked := make(chan []shared.ID, 1)
	revokeErr := make(chan error, 1)
	go func() {
		<-rotated
		var jtis []shared.ID
		err := h.Tx.WithinTx(context.Background(), func(ctx context.Context) error {
			if _, err := h.Refresh.LockFamily(ctx, fam.ID); err != nil { // waits for the rotation
				return err
			}
			var err error
			jtis, err = h.Refresh.RevokeFamily(ctx, fam.ID, now.Add(time.Second), now.Add(-time.Hour))
			return err
		})
		revokeErr <- err
		revoked <- jtis
	}()

	err := h.Tx.WithinTx(context.Background(), func(ctx context.Context) error {
		if _, err := h.Refresh.LockFamily(ctx, fam.ID); err != nil {
			return err
		}
		if won, err := h.Refresh.MarkRefreshTokenUsed(ctx, first.ID, now); err != nil || !won {
			t.Errorf("MarkRefreshTokenUsed = %v, %v", won, err)
		}
		if err := h.Refresh.InsertRefreshToken(ctx, next); err != nil {
			return err
		}
		if err := h.Refresh.SetSuccessor(ctx, first.ID, next.ID); err != nil {
			return err
		}
		close(rotated)
		waitForLockWaiter(t, s, "refresh_families") // the revoke is now waiting for this transaction
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := <-revokeErr; err != nil {
		t.Fatalf("revoke: %v", err)
	}
	jtis := <-revoked
	for _, want := range []shared.ID{first.AccessJTI, next.AccessJTI} {
		if !slices.Contains(jtis, want) {
			t.Errorf("revoke denylisted %v; want %v among them: the rotated token's access token must stop too", jtis, want)
		}
	}
}

var _ appidentity.RefreshTokens = (*postgres.RefreshTokens)(nil)

// login stores a family of user with one token created at created.
func login(t *testing.T, h identityports.RefreshHarness, user shared.UserID, created time.Time) (identity.RefreshFamily, identity.RefreshToken) {
	t.Helper()
	fam, err := identity.NewRefreshFamily(identity.FamilyID(h.NewID()), user)
	if err != nil {
		t.Fatal(err)
	}
	tok := newToken(t, h, fam.ID, created)
	if err := h.Tx.WithinTx(context.Background(), func(ctx context.Context) error {
		if err := h.Refresh.CreateFamily(ctx, fam); err != nil {
			return err
		}
		return h.Refresh.InsertRefreshToken(ctx, tok)
	}); err != nil {
		t.Fatal(err)
	}
	return fam, tok
}

func newToken(t *testing.T, h identityports.RefreshHarness, fam identity.FamilyID, created time.Time) identity.RefreshToken {
	t.Helper()
	var secret shared.Secret
	id := h.NewID()
	copy(secret[:], id[:])
	copy(secret[16:], h.NewID().String())
	tok, err := identity.NewRefreshToken(identity.RefreshTokenSpec{
		ID: identity.RefreshTokenID(id), Family: fam, Hash: secret.Hash(), AccessJTI: h.NewID(),
	}, created)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}
