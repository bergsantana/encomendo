package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

// errRotationRace means the presented token changed under its family's
// lock on every attempt, which the lock protocol makes impossible. It is a
// bug, so it matches no sentinel: a 500, never a 401, and the client keeps
// its session (ADR 0013).
var errRotationRace = errors.New("identity: refresh token kept changing under its family's lock")

// maxRotationAttempts bounds how often a rotation reads the token again
// after a conditional update changed nothing.
const maxRotationAttempts = 2

// pickStore chooses the store of the rotated session from the user's
// memberships and the store the token was issued for. An error refuses the
// call before anything rotates.
type pickStore func(ms []Membership, issued shared.TenantID) (Membership, error)

// keepStore keeps the token's store while the user still belongs to it and
// it is not suspended, and selects none otherwise (ADRs 0010 and 0022).
func keepStore(ms []Membership, issued shared.TenantID) (Membership, error) {
	m, _ := find(ms, issued) // not a member any more: a session without a store
	return m, nil
}

// Refresh trades a refresh token for a new session in the same store,
// following ADR 0013:
//   - an unused token rotates;
//   - a retry within 2 minutes of its first use, while its successor is
//     unused, supersedes that successor and rotates again;
//   - any other used token is reuse: the family is revoked, its recent
//     access tokens denylisted, and the answer is shared.ErrUnauthenticated;
//   - a malformed, unknown, expired or revoked token is
//     shared.ErrUnauthenticated and changes nothing.
//
// The role is read again from the membership; a removed member's session,
// and one whose store was suspended (ADR 0022), has no store. A store
// failure is returned as it is, never as ErrUnauthenticated.
func (s *Service) Refresh(ctx context.Context, in RefreshInput) (AuthResult, error) {
	return s.rotate(ctx, in.RefreshToken, keepStore)
}

// SwitchTenant is Refresh into another store the user belongs to. It
// returns a ValidationError on "tenantId" for no store, an error matching
// shared.ErrForbidden when the user is not a member, and
// tenancy.ErrTenantSuspended (which matches ErrForbidden too) when the store
// is suspended (ADR 0022). Each leaves the token as it was, so the client
// keeps its session (ADR 0013). Reuse still revokes.
func (s *Service) SwitchTenant(ctx context.Context, in SwitchTenantInput) (AuthResult, error) {
	if in.TenantID.IsZero() {
		return AuthResult{}, shared.NewValidationError(shared.FieldError{Field: "tenantId", Message: "Escolha uma loja."})
	}
	return s.rotate(ctx, in.RefreshToken, func(ms []Membership, _ shared.TenantID) (Membership, error) {
		if m, ok := find(ms, in.TenantID); ok {
			return m, nil
		}
		for _, m := range ms {
			if m.Tenant.ID == in.TenantID {
				return Membership{}, fmt.Errorf("switch to store %v: %w", in.TenantID, tenancy.ErrTenantSuspended)
			}
		}
		return Membership{}, fmt.Errorf("switch to store %v: not a member: %w", in.TenantID, shared.ErrForbidden)
	})
}

// rotation is what the locked transaction decided.
type rotation struct {
	reused bool
	jtis   []shared.ID // to denylist after a reuse
	sess   session
}

func (s *Service) rotate(ctx context.Context, presented string, pick pickStore) (AuthResult, error) {
	found, fam, err := s.findRefreshToken(ctx, presented)
	if err != nil {
		return AuthResult{}, err
	}
	fresh, err := s.drawToken(ctx)
	if err != nil {
		return AuthResult{}, err
	}
	now := s.clock.Now()
	var r rotation
	err = s.tx.WithinTx(userScope(ctx, fam.User), func(ctx context.Context) error {
		var err error
		r, err = s.rotateLocked(ctx, found.ID, fam.ID, pick, fresh, now)
		return err
	})
	if err != nil {
		return AuthResult{}, err
	}
	if r.reused {
		s.deny(ctx, r.jtis, now)
		return AuthResult{}, fmt.Errorf("refresh token reused; family revoked: %w", shared.ErrUnauthenticated)
	}
	return s.issue(ctx, r.sess, fresh, now)
}

