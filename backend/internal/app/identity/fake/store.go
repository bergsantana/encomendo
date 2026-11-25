package fake

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Store is an in-memory Users, RefreshTokens and ResetCodes, with the
// database's keys: unique emails (any case) and token hashes, tokens only in
// existing families, successors only in the token's own family. Like
// adapter/postgres it refuses calls outside a fake transaction, and a
// failed transaction undoes its writes (sharedfake.KeepOnRollback; ADR
// 0009).
//
// Fail and OnCall let a test break one method or slip a change in before
// it, which is how the conditional updates of ADR 0013 are exercised.
type Store struct {
	mu       sync.Mutex
	users    map[shared.UserID]identity.UserRecord
	families map[identity.FamilyID]identity.RefreshFamily
	tokens   map[identity.RefreshTokenID]identity.RefreshToken
	codes    map[identity.ResetCodeID]identity.PasswordResetCode
	faults   map[string]error
	hooks    map[string]func()
	calls    []string
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{
		users:    map[shared.UserID]identity.UserRecord{},
		families: map[identity.FamilyID]identity.RefreshFamily{},
		tokens:   map[identity.RefreshTokenID]identity.RefreshToken{},
		codes:    map[identity.ResetCodeID]identity.PasswordResetCode{},
		faults:   map[string]error{},
		hooks:    map[string]func(){},
	}
}

// Fail makes every later call of method return err; nil heals it.
func (s *Store) Fail(method string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.faults[method] = err
}

// OnCall runs fn before every later call of method, outside the store's
// lock, so fn may change the store as a concurrent request would.
func (s *Store) OnCall(method string, fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hooks[method] = fn
}

// Calls returns the names of the methods called so far, oldest first.
func (s *Store) Calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.calls)
}

// enter runs the hook, records the call and checks the transaction and the
// fault for method. On success it returns with s.mu held.
func (s *Store) enter(ctx context.Context, method string) error {
	s.mu.Lock()
	hook := s.hooks[method]
	s.mu.Unlock()
	if hook != nil {
		hook()
	}
	s.mu.Lock()
	s.calls = append(s.calls, method)
	if !sharedfake.InTx(ctx) {
		s.mu.Unlock()
		return appshared.ErrNoTransaction
	}
	if err := s.faults[method]; err != nil {
		s.mu.Unlock()
		return err
	}
	return nil
}

// ---- Users ----

// InsertUser implements Users.
func (s *Store) InsertUser(ctx context.Context, u *identity.User) error {
	if err := s.enter(ctx, "InsertUser"); err != nil {
		return err
	}
	defer s.mu.Unlock()
	for _, r := range s.users {
		if r.ID == u.ID() {
			return fmt.Errorf("users_pkey: %w", shared.ErrConflict)
		}
		if email, _ := identity.ParseEmail(r.Email); email.Equal(u.Email()) {
			return fmt.Errorf("users_email_key: %w", shared.ErrConflict)
		}
	}
	sharedfake.KeepOnRollback(ctx, &s.mu, s.users, u.ID())
	s.users[u.ID()] = identity.UserRecord{
		ID: u.ID(), Name: u.Name().String(), Email: u.Email().String(), PasswordHash: u.PasswordHash(), Version: u.Version(),
	}
	return nil
}

// UserByID implements Users.
func (s *Store) UserByID(ctx context.Context, id shared.UserID) (*identity.User, error) {
	if err := s.enter(ctx, "UserByID"); err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	r, ok := s.users[id]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return identity.RestoreUser(r)
}

// UsersByID implements Users.
func (s *Store) UsersByID(ctx context.Context, ids []shared.UserID) ([]*identity.User, error) {
	if err := s.enter(ctx, "UsersByID"); err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	var out []*identity.User
	for _, id := range ids {
		if r, ok := s.users[id]; ok {
			u, err := identity.RestoreUser(r)
			if err != nil {
				return nil, err
			}
			out = append(out, u)
		}
	}
	return out, nil
}

// UserByEmail implements Users.
func (s *Store) UserByEmail(ctx context.Context, email identity.Email) (*identity.User, error) {
	if err := s.enter(ctx, "UserByEmail"); err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	return s.userByEmail(email)
}

// LockUserByEmail implements Users. The fake has nothing to lock; the call
// is recorded so tests can check the order.
func (s *Store) LockUserByEmail(ctx context.Context, email identity.Email) (*identity.User, error) {
	if err := s.enter(ctx, "LockUserByEmail"); err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	return s.userByEmail(email)
}

func (s *Store) userByEmail(email identity.Email) (*identity.User, error) {
	for _, r := range s.users {
		if stored, _ := identity.ParseEmail(r.Email); stored.Equal(email) {
			return identity.RestoreUser(r)
		}
	}
	return nil, shared.ErrNotFound
}

