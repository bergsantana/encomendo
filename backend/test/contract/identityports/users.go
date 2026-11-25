package identityports

import (
	"context"
	"errors"
	"testing"

	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// UsersHarness is a Users port with the TxManager it runs in. Each harness
// starts with no accounts the suite could collide with; for Postgres, use a
// fresh suffix in the emails the suite builds (NewEmail).
type UsersHarness struct {
	Users appidentity.Users
	Tx    appshared.TxManager
	// NewEmail returns an address no account uses yet, in lower case.
	NewEmail func(t *testing.T) string
	// NewID returns a fresh UUIDv7.
	NewID func() shared.ID
}

// NewUser builds an unsaved account with a fresh email.
func (h UsersHarness) NewUser(t *testing.T, hash identity.PasswordHash) *identity.User {
	t.Helper()
	name, _ := identity.ParsePersonName("Ana Souza")
	email, err := identity.ParseEmail(h.NewEmail(t))
	if err != nil {
		t.Fatal(err)
	}
	u, err := identity.NewUser(shared.UserID(h.NewID()), identity.Profile{Name: name, Email: email}, hash)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// in runs fn in one transaction with the zero scope: the identity tables
// are global.
func in(t *testing.T, tx appshared.TxManager, fn func(ctx context.Context) error) {
	t.Helper()
	if err := tx.WithinTx(context.Background(), fn); err != nil {
		t.Fatal(err)
	}
}

// RunUsersContract checks the Users port: accounts by id and by email in
// any case, unique emails in any case, optimistic password updates, and
// refusal outside a transaction.
func RunUsersContract(t *testing.T, newHarness func(t *testing.T) UsersHarness) {
	t.Run("stored accounts read back by id and by email in any case", func(t *testing.T) { usersReadBack(t, newHarness(t)) })
	t.Run("an email has one account, in any case", func(t *testing.T) { usersUniqueEmail(t, newHarness(t)) })
	t.Run("password updates are optimistic", func(t *testing.T) { usersUpdatePassword(t, newHarness(t)) })
	t.Run("unknown accounts are not found", func(t *testing.T) { usersNotFound(t, newHarness(t)) })
	t.Run("needs a transaction", func(t *testing.T) { usersNeedTx(t, newHarness(t)) })
}

func usersReadBack(t *testing.T, h UsersHarness) {
	u := h.NewUser(t, "hash-1")
	in(t, h.Tx, func(ctx context.Context) error { return h.Users.InsertUser(ctx, u) })
	upper, _ := identity.ParseEmail(" " + caseFlip(u.Email().String()) + " ")
	in(t, h.Tx, func(ctx context.Context) error {
		byID, err := h.Users.UserByID(ctx, u.ID())
		if err != nil || byID.Email().Key() != u.Email().Key() || byID.Name() != u.Name() || byID.PasswordHash() != "hash-1" || byID.Version() != 1 {
			t.Errorf("UserByID = %+v, %v", byID, err)
		}
		for _, lookup := range []func(context.Context, identity.Email) (*identity.User, error){h.Users.UserByEmail, h.Users.LockUserByEmail} {
			byEmail, err := lookup(ctx, upper)
			if err != nil || byEmail.ID() != u.ID() {
				t.Errorf("by email %q = %+v, %v", upper, byEmail, err)
			}
		}
		hash, err := h.Users.PasswordHashForShare(ctx, u.ID())
		if err != nil || hash != "hash-1" {
			t.Errorf("PasswordHashForShare = %q, %v", hash, err)
		}
		many, err := h.Users.UsersByID(ctx, []shared.UserID{u.ID(), shared.UserID(h.NewID())})
		if err != nil || len(many) != 1 || many[0].ID() != u.ID() {
			t.Errorf("UsersByID = %+v, %v; want the one that exists", many, err)
		}
		return nil
	})
}

func usersUniqueEmail(t *testing.T, h UsersHarness) {
	u := h.NewUser(t, "hash-1")
	in(t, h.Tx, func(ctx context.Context) error { return h.Users.InsertUser(ctx, u) })
	name, _ := identity.ParsePersonName("Outra")
	shouting, _ := identity.ParseEmail(caseFlip(u.Email().String()))
	twin, err := identity.NewUser(shared.UserID(h.NewID()), identity.Profile{Name: name, Email: shouting}, "hash-2")
	if err != nil {
		t.Fatal(err)
	}
	err = h.Tx.WithinTx(context.Background(), func(ctx context.Context) error { return h.Users.InsertUser(ctx, twin) })
	if !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a second account for %q: %v, want ErrConflict", shouting, err)
	}
}

// caseFlip upper-cases the ASCII letters of s.
func caseFlip(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - 'a' + 'A'
		}
	}
	return string(b)
}

