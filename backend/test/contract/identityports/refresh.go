package identityports

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// RefreshHarness is a RefreshTokens and ResetCodes port with the Users port
// that stores the accounts they belong to.
type RefreshHarness struct {
	UsersHarness
	Refresh    appidentity.RefreshTokens
	ResetCodes appidentity.ResetCodes
	// Now is a recent time, at the store's precision (microseconds for
	// Postgres); the suite builds every timestamp from it and compares
	// them exactly.
	Now time.Time
	// Tenant is an existing store a staff token may be issued for.
	Tenant shared.TenantID
}

func (h RefreshHarness) account(t *testing.T) shared.UserID {
	t.Helper()
	u := h.NewUser(t, "hash")
	in(t, h.Tx, func(ctx context.Context) error { return h.Users.InsertUser(ctx, u) })
	return u.ID()
}

// login stores a family of user with one token created at created, and
// returns both.
func (h RefreshHarness) login(t *testing.T, user shared.UserID, created time.Time) (identity.RefreshFamily, identity.RefreshToken) {
	t.Helper()
	fam, err := identity.NewRefreshFamily(identity.FamilyID(h.NewID()), user)
	if err != nil {
		t.Fatal(err)
	}
	tok := h.token(t, fam.ID, created)
	in(t, h.Tx, func(ctx context.Context) error {
		if err := h.Refresh.CreateFamily(ctx, fam); err != nil {
			return err
		}
		return h.Refresh.InsertRefreshToken(ctx, tok)
	})
	return fam, tok
}

