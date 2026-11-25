package identity

import (
	"errors"
	"fmt"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Token lifetimes (docs/architecture.md 5.1, ADR 0013).
const (
	// AccessTokenLifetime is how long an access token works.
	AccessTokenLifetime = 15 * time.Minute
	// AccessTokenLeeway is the clock skew the verifier forgives on exp and
	// nbf.
	AccessTokenLeeway = 30 * time.Second
	// DenylistWindow is how far back a revoke reaches for access tokens to
	// denylist: no access token issued earlier still works.
	DenylistWindow = AccessTokenLifetime + AccessTokenLeeway
	// RefreshTokenLifetime is how long one refresh token works. Each use
	// issues a new one, so an active login never expires.
	RefreshTokenLifetime = 30 * 24 * time.Hour
	// RefreshRetryWindow is how long after its first use a refresh token may
	// be presented again as a retry of a lost answer (ADR 0013).
	RefreshRetryWindow = 2 * time.Minute
)

// ErrInvalidRefreshToken means a refresh family or token was built from
// missing parts. It is a bug, not a bad token, so it matches no sentinel.
var ErrInvalidRefreshToken = errors.New("identity: invalid refresh token record")

// FamilyID identifies a refresh family: one login.
type FamilyID shared.ID

// IsZero reports whether the id is unset.
func (id FamilyID) IsZero() bool { return shared.ID(id).IsZero() }

// String returns the canonical form.
func (id FamilyID) String() string { return shared.ID(id).String() }

// RefreshTokenID identifies one refresh token of a family.
type RefreshTokenID shared.ID

// IsZero reports whether the id is unset.
func (id RefreshTokenID) IsZero() bool { return shared.ID(id).IsZero() }

// String returns the canonical form.
func (id RefreshTokenID) String() string { return shared.ID(id).String() }

// RefreshFamily is one login of a staff user, shared by every token that
// login rotated through. Revoking it ends the login on every device at once,
// and every rotation locks it first (ADR 0013). Client account families come
// with milestone 7.
type RefreshFamily struct {
	ID        FamilyID
	User      shared.UserID
	RevokedAt time.Time // zero while the family is live
}

// NewRefreshFamily returns a live family. It returns ErrInvalidRefreshToken
// for a zero id or user.
func NewRefreshFamily(id FamilyID, user shared.UserID) (RefreshFamily, error) {
	if id.IsZero() || user.IsZero() {
		return RefreshFamily{}, fmt.Errorf("new refresh family %v of %v: %w", id, user, ErrInvalidRefreshToken)
	}
	return RefreshFamily{ID: id, User: user}, nil
}

// Revoked reports whether the family was revoked (reuse, logout or a
// password reset). No token of a revoked family works again.
func (f RefreshFamily) Revoked() bool { return !f.RevokedAt.IsZero() }

// RefreshTokenSpec is what a new refresh token is made of.
type RefreshTokenSpec struct {
	ID     RefreshTokenID
	Family FamilyID
	// Tenant is the store the session selects; zero for none (ADR 0010).
	Tenant shared.TenantID
	Hash   shared.SecretHash
	// AccessJTI is the jti of the access token issued with this one, so a
	// revoke can denylist it.
	AccessJTI shared.ID
}

// RefreshToken is one refresh token as stored: only the hash of its secret,
// and how it was used. The store changes it with conditional updates under
// the family's lock (ADR 0013); the rules for presenting it are UseAt.
type RefreshToken struct {
	ID           RefreshTokenID
	Family       FamilyID
	Tenant       shared.TenantID
	Hash         shared.SecretHash
	AccessJTI    shared.ID
	CreatedAt    time.Time
	ExpiresAt    time.Time
	UsedAt       time.Time      // first use; zero while unused
	ReplacedBy   RefreshTokenID // the live successor after a rotation or retry
	SupersededAt time.Time      // a retry replaced this token before anyone used it
}

// NewRefreshToken returns an unused token that expires RefreshTokenLifetime
// after now. It returns ErrInvalidRefreshToken for a zero id, family, hash
// or jti.
func NewRefreshToken(spec RefreshTokenSpec, now time.Time) (RefreshToken, error) {
	if spec.ID.IsZero() || spec.Family.IsZero() || spec.Hash == (shared.SecretHash{}) || spec.AccessJTI.IsZero() {
		return RefreshToken{}, fmt.Errorf("new refresh token %v: %w", spec.ID, ErrInvalidRefreshToken)
	}
	return RefreshToken{
		ID: spec.ID, Family: spec.Family, Tenant: spec.Tenant, Hash: spec.Hash, AccessJTI: spec.AccessJTI,
		CreatedAt: now, ExpiresAt: now.Add(RefreshTokenLifetime),
	}, nil
}

// Use says what presenting a refresh token does.
type Use int

// The outcomes of UseAt.
const (
	// UseRefused: the token is expired. Answer 401 and change nothing.
	UseRefused Use = iota
	// UseRotate: the token is unused. Mark it used and issue its successor.
	UseRotate
	// UseRetry: an answer was lost (ADR 0013). Supersede the unused
	// successor and issue a new one from this token.
	UseRetry
	// UseReuse: the token was used before, outside a retry. Revoke the
	// family and answer 401.
	UseReuse
)

// String names the outcome, for logs and test failures.
func (u Use) String() string {
	switch u {
	case UseRefused:
		return "refused"
	case UseRotate:
		return "rotate"
	case UseRetry:
		return "retry"
	case UseReuse:
		return "reuse"
	default:
		return fmt.Sprintf("Use(%d)", int(u))
	}
}

// UseAt decides what presenting t at now does, given successor, the token
// t.ReplacedBy names (nil when there is none). Read both after locking the
// family, never before (ADR 0013). The rules:
//   - an expired token is refused and revokes nothing: it is simply dead;
//   - a token a retry superseded is reuse;
//   - an unused token rotates;
//   - a used token is a retry when its first use was at most
//     RefreshRetryWindow ago and its successor was neither used nor
//     superseded; the window counts from the first use, so retries never
//     extend it;
//   - anything else is reuse.
//
// The caller checks the family first: no token of a revoked family is used.
func (t RefreshToken) UseAt(successor *RefreshToken, now time.Time) Use {
	switch {
	case !now.Before(t.ExpiresAt):
		return UseRefused
	case !t.SupersededAt.IsZero():
		return UseReuse
	case t.UsedAt.IsZero():
		return UseRotate
	case now.Sub(t.UsedAt) <= RefreshRetryWindow && unusedSuccessor(successor, t):
		return UseRetry
	default:
		return UseReuse
	}
}

// unusedSuccessor reports whether s is t's successor, neither used nor
// superseded.
func unusedSuccessor(s *RefreshToken, t RefreshToken) bool {
	return s != nil && s.ID == t.ReplacedBy && !t.ReplacedBy.IsZero() && s.UsedAt.IsZero() && s.SupersededAt.IsZero()
}

// RetriedTo returns t pointing at next, its new successor after a retry.
// UsedAt keeps the first use.
func (t RefreshToken) RetriedTo(next RefreshTokenID) RefreshToken {
	t.ReplacedBy = next
	return t
}
