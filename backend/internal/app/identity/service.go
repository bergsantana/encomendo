package identity

import (
	"context"

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
	// HMAC; ADR 0011). It must be secret and at least 32 bytes.
	ResetCodeKey []byte
}

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

// NewService returns a Service over d.
func NewService(d Deps) *Service {
	return &Service{
		users: d.Users, refresh: d.Refresh, resetCodes: d.ResetCodes,
		memberships: d.Memberships, invitations: d.Invitations,
		hasher: d.Hasher, tokens: d.Tokens, denylist: d.Denylist, mailer: d.Mailer,
		tx: d.Tx, clock: d.Clock, ids: d.IDs, secrets: d.Secrets,
		resetKey: d.ResetCodeKey,
	}
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
