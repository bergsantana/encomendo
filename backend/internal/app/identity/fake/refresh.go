package fake

import (
	"context"
	"fmt"
	"slices"
	"time"

	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// CreateFamily implements RefreshTokens.
func (s *Store) CreateFamily(ctx context.Context, f identity.RefreshFamily) error {
	if err := s.enter(ctx, "CreateFamily"); err != nil {
		return err
	}
	defer s.mu.Unlock()
	if _, ok := s.families[f.ID]; ok {
		return fmt.Errorf("refresh_families_pkey: %w", shared.ErrConflict)
	}
	if _, ok := s.users[f.User]; !ok {
		return fmt.Errorf("refresh_families_user_id_fkey: %w", shared.ErrInvalid)
	}
	sharedfake.KeepOnRollback(ctx, &s.mu, s.families, f.ID)
	s.families[f.ID] = f
	return nil
}

// InsertRefreshToken implements RefreshTokens.
func (s *Store) InsertRefreshToken(ctx context.Context, t identity.RefreshToken) error {
	if err := s.enter(ctx, "InsertRefreshToken"); err != nil {
		return err
	}
	defer s.mu.Unlock()
	if _, ok := s.families[t.Family]; !ok {
		return fmt.Errorf("refresh_tokens_family_id_audience_fkey: %w", shared.ErrInvalid)
	}
	for _, other := range s.tokens {
		if other.ID == t.ID || other.Hash == t.Hash {
			return fmt.Errorf("refresh_tokens unique key: %w", shared.ErrConflict)
		}
	}
	sharedfake.KeepOnRollback(ctx, &s.mu, s.tokens, t.ID)
	s.tokens[t.ID] = t
	return nil
}

// RefreshTokenByHash implements RefreshTokens.
func (s *Store) RefreshTokenByHash(ctx context.Context, hash shared.SecretHash) (identity.RefreshToken, identity.RefreshFamily, error) {
	if err := s.enter(ctx, "RefreshTokenByHash"); err != nil {
		return identity.RefreshToken{}, identity.RefreshFamily{}, err
	}
	defer s.mu.Unlock()
	for _, t := range s.tokens {
		if t.Hash == hash {
			return t, s.families[t.Family], nil
		}
	}
	return identity.RefreshToken{}, identity.RefreshFamily{}, shared.ErrNotFound
}

// RefreshTokenByID implements RefreshTokens.
func (s *Store) RefreshTokenByID(ctx context.Context, id identity.RefreshTokenID) (identity.RefreshToken, error) {
	if err := s.enter(ctx, "RefreshTokenByID"); err != nil {
		return identity.RefreshToken{}, err
	}
	defer s.mu.Unlock()
	t, ok := s.tokens[id]
	if !ok {
		return identity.RefreshToken{}, shared.ErrNotFound
	}
	return t, nil
}

// LockFamily implements RefreshTokens. Calls run one at a time anyway; the
// call is recorded so tests can check the protocol's order.
func (s *Store) LockFamily(ctx context.Context, id identity.FamilyID) (identity.RefreshFamily, error) {
	if err := s.enter(ctx, "LockFamily"); err != nil {
		return identity.RefreshFamily{}, err
	}
	defer s.mu.Unlock()
	f, ok := s.families[id]
	if !ok {
		return identity.RefreshFamily{}, shared.ErrNotFound
	}
	return f, nil
}

// MarkRefreshTokenUsed implements RefreshTokens.
func (s *Store) MarkRefreshTokenUsed(ctx context.Context, id identity.RefreshTokenID, at time.Time) (bool, error) {
	if err := s.enter(ctx, "MarkRefreshTokenUsed"); err != nil {
		return false, err
	}
	defer s.mu.Unlock()
	t, ok := s.tokens[id]
	if !ok || !t.UsedAt.IsZero() || !t.SupersededAt.IsZero() {
		return false, nil
	}
	sharedfake.KeepOnRollback(ctx, &s.mu, s.tokens, id)
	t.UsedAt = at
	s.tokens[id] = t
	return true, nil
}

// SupersedeRefreshToken implements RefreshTokens.
func (s *Store) SupersedeRefreshToken(ctx context.Context, id identity.RefreshTokenID, at time.Time) (bool, error) {
	if err := s.enter(ctx, "SupersedeRefreshToken"); err != nil {
		return false, err
	}
	defer s.mu.Unlock()
	t, ok := s.tokens[id]
	if !ok || !t.UsedAt.IsZero() || !t.SupersededAt.IsZero() {
		return false, nil
	}
	sharedfake.KeepOnRollback(ctx, &s.mu, s.tokens, id)
	t.SupersededAt = at
	s.tokens[id] = t
	return true, nil
}

// SetSuccessor implements RefreshTokens, with the keys of 0002_identity: the
// token must be used and next must be a token of the same family.
func (s *Store) SetSuccessor(ctx context.Context, id, next identity.RefreshTokenID) error {
	if err := s.enter(ctx, "SetSuccessor"); err != nil {
		return err
	}
	defer s.mu.Unlock()
	t, ok := s.tokens[id]
	if !ok {
		return shared.ErrNotFound
	}
	n, ok := s.tokens[next]
	if !ok || n.Family != t.Family {
		return fmt.Errorf("refresh_tokens_family_id_replaced_by_fkey: %w", shared.ErrInvalid)
	}
	if t.UsedAt.IsZero() {
		return fmt.Errorf("refresh_tokens: replaced_by needs used_at (check violation)")
	}
	sharedfake.KeepOnRollback(ctx, &s.mu, s.tokens, id)
	t.ReplacedBy = next
	s.tokens[id] = t
	return nil
}

// RevokeFamily implements RefreshTokens.
func (s *Store) RevokeFamily(ctx context.Context, id identity.FamilyID, at, issuedSince time.Time) ([]shared.ID, error) {
	if err := s.enter(ctx, "RevokeFamily"); err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	return s.revoke(ctx, func(f identity.RefreshFamily) bool { return f.ID == id }, at, issuedSince), nil
}

// RevokeUserFamilies implements RefreshTokens.
func (s *Store) RevokeUserFamilies(ctx context.Context, user shared.UserID, at, issuedSince time.Time) ([]shared.ID, error) {
	if err := s.enter(ctx, "RevokeUserFamilies"); err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	return s.revoke(ctx, func(f identity.RefreshFamily) bool { return f.User == user }, at, issuedSince), nil
}

func (s *Store) revoke(ctx context.Context, match func(identity.RefreshFamily) bool, at, issuedSince time.Time) []shared.ID {
	revoked := map[identity.FamilyID]bool{}
	for id, f := range s.families {
		if !match(f) {
			continue
		}
		if !f.Revoked() {
			sharedfake.KeepOnRollback(ctx, &s.mu, s.families, id)
			f.RevokedAt = at
			s.families[id] = f
		}
		revoked[id] = true
	}
	var jtis []shared.ID
	for _, t := range s.tokens {
		if revoked[t.Family] && t.CreatedAt.After(issuedSince) {
			jtis = append(jtis, t.AccessJTI)
		}
	}
	slices.SortFunc(jtis, compareIDs)
	return jtis
}

func compareIDs(a, b shared.ID) int {
	switch x, y := a.String(), b.String(); {
	case x < y:
		return -1
	case x > y:
		return 1
	default:
		return 0
	}
}

// RefreshToken returns a stored token, for assertions.
func (s *Store) RefreshToken(id identity.RefreshTokenID) (identity.RefreshToken, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tokens[id]
	return t, ok
}

// RefreshTokenWithHash returns the stored token with this hash, for
// assertions.
func (s *Store) RefreshTokenWithHash(hash shared.SecretHash) (identity.RefreshToken, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.tokens {
		if t.Hash == hash {
			return t, true
		}
	}
	return identity.RefreshToken{}, false
}

// Family returns a stored family, for assertions.
func (s *Store) Family(id identity.FamilyID) (identity.RefreshFamily, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.families[id]
	return f, ok
}

// LiveTokens returns the tokens a client could still present successfully
// at now: in a live family, unexpired, and neither used nor superseded.
// Each family must have at most one (ADR 0013).
func (s *Store) LiveTokens(now time.Time) []identity.RefreshToken {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []identity.RefreshToken
	for _, t := range s.tokens {
		if !s.families[t.Family].Revoked() && now.Before(t.ExpiresAt) && t.UsedAt.IsZero() && t.SupersededAt.IsZero() {
			out = append(out, t)
		}
	}
	return out
}