func (h RefreshHarness) token(t *testing.T, fam identity.FamilyID, created time.Time) identity.RefreshToken {
	t.Helper()
	var secret shared.Secret
	id := h.NewID()
	copy(secret[:], id[:])
	copy(secret[16:], h.NewID().String())
	tok, err := identity.NewRefreshToken(identity.RefreshTokenSpec{
		ID: identity.RefreshTokenID(h.NewID()), Family: fam, Tenant: h.Tenant, Hash: secret.Hash(), AccessJTI: h.NewID(),
	}, created)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func (h RefreshHarness) insert(t *testing.T, tok identity.RefreshToken) {
	t.Helper()
	in(t, h.Tx, func(ctx context.Context) error { return h.Refresh.InsertRefreshToken(ctx, tok) })
}

// RunRefreshTokensContract checks the RefreshTokens port: tokens found by
// hash with their family, the conditional updates the lock protocol relies
// on, successors kept in their family, and revokes that report the recent
// access tokens (ADR 0013). Taking the locks themselves is checked against
// Postgres with concurrent transactions.
func RunRefreshTokensContract(t *testing.T, newHarness func(t *testing.T) RefreshHarness) {
	t.Run("a token reads back by hash with its family, and by id", func(t *testing.T) { refreshReadBack(t, newHarness(t)) })
	t.Run("marking used is conditional", func(t *testing.T) { refreshConditional(t, newHarness(t)) })
	t.Run("superseding is conditional", func(t *testing.T) { refreshConditionalSupersede(t, newHarness(t)) })
	t.Run("a successor stays in its family", func(t *testing.T) { refreshSuccessor(t, newHarness(t)) })
	t.Run("revoking reports the recent access tokens", func(t *testing.T) { refreshRevoke(t, newHarness(t)) })
	t.Run("revoking a user's families leaves others alone", func(t *testing.T) { refreshRevokeUser(t, newHarness(t)) })
	t.Run("unknown tokens and families are not found", func(t *testing.T) { refreshNotFound(t, newHarness(t)) })
	t.Run("a hash is stored once", func(t *testing.T) { refreshUniqueHash(t, newHarness(t)) })
}

func refreshReadBack(t *testing.T, h RefreshHarness) {
	user := h.account(t)
	fam, tok := h.login(t, user, h.Now)
	in(t, h.Tx, func(ctx context.Context) error {
		got, gotFam, err := h.Refresh.RefreshTokenByHash(ctx, tok.Hash)
		if err != nil || got.ID != tok.ID || got.Tenant != h.Tenant || got.AccessJTI != tok.AccessJTI || !got.ExpiresAt.Equal(tok.ExpiresAt) || !got.CreatedAt.Equal(tok.CreatedAt) {
			t.Errorf("by hash = %+v, %v; want %+v", got, err, tok)
		}
		if gotFam.ID != fam.ID || gotFam.User != user || gotFam.Revoked() {
			t.Errorf("family = %+v", gotFam)
		}
		byID, err := h.Refresh.RefreshTokenByID(ctx, tok.ID)
		if err != nil || byID.Hash != tok.Hash || !byID.UsedAt.IsZero() {
			t.Errorf("by id = %+v, %v", byID, err)
		}
		locked, err := h.Refresh.LockFamily(ctx, fam.ID)
		if err != nil || locked.ID != fam.ID {
			t.Errorf("LockFamily = %+v, %v", locked, err)
		}
		return nil
	})
}

func refreshConditional(t *testing.T, h RefreshHarness) {
	user := h.account(t)
	_, tok := h.login(t, user, h.Now)
	at := h.Now.Add(time.Minute)
	in(t, h.Tx, func(ctx context.Context) error {
		for i, want := range []bool{true, false} {
			won, err := h.Refresh.MarkRefreshTokenUsed(ctx, tok.ID, at)
			if err != nil || won != want {
				t.Errorf("MarkRefreshTokenUsed #%d = %v, %v; want %v", i+1, won, err, want)
			}
		}
		if won, err := h.Refresh.SupersedeRefreshToken(ctx, tok.ID, at); err != nil || won {
			t.Errorf("superseding a used token = %v, %v; want false", won, err)
		}
		got, err := h.Refresh.RefreshTokenByID(ctx, tok.ID)
		if err != nil || !got.UsedAt.Equal(at) || !got.SupersededAt.IsZero() {
			t.Errorf("after the first use: %+v, %v", got, err)
		}
		return nil
	})
}

func refreshConditionalSupersede(t *testing.T, h RefreshHarness) {
	_, other := h.login(t, h.account(t), h.Now)
	at := h.Now.Add(time.Minute)
	in(t, h.Tx, func(ctx context.Context) error {
		for i, want := range []bool{true, false} {
			won, err := h.Refresh.SupersedeRefreshToken(ctx, other.ID, at)
			if err != nil || won != want {
				t.Errorf("SupersedeRefreshToken #%d = %v, %v; want %v", i+1, won, err, want)
			}
		}
		if won, err := h.Refresh.MarkRefreshTokenUsed(ctx, other.ID, at); err != nil || won {
			t.Errorf("using a superseded token = %v, %v; want false", won, err)
		}
		// UseAt reads SupersededAt; a store that drops it hides a retry's
		// victim, and theft detection with it (ADR 0013).
		got, err := h.Refresh.RefreshTokenByID(ctx, other.ID)
		if err != nil || !got.SupersededAt.Equal(at) || !got.UsedAt.IsZero() {
			t.Errorf("after superseding: %+v, %v; want SupersededAt %v and no use", got, err, at)
		}
		return nil
	})
}

func refreshSuccessor(t *testing.T, h RefreshHarness) {
	user := h.account(t)
	fam, tok := h.login(t, user, h.Now)
	next := h.token(t, fam.ID, h.Now.Add(time.Minute))
	h.insert(t, next)
	_, stranger := h.login(t, user, h.Now)
	in(t, h.Tx, func(ctx context.Context) error {
		if _, err := h.Refresh.MarkRefreshTokenUsed(ctx, tok.ID, h.Now.Add(time.Minute)); err != nil {
			return err
		}
		if err := h.Refresh.SetSuccessor(ctx, tok.ID, next.ID); err != nil {
			t.Errorf("SetSuccessor in the family: %v", err)
		}
		got, err := h.Refresh.RefreshTokenByID(ctx, tok.ID)
		if err != nil || got.ReplacedBy != next.ID {
			t.Errorf("ReplacedBy = %v, %v", got.ReplacedBy, err)
		}
		return nil
	})
	err := h.Tx.WithinTx(context.Background(), func(ctx context.Context) error {
		return h.Refresh.SetSuccessor(ctx, tok.ID, stranger.ID)
	})
	if err == nil {
		t.Error("a successor from another family was accepted")
	}
}

func refreshRevoke(t *testing.T, h RefreshHarness) {
	user := h.account(t)
	old := h.Now.Add(-time.Hour)
	fam, first := h.login(t, user, old)
	recent := h.token(t, fam.ID, h.Now.Add(-time.Minute))
	h.insert(t, recent)
	since := h.Now.Add(-identity.DenylistWindow)
	in(t, h.Tx, func(ctx context.Context) error {
		jtis, err := h.Refresh.RevokeFamily(ctx, fam.ID, h.Now, since)
		if err != nil || !slices.Equal(jtis, []shared.ID{recent.AccessJTI}) {
			t.Errorf("RevokeFamily = %v, %v; want only the recent token's jti, not %v", jtis, err, first.AccessJTI)
		}
		return nil
	})
	in(t, h.Tx, func(ctx context.Context) error {
		if _, err := h.Refresh.RevokeFamily(ctx, fam.ID, h.Now.Add(time.Hour), since); err != nil {
			return err
		}
		got, err := h.Refresh.LockFamily(ctx, fam.ID)
		if err != nil || !got.RevokedAt.Equal(h.Now) {
			t.Errorf("after two revokes: %+v, %v; want the first revoke's time", got, err)
		}
		return nil
	})
}

func refreshRevokeUser(t *testing.T, h RefreshHarness) {
	user, other := h.account(t), h.account(t)
	famA, tokA := h.login(t, user, h.Now)
	famB, tokB := h.login(t, user, h.Now)
	famC, _ := h.login(t, other, h.Now)
	in(t, h.Tx, func(ctx context.Context) error {
		jtis, err := h.Refresh.RevokeUserFamilies(ctx, user, h.Now, h.Now.Add(-identity.DenylistWindow))
		slices.SortFunc(jtis, func(a, b shared.ID) int { return compare(a.String(), b.String()) })
		want := []shared.ID{tokA.AccessJTI, tokB.AccessJTI}
		slices.SortFunc(want, func(a, b shared.ID) int { return compare(a.String(), b.String()) })
		if err != nil || !slices.Equal(jtis, want) {
			t.Errorf("RevokeUserFamilies = %v, %v; want %v", jtis, err, want)
		}
		for _, f := range []identity.FamilyID{famA.ID, famB.ID, famC.ID} {
			got, err := h.Refresh.LockFamily(ctx, f)
			if err != nil || got.Revoked() != (f != famC.ID) {
				t.Errorf("family %v revoked = %v, %v", f, got.Revoked(), err)
			}
		}
		return nil
	})
}

func compare(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func refreshNotFound(t *testing.T, h RefreshHarness) {
	in(t, h.Tx, func(ctx context.Context) error {
		if _, _, err := h.Refresh.RefreshTokenByHash(ctx, shared.Secret{1}.Hash()); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("RefreshTokenByHash: %v", err)
		}
		if _, err := h.Refresh.RefreshTokenByID(ctx, identity.RefreshTokenID(h.NewID())); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("RefreshTokenByID: %v", err)
		}
		if _, err := h.Refresh.LockFamily(ctx, identity.FamilyID(h.NewID())); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("LockFamily: %v", err)
		}
		return nil
	})
	if _, _, err := h.Refresh.RefreshTokenByHash(context.Background(), shared.Secret{1}.Hash()); !errors.Is(err, appshared.ErrNoTransaction) {
		t.Errorf("outside a transaction: %v, want ErrNoTransaction", err)
	}
}

