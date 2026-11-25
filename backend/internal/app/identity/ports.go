package identity

import (
	"context"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// UseCases is the driving port: what the /auth handlers call. Every call
// that starts a session returns an AuthResult (ADR 0010).
type UseCases interface {
	SignUp(ctx context.Context, in SignUpInput) (AuthResult, error)
	Login(ctx context.Context, in LoginInput) (AuthResult, error)
	Refresh(ctx context.Context, in RefreshInput) (AuthResult, error)
	SwitchTenant(ctx context.Context, in SwitchTenantInput) (AuthResult, error)
	Logout(ctx context.Context, in LogoutInput) error
	RequestPasswordReset(ctx context.Context, in PasswordResetInput) error
	ConfirmPasswordReset(ctx context.Context, in ConfirmPasswordResetInput) error
	PreviewInvitation(ctx context.Context, token string) (InvitationPreview, error)
	AcceptInvitation(ctx context.Context, in AcceptInvitationInput) (AuthResult, error)
}

// The repository ports. The tables behind them are global, with no RLS
// (0002_identity), so they work in any scope; every method runs inside
// TxManager.WithinTx and returns appshared.ErrNoTransaction outside it. A
// missing row is shared.ErrNotFound; any other failure passes through
// unchanged, so an outage never reads as a bad token (ADR 0013).

// Users stores staff accounts.
type Users interface {
	// InsertUser stores a new account. It returns an error matching
	// shared.ErrConflict when the email already has one, in any case.
	InsertUser(ctx context.Context, u *identity.User) error
	// UserByID returns the account with this id.
	UserByID(ctx context.Context, id shared.UserID) (*identity.User, error)
	// UsersByID returns the accounts with these ids that exist, in any
	// order; unknown ids are left out.
	UsersByID(ctx context.Context, ids []shared.UserID) ([]*identity.User, error)
	// UserByEmail returns the account with this email, ignoring case.
	UserByEmail(ctx context.Context, email identity.Email) (*identity.User, error)
	// LockUserByEmail is UserByEmail with the row locked FOR UPDATE until
	// the transaction ends. Password reset takes it first, so resets of one
	// account run one at a time and logins wait for them (ADR 0013).
	LockUserByEmail(ctx context.Context, email identity.Email) (*identity.User, error)
	// PasswordHashForShare reads the account's current password hash FOR
	// SHARE. Login and invitation acceptance compare it with the hash they
	// verified, in the transaction that creates the session (ADR 0013).
	PasswordHashForShare(ctx context.Context, id shared.UserID) (identity.PasswordHash, error)
	// UpdatePassword stores u's password hash while the stored version is
	// still u.Version(), and stores the next version. It returns
	// shared.ErrVersionConflict otherwise.
	UpdatePassword(ctx context.Context, u *identity.User) error
}

// RefreshTokens stores refresh families and tokens. Its callers follow the
// lock protocol of ADR 0013: find the token, lock its family, read the token
// again, decide, then change it only with the conditional methods.
type RefreshTokens interface {
	// CreateFamily stores a new live family.
	CreateFamily(ctx context.Context, f identity.RefreshFamily) error
	// InsertRefreshToken stores a new token of an existing family.
	InsertRefreshToken(ctx context.Context, t identity.RefreshToken) error
	// RefreshTokenByHash finds a staff token by the hash of its secret,
	// whatever its state, with its family. It is the first read of the
	// protocol and may be stale by the time the family is locked.
	RefreshTokenByHash(ctx context.Context, hash shared.SecretHash) (identity.RefreshToken, identity.RefreshFamily, error)
	// RefreshTokenByID reads a token again, after its family is locked.
	RefreshTokenByID(ctx context.Context, id identity.RefreshTokenID) (identity.RefreshToken, error)
	// LockFamily locks the family row FOR UPDATE until the transaction
	// ends and returns it as it is then.
	LockFamily(ctx context.Context, id identity.FamilyID) (identity.RefreshFamily, error)
	// MarkRefreshTokenUsed sets the token's first use to at, only while it
	// is neither used nor superseded, and reports whether it did.
	MarkRefreshTokenUsed(ctx context.Context, id identity.RefreshTokenID, at time.Time) (bool, error)
	// SupersedeRefreshToken marks a token superseded by a retry at at, only
	// while it is neither used nor superseded, and reports whether it did.
	SupersedeRefreshToken(ctx context.Context, id identity.RefreshTokenID, at time.Time) (bool, error)
	// SetSuccessor points a used token at next, a token of its family.
	SetSuccessor(ctx context.Context, id, next identity.RefreshTokenID) error
	// RevokeFamily revokes the family at at, if it is still live, and
	// returns the access jtis of its tokens created after issuedSince, for
	// the denylist. Revoking a revoked family keeps its first revoke time.
	// Callers lock the family first (LockFamily). The jtis are read in a
	// statement that starts after the revoke, never in the same one: a
	// single statement reads with the snapshot it took before waiting for
	// the lock, and would miss the token of a rotation that committed
	// meanwhile (ADR 0013).
	RevokeFamily(ctx context.Context, id identity.FamilyID, at, issuedSince time.Time) ([]shared.ID, error)
	// RevokeUserFamilies does RevokeFamily for every family of the user:
	// it locks them all FOR UPDATE first, then revokes them and reads the
	// jtis in later statements, for the same reason.
	RevokeUserFamilies(ctx context.Context, user shared.UserID, at, issuedSince time.Time) ([]shared.ID, error)
}

// ResetCodes stores password reset codes (ADR 0011).
type ResetCodes interface {
	// InsertResetCode stores a new code. It replaces the user's earlier
	// codes simply by being the newest.
	InsertResetCode(ctx context.Context, c identity.PasswordResetCode) error
	// NewestResetCode returns the user's most recent code.
	NewestResetCode(ctx context.Context, user shared.UserID) (identity.PasswordResetCode, error)
	// WrongResetAttemptsSince sums the wrong attempts on the user's codes
	// created after since.
	WrongResetAttemptsSince(ctx context.Context, user shared.UserID, since time.Time) (int, error)
	// AddWrongResetAttempt counts one wrong try on the code in one atomic
	// statement, only while it has fewer than identity.MaxResetAttempts, and
	// reports whether it did.
	AddWrongResetAttempt(ctx context.Context, id identity.ResetCodeID) (bool, error)
	// MarkResetCodeUsed uses the code up at at, only while it is unused,
	// unexpired at at and under identity.MaxResetAttempts, and reports
	// whether it did.
	MarkResetCodeUsed(ctx context.Context, id identity.ResetCodeID, at time.Time) (bool, error)
}

// PasswordHasher hashes and checks passwords with argon2id
// (adapter/auth/argon2id.go). It is slow on purpose, so use cases call it
// outside every transaction.
type PasswordHasher interface {
	// Hash returns a salted hash of password.
	Hash(ctx context.Context, password string) (identity.PasswordHash, error)
	// Verify reports whether password matches hash. For the empty hash it
	// does the same work as for a real one and reports false, so an unknown
	// email takes as long as a wrong password (ADR 0010). An error means
	// the hash is unreadable or the hasher failed, never a wrong password.
	Verify(ctx context.Context, hash identity.PasswordHash, password string) (bool, error)
}

// AccessClaims is what an access token says (docs/architecture.md 5.1).
type AccessClaims struct {
	Principal shared.Principal
	JTI       shared.ID
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// AccessTokenIssuer signs access tokens (adapter/auth/jwt.go: EdDSA with a
// kid).
type AccessTokenIssuer interface {
	// Issue returns the signed token for claims.
	Issue(ctx context.Context, claims AccessClaims) (string, error)
}

// Denylist stops access tokens before they expire (Redis).
type Denylist interface {
	// Deny stops the access tokens with these jtis until until. It never
	// fails the caller: the revoke in Postgres has committed by then, and
	// the adapter logs a Redis failure (ADR 0013). The service calls it
	// with a context the request's end does not cancel; the adapter must
	// not add a cancellation of its own beyond a short timeout.
	Deny(ctx context.Context, jtis []shared.ID, until time.Time)
}

// ResetCodeEmail is the "Esqueci a senha" message.
type ResetCodeEmail struct {
	To   identity.Email
	Name identity.PersonName
	Code identity.ResetCode
}

// ResetCodeMailer sends password reset codes (adapter/notify/smtp.go).
type ResetCodeMailer interface {
	// SendResetCode returns at once: delivery happens off the request path,
	// so the answer's timing never tells whether the account exists, and the
	// adapter logs a failure instead of returning it (ADR 0011). It must not
	// stop when ctx, the request's context, ends.
	SendResetCode(ctx context.Context, email ResetCodeEmail)
}

// TenantSummary names a store in a session or an invitation.
type TenantSummary struct {
	ID   shared.TenantID
	Name string
	Slug string
}

// Membership is one store a user belongs to, with their role there.
type Membership struct {
	Tenant TenantSummary
	Role   shared.Role
}

// Memberships reads which stores a user belongs to (tenancy, through
// adapter/inproc; ADR 0011). It runs in a transaction scoped to that user
// alone, the only scope in which RLS shows a user's memberships in every
// store (ADR 0010).
type Memberships interface {
	// MembershipsOf returns the user's memberships sorted by store name,
	// then id; none is an empty list, not an error.
	MembershipsOf(ctx context.Context, user shared.UserID) ([]Membership, error)
}

// PendingInvitation is an invitation that can still be accepted.
type PendingInvitation struct {
	ID        shared.InvitationID
	Tenant    TenantSummary
	Email     identity.Email
	Role      shared.Role
	ExpiresAt time.Time
}

// Join is a user using up an invitation to join its store.
type Join struct {
	Tenant shared.TenantID
	Hash   shared.SecretHash
	User   shared.UserID
	Email  identity.Email // the user's, which must be the invited one
	At     time.Time
}

// Invitations reads and uses up invitations (tenancy, through
// adapter/inproc; ADR 0011). Both methods run in a transaction scoped to the
// invitation's store, the tenant inside the token, so RLS finds nothing in
// any other store.
type Invitations interface {
	// PendingInvitation returns the invitation whose secret has hash, if it
	// is pending at now and its sender can still manage the team. It returns
	// tenancy.ErrInvitationNotPending otherwise, the same for every case.
	PendingInvitation(ctx context.Context, tenant shared.TenantID, hash shared.SecretHash, now time.Time) (PendingInvitation, error)
	// JoinByInvitation uses the invitation up and makes the user a member
	// with its role. It returns tenancy.ErrInvitationNotPending as
	// PendingInvitation does, also when j.Email is not the invited one, and
	// tenancy.ErrAlreadyMember when the user already belongs to the store,
	// leaving the invitation pending.
	JoinByInvitation(ctx context.Context, j Join) error
}