// PasswordHashForShare implements Users.
func (s *Store) PasswordHashForShare(ctx context.Context, id shared.UserID) (identity.PasswordHash, error) {
	if err := s.enter(ctx, "PasswordHashForShare"); err != nil {
		return "", err
	}
	defer s.mu.Unlock()
	r, ok := s.users[id]
	if !ok {
		return "", shared.ErrNotFound
	}
	return r.PasswordHash, nil
}

// UpdatePassword implements Users.
func (s *Store) UpdatePassword(ctx context.Context, u *identity.User) error {
	if err := s.enter(ctx, "UpdatePassword"); err != nil {
		return err
	}
	defer s.mu.Unlock()
	r, ok := s.users[u.ID()]
	if !ok || r.Version != u.Version() {
		return shared.ErrVersionConflict
	}
	sharedfake.KeepOnRollback(ctx, &s.mu, s.users, u.ID())
	r.PasswordHash, r.Version = u.PasswordHash(), r.Version+1
	s.users[u.ID()] = r
	return nil
}

// User returns the stored account, for assertions outside a transaction.
func (s *Store) User(id shared.UserID) (identity.UserRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.users[id]
	return r, ok
}

// ---- ResetCodes ----

// InsertResetCode implements ResetCodes.
func (s *Store) InsertResetCode(ctx context.Context, c identity.PasswordResetCode) error {
	if err := s.enter(ctx, "InsertResetCode"); err != nil {
		return err
	}
	defer s.mu.Unlock()
	if _, ok := s.users[c.User]; !ok {
		return fmt.Errorf("password_reset_codes_user_id_fkey: %w", shared.ErrInvalid)
	}
	sharedfake.KeepOnRollback(ctx, &s.mu, s.codes, c.ID)
	s.codes[c.ID] = c
	return nil
}

// NewestResetCode implements ResetCodes.
func (s *Store) NewestResetCode(ctx context.Context, user shared.UserID) (identity.PasswordResetCode, error) {
	if err := s.enter(ctx, "NewestResetCode"); err != nil {
		return identity.PasswordResetCode{}, err
	}
	defer s.mu.Unlock()
	var newest identity.PasswordResetCode
	found := false
	for _, c := range s.codes {
		if c.User != user {
			continue
		}
		if !found || c.CreatedAt.After(newest.CreatedAt) ||
			(c.CreatedAt.Equal(newest.CreatedAt) && shared.ID(c.ID).String() > shared.ID(newest.ID).String()) {
			newest, found = c, true
		}
	}
	if !found {
		return identity.PasswordResetCode{}, shared.ErrNotFound
	}
	return newest, nil
}

// WrongResetAttemptsSince implements ResetCodes.
func (s *Store) WrongResetAttemptsSince(ctx context.Context, user shared.UserID, since time.Time) (int, error) {
	if err := s.enter(ctx, "WrongResetAttemptsSince"); err != nil {
		return 0, err
	}
	defer s.mu.Unlock()
	n := 0
	for _, c := range s.codes {
		if c.User == user && c.CreatedAt.After(since) {
			n += c.Attempts
		}
	}
	return n, nil
}

// AddWrongResetAttempt implements ResetCodes.
func (s *Store) AddWrongResetAttempt(ctx context.Context, id identity.ResetCodeID) (bool, error) {
	if err := s.enter(ctx, "AddWrongResetAttempt"); err != nil {
		return false, err
	}
	defer s.mu.Unlock()
	c, ok := s.codes[id]
	if !ok || c.Attempts >= identity.MaxResetAttempts {
		return false, nil
	}
	sharedfake.KeepOnRollback(ctx, &s.mu, s.codes, id)
	c.Attempts++
	s.codes[id] = c
	return true, nil
}

// MarkResetCodeUsed implements ResetCodes.
func (s *Store) MarkResetCodeUsed(ctx context.Context, id identity.ResetCodeID, at time.Time) (bool, error) {
	if err := s.enter(ctx, "MarkResetCodeUsed"); err != nil {
		return false, err
	}
	defer s.mu.Unlock()
	c, ok := s.codes[id]
	if !ok || !c.UsableAt(at) {
		return false, nil
	}
	sharedfake.KeepOnRollback(ctx, &s.mu, s.codes, id)
	c.UsedAt = at
	s.codes[id] = c
	return true, nil
}

// ResetCodes returns every stored code, for assertions.
func (s *Store) ResetCodes() []identity.PasswordResetCode {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]identity.PasswordResetCode, 0, len(s.codes))
	for _, c := range s.codes {
		out = append(out, c)
	}
	return out
}
