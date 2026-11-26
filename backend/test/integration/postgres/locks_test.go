//go:build integration

package postgres_test

import (
	"context"
	"encoding/base64"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/inproc"
	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres"
	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

// blockedBy proves that a lock does its job: hold runs in one transaction
// that stays open, other runs in a second one, which must wait on table
// until the first commits, and then succeed. A repository method that drops
// its FOR UPDATE or FOR SHARE lets other through, and the test fails.
func blockedBy(t *testing.T, scope appshared.Scope, table string, hold, other func(ctx context.Context) error) {
	t.Helper()
	tx := postgres.NewTxManager(openApp(t))
	s := env.Seeder(t)
	ctx := appshared.WithScope(context.Background(), scope)
	held, release := make(chan struct{}), make(chan struct{})
	holdErr := make(chan error, 1)
	go func() {
		holdErr <- tx.WithinTx(ctx, func(ctx context.Context) error {
			if err := hold(ctx); err != nil {
				return err
			}
			close(held)
			<-release
			return nil
		})
	}()
	select {
	case <-held:
	case err := <-holdErr:
		t.Fatalf("the first transaction ended early: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("the first transaction never took its lock")
	}

	var otherDone atomic.Bool
	otherErr := make(chan error, 1)
	go func() {
		err := tx.WithinTx(ctx, other)
		otherDone.Store(true)
		otherErr <- err
	}()
	waitForLockWaiter(t, s, table)
	if otherDone.Load() {
		t.Fatal("the second transaction did not wait for the lock")
	}
	close(release)
	if err := <-holdErr; err != nil {
		t.Fatalf("first transaction: %v", err)
	}
	if err := <-otherErr; err != nil {
		t.Fatalf("second transaction: %v", err)
	}
}

func TestLockUserByEmail_MakesAnotherResetWait(t *testing.T) {
	t.Parallel()
	h := usersHarness(t)
	u := h.NewUser(t, "hash")
	if err := h.Tx.WithinTx(context.Background(), func(ctx context.Context) error { return h.Users.InsertUser(ctx, u) }); err != nil {
		t.Fatal(err)
	}
	lock := func(ctx context.Context) error { _, err := h.Users.LockUserByEmail(ctx, u.Email()); return err }

	blockedBy(t, appshared.Scope{}, "users", lock, lock)
}

// TestPasswordHashForShare_MakesAPasswordChangeWait: the login that verified
// a hash and then re-reads it FOR SHARE holds the row, so a reset's update
// waits, and the family the login creates is revoked by that reset (ADR
// 0013). Acceptance of an invitation uses the same read.
func TestPasswordHashForShare_MakesAPasswordChangeWait(t *testing.T) {
	t.Parallel()
	h := usersHarness(t)
	u := h.NewUser(t, "hash")
	if err := h.Tx.WithinTx(context.Background(), func(ctx context.Context) error { return h.Users.InsertUser(ctx, u) }); err != nil {
		t.Fatal(err)
	}
	if err := u.ChangePassword("new-hash"); err != nil {
		t.Fatal(err)
	}

	blockedBy(t, appshared.Scope{}, "users",
		func(ctx context.Context) error { _, err := h.Users.PasswordHashForShare(ctx, u.ID()); return err },
		func(ctx context.Context) error { return h.Users.UpdatePassword(ctx, u) })
}

// TestLockMembershipOf_MakesADemotionWait: team management reads the
// caller's role FOR SHARE, so a concurrent demotion or removal of the
// caller waits and the invitations it revokes include the caller's (ADR
// 0011).
func TestLockMembershipOf_MakesADemotionOrRemovalWait(t *testing.T) {
	t.Parallel()
	s := env.Seeder(t)
	members := postgres.NewMembers()

	for name, change := range map[string]func(ctx context.Context, store shared.TenantID, admin shared.UserID) error{
		"demotion": func(ctx context.Context, store shared.TenantID, admin shared.UserID) error {
			m, err := members.MembershipOf(ctx, store, admin)
			if err != nil {
				return err
			}
			if err := m.ChangeRole(shared.RoleViewer); err != nil {
				return err
			}
			return members.UpdateRole(ctx, m)
		},
		"removal": func(ctx context.Context, store shared.TenantID, admin shared.UserID) error {
			return members.DeleteMembership(ctx, store, admin)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			owner, admin := s.User(), s.User()
			store := s.Tenant(owner)
			s.Member(store, admin, shared.RoleAdmin)

			blockedBy(t, appshared.Scope{TenantID: store, UserID: owner}, "memberships",
				func(ctx context.Context) error { _, err := members.LockMembershipOf(ctx, store, admin); return err },
				func(ctx context.Context) error { return change(ctx, store, admin) })
		})
	}
}

// TestRevokeUserFamilies_WaitsForARotationInFlight: a password reset revokes
// every family of the account, locking them first. One whose rotation is
// still uncommitted makes it wait, and it then also returns the access token
// the rotation issued (ADR 0013).
func TestRevokeUserFamilies_WaitsForARotationInFlight(t *testing.T) {
	t.Parallel()
	h := refreshHarness(t)
	s := env.Seeder(t)
	now := h.Now
	user := s.User()
	fam, first := login(t, h, user, now.Add(-time.Minute))
	next := newToken(t, h, fam.ID, now)
	var jtis []shared.ID

	blockedBy(t, appshared.Scope{}, "refresh_families",
		func(ctx context.Context) error {
			if _, err := h.Refresh.LockFamily(ctx, fam.ID); err != nil {
				return err
			}
			if won, err := h.Refresh.MarkRefreshTokenUsed(ctx, first.ID, now); err != nil || !won {
				t.Errorf("MarkRefreshTokenUsed = %v, %v", won, err)
			}
			if err := h.Refresh.InsertRefreshToken(ctx, next); err != nil {
				return err
			}
			return h.Refresh.SetSuccessor(ctx, first.ID, next.ID)
		},
		func(ctx context.Context) error {
			var err error
			jtis, err = h.Refresh.RevokeUserFamilies(ctx, user, now.Add(time.Second), now.Add(-time.Hour))
			return err
		})

	for _, want := range []shared.ID{first.AccessJTI, next.AccessJTI} {
		if !slices.Contains(jtis, want) {
			t.Errorf("RevokeUserFamilies returned %v; want %v among them", jtis, want)
		}
	}
}

// TestDefaultWorkflow_IsTheSeededPresetSettingsJoinOn: CreateTenant takes the
// default preset from here, and SettingsOf joins on the same id and version
// for its name.
func TestDefaultWorkflow_IsTheSeededPresetSettingsJoinOn(t *testing.T) {
	t.Parallel()
	s := env.Seeder(t)
	tx := postgres.NewTxManager(openApp(t))
	store := s.Tenant(s.User())
	err := tx.WithinTx(appshared.WithScope(context.Background(), appshared.Scope{TenantID: store}), func(ctx context.Context) error {
		ref, err := postgres.NewWorkflowPresets().DefaultWorkflow(ctx)
		if err != nil {
			return err
		}
		settings, err := postgres.NewTenants().SettingsOf(ctx, store)
		if err != nil {
			return err
		}
		if ref.IsZero() || ref.Name == "" || ref != settings.Workflow() {
			t.Errorf("DefaultWorkflow = %+v; the seeded store's settings point at %+v", ref, settings.Workflow())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestTenancyUsers_ReadsAccountsThroughIdentity covers the in-process
// directory tenancy uses to find an account by email or id.
func TestTenancyUsers_ReadsAccountsThroughIdentity(t *testing.T) {
	t.Parallel()
	h := usersHarness(t)
	u := h.NewUser(t, "hash")
	if err := h.Tx.WithinTx(context.Background(), func(ctx context.Context) error { return h.Users.InsertUser(ctx, u) }); err != nil {
		t.Fatal(err)
	}
	dir := inproc.NewTenancyUsers(appidentity.NewDirectory(h.Users))
	upper, err := identity.ParseEmail(upperCase(u.Email().String()))
	if err != nil {
		t.Fatal(err)
	}
	stranger, err := identity.ParseEmail("ninguem-" + suffix() + "@teste.encomendo.com.br")
	if err != nil {
		t.Fatal(err)
	}

	err = h.Tx.WithinTx(context.Background(), func(ctx context.Context) error {
		got, err := dir.UserByEmail(ctx, upper)
		if err != nil || got.ID != u.ID() || got.Email != u.Email().String() || got.Name != u.Name().String() {
			t.Errorf("UserByEmail in another case = %+v, %v; want the account", got, err)
		}
		if _, err := dir.UserByEmail(ctx, stranger); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("UserByEmail of a stranger: %v; want ErrNotFound", err)
		}
		many, err := dir.UsersByID(ctx, []shared.UserID{u.ID(), shared.UserID(pgtest.NewID())})
		if err != nil || len(many) != 1 || many[0].ID != u.ID() {
			t.Errorf("UsersByID = %+v, %v; want only the account that exists", many, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func b64(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }

func upperCase(s string) string {
	out := []byte(s)
	for i, c := range out {
		if c >= 'a' && c <= 'z' {
			out[i] = c - 32
		}
	}
	return string(out)
}

// TestKeysetCursors_AForgedCursorIsA400: a cursor the repository did not
// issue is a validation error, whatever it holds, never a database error.
func TestKeysetCursors_AForgedCursorIsABadRequest(t *testing.T) {
	t.Parallel()
	s := env.Seeder(t)
	tx := postgres.NewTxManager(openApp(t))
	store := s.Tenant(s.User())
	id := pgtest.NewID().String()
	forged := map[string]string{
		"not base64":              "%%%",
		"no separator":            "MTIz",
		"micros not a number":     b64("abc." + id),
		"before 4713 BC":          b64("-210866803200000001." + id),
		"after the year 9999":     b64("253402300800000000." + id),
		"min int64":               b64("-9223372036854775808." + id),
		"max int64":               b64("9223372036854775807." + id),
		"non-canonical digits":    b64("0005." + id),
		"a plus sign":             b64("+5." + id),
		"an id that is not an id": b64("5.hello"),
	}
	for name, cursor := range forged {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			page, err := shared.NewPageRequest(cursor, 10)
			if err != nil {
				t.Fatal(err)
			}
			err = tx.WithinTx(appshared.WithScope(context.Background(), appshared.Scope{TenantID: store}), func(ctx context.Context) error {
				if _, err := postgres.NewMembers().Memberships(ctx, store, page); !errors.Is(err, shared.ErrInvalid) {
					t.Errorf("Memberships: %v; want a validation error", err)
				}
				if _, err := postgres.NewInvitations().PendingInvitations(ctx, store, time.Now(), page); !errors.Is(err, shared.ErrInvalid) {
					t.Errorf("PendingInvitations: %v; want a validation error", err)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestRefreshTokens_NeverTouchAClientFamily: the tables also hold client
// families from milestone 7. A staff code path handed one of their ids must
// change nothing and find nothing, whatever the cause.
func TestRefreshTokens_NeverTouchAClientFamily(t *testing.T) {
	t.Parallel()
	h := refreshHarness(t)
	s := env.Seeder(t)
	family, token := pgtest.NewID().String(), pgtest.NewID().String()
	if _, err := s.DB().ExecContext(t.Context(), `INSERT INTO refresh_families (id, audience, client_account_id) VALUES ($1, 'client', $2)`,
		family, s.ClientAccount().String()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().ExecContext(t.Context(),
		`INSERT INTO refresh_tokens (id, family_id, audience, token_hash, access_jti, expires_at) VALUES ($1, $2, 'client', $3, $4, now() + interval '1 day')`,
		token, family, pgtest.RandomHash(), pgtest.NewID().String()); err != nil {
		t.Fatal(err)
	}
	famID, _ := shared.ParseID(family)
	tokID, _ := shared.ParseID(token)
	succ := newToken(t, h, identity.FamilyID(famID), h.Now)

	err := h.Tx.WithinTx(context.Background(), func(ctx context.Context) error {
		if ok, err := h.Refresh.MarkRefreshTokenUsed(ctx, identity.RefreshTokenID(tokID), h.Now); ok || err != nil {
			t.Errorf("MarkRefreshTokenUsed = %v, %v; want false", ok, err)
		}
		if ok, err := h.Refresh.SupersedeRefreshToken(ctx, identity.RefreshTokenID(tokID), h.Now); ok || err != nil {
			t.Errorf("SupersedeRefreshToken = %v, %v; want false", ok, err)
		}
		if err := h.Refresh.SetSuccessor(ctx, identity.RefreshTokenID(tokID), succ.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("SetSuccessor: %v; want ErrNotFound", err)
		}
		if jtis, err := h.Refresh.RevokeFamily(ctx, identity.FamilyID(famID), h.Now, h.Now.Add(-time.Hour)); err != nil || len(jtis) != 0 {
			t.Errorf("RevokeFamily = %v, %v; want nothing", jtis, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var revoked, used bool
	if err := s.DB().QueryRowContext(t.Context(),
		`SELECT f.revoked_at IS NOT NULL, t.used_at IS NOT NULL FROM refresh_families f JOIN refresh_tokens t ON t.family_id = f.id WHERE f.id = $1`,
		family).Scan(&revoked, &used); err != nil {
		t.Fatal(err)
	}
	if revoked || used {
		t.Errorf("a client family was changed by staff code: revoked %v, used %v", revoked, used)
	}
}