// findRefreshToken is the protocol's first read: the token and its family,
// possibly stale. Only a malformed or unknown token is
// shared.ErrUnauthenticated.
func (s *Service) findRefreshToken(ctx context.Context, presented string) (identity.RefreshToken, identity.RefreshFamily, error) {
	secret, err := shared.ParseSecret(presented)
	if err != nil {
		return identity.RefreshToken{}, identity.RefreshFamily{}, fmt.Errorf("malformed refresh token: %w", shared.ErrUnauthenticated)
	}
	var tok identity.RefreshToken
	var fam identity.RefreshFamily
	err = s.tx.WithinTx(publicScope(ctx), func(ctx context.Context) error {
		var err error
		tok, fam, err = s.refresh.RefreshTokenByHash(ctx, secret.Hash())
		if errors.Is(err, shared.ErrNotFound) {
			return fmt.Errorf("unknown refresh token: %w", shared.ErrUnauthenticated)
		}
		if err != nil {
			return fmt.Errorf("find refresh token: %w", err)
		}
		return nil
	})
	return tok, fam, err
}

// rotateLocked locks the family, then decides from a fresh read of the
// token and changes it only with conditional updates (ADR 0013). A reuse
// returns nil with reused set, so the revoke commits.
func (s *Service) rotateLocked(ctx context.Context, id identity.RefreshTokenID, famID identity.FamilyID, pick pickStore, fresh freshToken, now time.Time) (rotation, error) {
	fam, err := s.refresh.LockFamily(ctx, famID)
	if errors.Is(err, shared.ErrNotFound) {
		return rotation{}, fmt.Errorf("refresh family gone: %w", shared.ErrUnauthenticated)
	}
	if err != nil {
		return rotation{}, fmt.Errorf("lock refresh family: %w", err)
	}
	if fam.Revoked() {
		return rotation{}, fmt.Errorf("refresh family revoked: %w", shared.ErrUnauthenticated)
	}
	st := &rotationState{fam: fam, pick: pick, fresh: fresh, now: now}
	for range maxRotationAttempts {
		r, done, err := s.attemptRotation(ctx, id, st)
		if err != nil || done {
			return r, err
		}
		// Another call changed the token first: read it again.
	}
	return rotation{}, errRotationRace
}

// rotationState is what the attempts of one rotation share.
type rotationState struct {
	fam   identity.RefreshFamily
	pick  pickStore
	fresh freshToken
	now   time.Time
	sess  *session // read once, before anything rotates (ADR 0013)
}

// attemptRotation reads the token again, decides, and acts. done is false
// only when the conditional update lost to another call.
func (s *Service) attemptRotation(ctx context.Context, id identity.RefreshTokenID, st *rotationState) (rotation, bool, error) {
	tok, succ, err := s.reread(ctx, id)
	if err != nil {
		return rotation{}, true, err
	}
	use := tok.UseAt(succ, st.now)
	switch use {
	case identity.UseRefused:
		return rotation{}, true, fmt.Errorf("refresh token expired: %w", shared.ErrUnauthenticated)
	case identity.UseReuse:
		jtis, err := s.refresh.RevokeFamily(ctx, st.fam.ID, st.now, st.now.Add(-identity.DenylistWindow))
		if err != nil {
			return rotation{}, true, fmt.Errorf("revoke refresh family: %w", err)
		}
		return rotation{reused: true, jtis: jtis}, true, nil
	}
	if st.sess == nil {
		read, err := s.sessionOf(ctx, st.fam.User, tok.Tenant, st.pick)
		if err != nil {
			return rotation{}, true, err
		}
		st.sess = &read
	}
	won, err := s.claim(ctx, tok, succ, use, st.now)
	if err != nil || !won {
		return rotation{}, err != nil, err
	}
	if err := s.replace(ctx, tok, st.fresh, st.sess.selected.Tenant.ID, st.now); err != nil {
		return rotation{}, true, err
	}
	return rotation{sess: *st.sess}, true, nil
}

// reread reads the token and its successor again, under the family's lock.
// A token gone since the first read is refused; a successor gone is no
// successor, which UseAt treats as reuse.
func (s *Service) reread(ctx context.Context, id identity.RefreshTokenID) (identity.RefreshToken, *identity.RefreshToken, error) {
	tok, err := s.refresh.RefreshTokenByID(ctx, id)
	if errors.Is(err, shared.ErrNotFound) {
		return identity.RefreshToken{}, nil, fmt.Errorf("refresh token gone: %w", shared.ErrUnauthenticated)
	}
	if err != nil {
		return identity.RefreshToken{}, nil, fmt.Errorf("read refresh token again: %w", err)
	}
	if tok.ReplacedBy.IsZero() {
		return tok, nil, nil
	}
	succ, err := s.refresh.RefreshTokenByID(ctx, tok.ReplacedBy)
	if errors.Is(err, shared.ErrNotFound) {
		return tok, nil, nil
	}
	if err != nil {
		return identity.RefreshToken{}, nil, fmt.Errorf("read successor: %w", err)
	}
	return tok, &succ, nil
}