func refreshUniqueHash(t *testing.T, h RefreshHarness) {
	user := h.account(t)
	fam, tok := h.login(t, user, h.Now)
	twin := h.token(t, fam.ID, h.Now)
	twin.Hash = tok.Hash
	err := h.Tx.WithinTx(context.Background(), func(ctx context.Context) error { return h.Refresh.InsertRefreshToken(ctx, twin) })
	if !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a second token with the same hash: %v, want ErrConflict", err)
	}
}

// RunResetCodesContract checks the ResetCodes port: the newest code wins,
// wrong attempts sum over a window, and the counting and using up are
// atomic and conditional (ADR 0011).
func RunResetCodesContract(t *testing.T, newHarness func(t *testing.T) RefreshHarness) {
	t.Run("the newest code wins and attempts sum over the window", func(t *testing.T) { resetNewest(t, newHarness(t)) })
	t.Run("wrong attempts stop at the limit", func(t *testing.T) { resetAttempts(t, newHarness(t)) })
	t.Run("a code is used up once, only while usable", func(t *testing.T) { resetUse(t, newHarness(t)) })
}

func (h RefreshHarness) code(t *testing.T, user shared.UserID, created time.Time, attempts int) identity.PasswordResetCode {
	t.Helper()
	c, err := identity.NewPasswordResetCode(identity.ResetCodeID(h.NewID()), user, identity.CodeHash{7, 7}, created)
	if err != nil {
		t.Fatal(err)
	}
	in(t, h.Tx, func(ctx context.Context) error {
		if err := h.ResetCodes.InsertResetCode(ctx, c); err != nil {
			return err
		}
		for range attempts {
			if _, err := h.ResetCodes.AddWrongResetAttempt(ctx, c.ID); err != nil {
				return err
			}
		}
		return nil
	})
	c.Attempts = attempts
	return c
}

