package identity

import (
	"context"
	"fmt"
	"time"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Deps are the Service's ports. Every one is required.
type Deps struct {
	Users       Users
	Refresh     RefreshTokens
	ResetCodes  ResetCodes
	Memberships Memberships
	Invitations Invitations
	Hasher      PasswordHasher
	Tokens      AccessTokenIssuer
	Denylist    Denylist
	Mailer      ResetCodeMailer
	Tx          appshared.TxManager
	Clock       appshared.Clock
	IDs         appshared.IDGenerator
	Secrets     appshared.SecretGenerator
	// ResetCodeKey is the server key reset codes are stored under (an
	// HMAC; ADR 0011). It must be secret and at least 32 bytes: with a weak
	// key the HMAC is a public function, and six digits fall to a million
	// guesses.
	ResetCodeKey []byte
}

// MinResetCodeKeyLength is the shortest ResetCodeKey NewService accepts.
const MinResetCodeKeyLength = 32

// Service implements UseCases: staff sign-up, login and sessions, password
// reset, and joining a store by invitation.
//
// The /auth calls are public, so they set their own RLS scope, as ADR 0009
// lists: the zero scope for the global identity tables, the user alone to
// read their stores, and the store inside an invitation's token.
type Service struct {
	users       Users
	refresh     RefreshTokens
	resetCodes  ResetCodes
	memberships Memberships
	invitations Invitations
	hasher      PasswordHasher
	tokens      AccessTokenIssuer
	denylist    Denylist
	mailer      ResetCodeMailer
	tx          appshared.TxManager
	clock       appshared.Clock
	ids         appshared.IDGenerator
	secrets     appshared.SecretGenerator
	resetKey    []byte
}

// NewService returns a Service over d. It returns an error when the reset
// code key is shorter than MinResetCodeKeyLength, so a missing key stops the
// API at start instead of weakening every code.
func NewService(d Deps) (*Service, error) {
	if len(d.ResetCodeKey) < MinResetCodeKeyLength {
		return nil, fmt.Errorf("identity: the reset code key has %d bytes, want at least %d", len(d.ResetCodeKey), MinResetCodeKeyLength)
	}
	return &Service{
		users: d.Users, refresh: d.Refresh, resetCodes: d.ResetCodes,
		memberships: d.Memberships, invitations: d.Invitations,
		hasher: d.Hasher, tokens: d.Tokens, denylist: d.Denylist, mailer: d.Mailer,
		tx: d.Tx, clock: d.Clock, ids: d.IDs, secrets: d.Secrets,
		resetKey: d.ResetCodeKey,
	}, nil
}

// detachedTimeout bounds work that must finish after the caller has gone.
const detachedTimeout = 10 * time.Second

// detached returns a context that ctx's cancellation does not stop, bounded
// by detachedTimeout. A logout's revoke and every denylist write run on it:
// a client that drops the connection must not keep a thief's session or
// access token alive (ADR 0013).
func detached(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), detachedTimeout)
}

// deny denylists jtis for as long as any of them can still work, on a
// detached context.
func (s *Service) deny(ctx context.Context, jtis []shared.ID, now time.Time) {
	if len(jtis) == 0 {
		return
	}
	ctx, cancel := detached(ctx)
	defer cancel()
	s.denylist.Deny(ctx, jtis, now.Add(identity.DenylistWindow))
}

// publicScope is for the global identity tables, which have no RLS: no
// tenant rows are visible.
func publicScope(ctx context.Context) context.Context {
	return appshared.WithScope(ctx, appshared.Scope{})
}

// userScope shows the user's own memberships and stores, and no store's
// rows (ADR 0010).
func userScope(ctx context.Context, user shared.UserID) context.Context {
	return appshared.WithScope(ctx, appshared.Scope{UserID: user})
}

// storeScope is the store inside an invitation's token (ADR 0011).
func storeScope(ctx context.Context, tenant shared.TenantID) context.Context {
	return appshared.WithScope(ctx, appshared.Scope{TenantID: tenant})
}

func userView(u *identity.User) UserView {
	return UserView{ID: u.ID(), Name: u.Name().String(), Email: u.Email().String()}
}

// find returns the membership in tenant, if any.
func find(ms []Membership, tenant shared.TenantID) (Membership, bool) {
	for _, m := range ms {
		if m.Tenant.ID == tenant && !tenant.IsZero() {
			return m, true
		}
	}
	return Membership{}, false
}