// sessionOf reads the user's stores, picks the session's, and reads the
// user.
func (s *Service) sessionOf(ctx context.Context, user shared.UserID, issued shared.TenantID, pick pickStore) (session, error) {
	ms, err := s.memberships.MembershipsOf(ctx, user)
	if err != nil {
		return session{}, fmt.Errorf("read memberships: %w", err)
	}
	selected, err := pick(ms, issued)
	if err != nil {
		return session{}, err
	}
	u, err := s.users.UserByID(ctx, user)
	if errors.Is(err, shared.ErrNotFound) {
		return session{}, fmt.Errorf("account of the refresh family gone: %w", shared.ErrUnauthenticated)
	}
	if err != nil {
		return session{}, fmt.Errorf("read user: %w", err)
	}
	return session{user: u, memberships: ms, selected: selected}, nil
}

// claim makes the conditional change that settles a race: a rotation marks
// the token used, a retry supersedes the unused successor. It reports false
// when another call changed that row first.
func (s *Service) claim(ctx context.Context, tok identity.RefreshToken, succ *identity.RefreshToken, use identity.Use, now time.Time) (bool, error) {
	if use == identity.UseRetry {
		won, err := s.refresh.SupersedeRefreshToken(ctx, succ.ID, now)
		if err != nil {
			return false, fmt.Errorf("supersede refresh token: %w", err)
		}
		return won, nil
	}
	won, err := s.refresh.MarkRefreshTokenUsed(ctx, tok.ID, now)
	if err != nil {
		return false, fmt.Errorf("mark refresh token used: %w", err)
	}
	return won, nil
}

// replace stores the new token and points the presented one at it. The
// successor goes in first: the key on replaced_by needs it to exist.
func (s *Service) replace(ctx context.Context, tok identity.RefreshToken, fresh freshToken, tenant shared.TenantID, now time.Time) error {
	next, err := fresh.in(tok.Family, tenant, now)
	if err != nil {
		return err
	}
	if err := s.refresh.InsertRefreshToken(ctx, next); err != nil {
		return fmt.Errorf("store refresh token: %w", err)
	}
	if err := s.refresh.SetSuccessor(ctx, tok.ID, next.ID); err != nil {
		return fmt.Errorf("point refresh token at its successor: %w", err)
	}
	return nil
}

// Logout revokes the family of the presented refresh token and denylists
// its recent access tokens and the one sent with the call. It succeeds for a
// missing, malformed or unknown token (the endpoint always answers 204) and
// returns only store failures. It finishes even when the client drops the
// connection: the user believes they are logged out.
func (s *Service) Logout(ctx context.Context, in LogoutInput) error {
	ctx, cancel := detached(ctx)
	defer cancel()
	now := s.clock.Now()
	var jtis []shared.ID
	if secret, err := shared.ParseSecret(in.RefreshToken); err == nil {
		err := s.tx.WithinTx(publicScope(ctx), func(ctx context.Context) error {
			var err error
			jtis, err = s.revokeFamilyOf(ctx, secret.Hash(), now)
			return err
		})
		if err != nil {
			return err
		}
	}
	if !in.AccessJTI.IsZero() {
		jtis = append(jtis, in.AccessJTI)
	}
	s.deny(ctx, jtis, now)
	return nil
}

// revokeFamilyOf locks the family of the token with hash, then revokes it.
// The lock waits for a rotation in flight, so the token it made is revoked
// and its access jti is among those returned (ADR 0013). An unknown token
// revokes nothing.
func (s *Service) revokeFamilyOf(ctx context.Context, hash shared.SecretHash, now time.Time) ([]shared.ID, error) {
	_, fam, err := s.refresh.RefreshTokenByHash(ctx, hash)
	if errors.Is(err, shared.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find refresh token: %w", err)
	}
	if _, err := s.refresh.LockFamily(ctx, fam.ID); errors.Is(err, shared.ErrNotFound) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("lock refresh family: %w", err)
	}
	jtis, err := s.refresh.RevokeFamily(ctx, fam.ID, now, now.Add(-identity.DenylistWindow))
	if err != nil {
		return nil, fmt.Errorf("revoke refresh family: %w", err)
	}
	return jtis, nil
}