func resetNewest(t *testing.T, h RefreshHarness) {
	user := h.account(t)
	h.code(t, user, h.Now.Add(-25*time.Hour), 5) // out of the window
	h.code(t, user, h.Now.Add(-2*time.Hour), 3)
	newest := h.code(t, user, h.Now.Add(-time.Minute), 1)
	codeless := h.account(t)
	in(t, h.Tx, func(ctx context.Context) error {
		got, err := h.ResetCodes.NewestResetCode(ctx, user)
		if err != nil || got.ID != newest.ID || got.Attempts != 1 || got.Hash != newest.Hash || !got.ExpiresAt.Equal(newest.ExpiresAt) {
			t.Errorf("NewestResetCode = %+v, %v; want %+v", got, err, newest)
		}
		n, err := h.ResetCodes.WrongResetAttemptsSince(ctx, user, h.Now.Add(-identity.ResetBudgetWindow))
		if err != nil || n != 4 {
			t.Errorf("WrongResetAttemptsSince = %d, %v; want 4", n, err)
		}
		if _, err := h.ResetCodes.NewestResetCode(ctx, codeless); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("no code: %v, want ErrNotFound", err)
		}
		return nil
	})
}

func resetAttempts(t *testing.T, h RefreshHarness) {
	c := h.code(t, h.account(t), h.Now, identity.MaxResetAttempts-1)
	in(t, h.Tx, func(ctx context.Context) error {
		for i, want := range []bool{true, false} {
			counted, err := h.ResetCodes.AddWrongResetAttempt(ctx, c.ID)
			if err != nil || counted != want {
				t.Errorf("attempt #%d = %v, %v; want %v", i+1, counted, err, want)
			}
		}
		if used, err := h.ResetCodes.MarkResetCodeUsed(ctx, c.ID, h.Now); err != nil || used {
			t.Errorf("using a code with 5 wrong tries = %v, %v; want false", used, err)
		}
		return nil
	})
}

func resetUse(t *testing.T, h RefreshHarness) {
	user := h.account(t)
	c := h.code(t, user, h.Now, 0)
	expired := h.code(t, user, h.Now.Add(-time.Hour), 0)
	in(t, h.Tx, func(ctx context.Context) error {
		for i, want := range []bool{true, false} {
			used, err := h.ResetCodes.MarkResetCodeUsed(ctx, c.ID, h.Now.Add(time.Minute))
			if err != nil || used != want {
				t.Errorf("use #%d = %v, %v; want %v", i+1, used, err, want)
			}
		}
		if used, err := h.ResetCodes.MarkResetCodeUsed(ctx, expired.ID, h.Now); err != nil || used {
			t.Errorf("using an expired code = %v, %v; want false", used, err)
		}
		return nil
	})
}