func usersUpdatePassword(t *testing.T, h UsersHarness) {
	u := h.NewUser(t, "old")
	in(t, h.Tx, func(ctx context.Context) error { return h.Users.InsertUser(ctx, u) })
	stale := *u
	if err := u.ChangePassword("new"); err != nil {
		t.Fatal(err)
	}
	in(t, h.Tx, func(ctx context.Context) error { return h.Users.UpdatePassword(ctx, u) })
	in(t, h.Tx, func(ctx context.Context) error {
		got, err := h.Users.UserByID(ctx, u.ID())
		if err != nil || got.PasswordHash() != "new" || got.Version() != 2 {
			t.Errorf("after the update: %+v, %v; want the new hash at version 2", got, err)
		}
		return nil
	})
	if err := stale.ChangePassword("lost"); err != nil {
		t.Fatal(err)
	}
	err := h.Tx.WithinTx(context.Background(), func(ctx context.Context) error { return h.Users.UpdatePassword(ctx, &stale) })
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("a stale update: %v, want ErrVersionConflict", err)
	}
}

func usersNotFound(t *testing.T, h UsersHarness) {
	nobody, _ := identity.ParseEmail(h.NewEmail(t))
	in(t, h.Tx, func(ctx context.Context) error {
		if _, err := h.Users.UserByID(ctx, shared.UserID(h.NewID())); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("UserByID: %v", err)
		}
		if _, err := h.Users.UserByEmail(ctx, nobody); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("UserByEmail: %v", err)
		}
		if _, err := h.Users.LockUserByEmail(ctx, nobody); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("LockUserByEmail: %v", err)
		}
		if _, err := h.Users.PasswordHashForShare(ctx, shared.UserID(h.NewID())); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("PasswordHashForShare: %v", err)
		}
		return nil
	})
}

func usersNeedTx(t *testing.T, h UsersHarness) {
	if _, err := h.Users.UserByID(context.Background(), shared.UserID(h.NewID())); !errors.Is(err, appshared.ErrNoTransaction) {
		t.Errorf("outside a transaction: %v, want ErrNoTransaction", err)
	}
}

// RunPasswordHasherContract checks a PasswordHasher: salted hashes that
// verify only their password, the empty hash that never verifies, and an
// error, not a refusal, for an unreadable hash.
func RunPasswordHasherContract(t *testing.T, newHasher func(t *testing.T) appidentity.PasswordHasher) {
	h := newHasher(t)
	ctx := context.Background()
	a, err := h.Hash(ctx, "segredo-forte")
	if err != nil {
		t.Fatal(err)
	}
	b, err := h.Hash(ctx, "segredo-forte")
	if err != nil {
		t.Fatal(err)
	}
	if a == b || a == "" || a == "segredo-forte" {
		t.Errorf("hashes %q and %q: want two different salted hashes, neither the password", a, b)
	}
	for _, c := range []struct {
		hash     identity.PasswordHash
		password string
		want     bool
	}{{a, "segredo-forte", true}, {b, "segredo-forte", true}, {a, "segredo-fraco", false}, {"", "segredo-forte", false}, {"", "", false}} {
		ok, err := h.Verify(ctx, c.hash, c.password)
		if err != nil || ok != c.want {
			t.Errorf("Verify(%q, %q) = %v, %v; want %v", c.hash, c.password, ok, err, c.want)
		}
	}
	if _, err := h.Verify(ctx, "not a hash", "x"); err == nil {
		t.Error("an unreadable hash verified without an error")
	}
}
