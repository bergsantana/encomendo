package tenancy

import (
	"encoding/base64"
	"fmt"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// InvitationLifetime is how long an invitation link works (ADR 0011).
const InvitationLifetime = 7 * 24 * time.Hour

// InvitationSpec is what a new invitation is made of.
type InvitationSpec struct {
	ID        shared.InvitationID
	Tenant    shared.TenantID
	Email     identity.Email
	Role      shared.Role
	Hash      shared.SecretHash // of the link's secret; the secret itself is only in the email
	InvitedBy shared.UserID
}

// Invitation ("Convite") offers one email a role in one store, for 7 days,
// once (ADR 0011).
type Invitation struct {
	id         shared.InvitationID
	tenant     shared.TenantID
	email      identity.Email
	role       shared.Role
	hash       shared.SecretHash
	invitedBy  shared.UserID
	createdAt  time.Time
	expiresAt  time.Time
	acceptedAt time.Time
	revokedAt  time.Time
}

// NewInvitation returns a pending invitation that expires InvitationLifetime
// after now. It returns a ValidationError on "role" for the owner role, and
// ErrInvalidInvitation for any zero part.
func NewInvitation(spec InvitationSpec, now time.Time) (*Invitation, error) {
	if _, err := ParseInvitableRole(string(spec.Role)); err != nil {
		return nil, err
	}
	if spec.ID.IsZero() || spec.Tenant.IsZero() || spec.Email.IsZero() || spec.Hash == (shared.SecretHash{}) || spec.InvitedBy.IsZero() {
		return nil, fmt.Errorf("new invitation %v: %w", spec.ID, ErrInvalidInvitation)
	}
	return &Invitation{
		id: spec.ID, tenant: spec.Tenant, email: spec.Email, role: spec.Role, hash: spec.Hash, invitedBy: spec.InvitedBy,
		createdAt: now, expiresAt: now.Add(InvitationLifetime),
	}, nil
}

// InvitationRecord is an invitation as a repository stores it.
type InvitationRecord struct {
	ID         shared.InvitationID
	Tenant     shared.TenantID
	Email      string
	Role       shared.Role
	Hash       shared.SecretHash
	InvitedBy  shared.UserID
	CreatedAt  time.Time
	ExpiresAt  time.Time
	AcceptedAt time.Time
	RevokedAt  time.Time
}

// RestoreInvitation rebuilds a stored invitation. It returns
// ErrInvalidInvitation when the record breaks a rule a stored invitation
// always keeps.
func RestoreInvitation(r InvitationRecord) (*Invitation, error) {
	email, err := identity.ParseEmail(r.Email)
	if err != nil || !invitable(r.Role) || (!r.AcceptedAt.IsZero() && !r.RevokedAt.IsZero()) {
		return nil, fmt.Errorf("restore invitation %v: %w", r.ID, ErrInvalidInvitation)
	}
	inv, err := NewInvitation(InvitationSpec{
		ID: r.ID, Tenant: r.Tenant, Email: email, Role: r.Role, Hash: r.Hash, InvitedBy: r.InvitedBy,
	}, r.CreatedAt)
	if err != nil {
		return nil, err
	}
	inv.expiresAt, inv.acceptedAt, inv.revokedAt = r.ExpiresAt, r.AcceptedAt, r.RevokedAt
	return inv, nil
}

// ID returns the invitation's id.
func (i *Invitation) ID() shared.InvitationID { return i.id }

// TenantID returns the store the invitation is for.
func (i *Invitation) TenantID() shared.TenantID { return i.tenant }

// Email returns the invited address.
func (i *Invitation) Email() identity.Email { return i.email }

// Role returns the role it offers: admin, staff or viewer.
func (i *Invitation) Role() shared.Role { return i.role }

// Hash returns the stored hash of the link's secret.
func (i *Invitation) Hash() shared.SecretHash { return i.hash }

// InvitedBy returns who sent it. Accepting checks that they can still
// manage the team (ADR 0011).
func (i *Invitation) InvitedBy() shared.UserID { return i.invitedBy }

// CreatedAt returns when it was sent.
func (i *Invitation) CreatedAt() time.Time { return i.createdAt }

// ExpiresAt returns when the link stops working.
func (i *Invitation) ExpiresAt() time.Time { return i.expiresAt }

// AcceptedAt returns when it was accepted; zero if it was not.
func (i *Invitation) AcceptedAt() time.Time { return i.acceptedAt }

// RevokedAt returns when it was revoked; zero if it was not.
func (i *Invitation) RevokedAt() time.Time { return i.revokedAt }

// PendingAt reports whether the invitation can still be accepted at now:
// neither accepted nor revoked, and not expired.
func (i *Invitation) PendingAt(now time.Time) bool {
	return i.acceptedAt.IsZero() && i.revokedAt.IsZero() && now.Before(i.expiresAt)
}

// Accept uses the invitation up. It returns ErrInvitationNotPending when it
// is no longer pending at now.
func (i *Invitation) Accept(now time.Time) error {
	if !i.PendingAt(now) {
		return ErrInvitationNotPending
	}
	i.acceptedAt = now
	return nil
}

// Revoke cancels the invitation, so its link stops working. It returns
// ErrInvitationNotPending when it is no longer pending at now.
func (i *Invitation) Revoke(now time.Time) error {
	if !i.PendingAt(now) {
		return ErrInvitationNotPending
	}
	i.revokedAt = now
	return nil
}

// InvitationToken is what an invitation link carries: the store's id, which
// is not secret, and 32 random bytes, which are (ADR 0011). Accepting reads
// the tenant from the token to scope its transaction, then finds the
// invitation by the secret's hash under RLS, so no lookup crosses stores.
type InvitationToken struct {
	tenant shared.TenantID
	secret shared.Secret
}

const invitationTokenBytes = 16 + 32

// NewInvitationToken joins a store id and a secret into a link token.
func NewInvitationToken(tenant shared.TenantID, secret shared.Secret) InvitationToken {
	return InvitationToken{tenant: tenant, secret: secret}
}

// ParseInvitationToken reads what Reveal writes. It returns
// ErrInvitationNotPending for anything else, including a zero tenant, so a
// malformed link gets the same 404 as an unknown one.
func ParseInvitationToken(s string) (InvitationToken, error) {
	var raw [invitationTokenBytes]byte
	if len(s) != base64.RawURLEncoding.EncodedLen(len(raw)) {
		return InvitationToken{}, ErrInvitationNotPending
	}
	if n, err := base64.RawURLEncoding.Strict().Decode(raw[:], []byte(s)); err != nil || n != len(raw) {
		return InvitationToken{}, ErrInvitationNotPending
	}
	var t InvitationToken
	copy(t.tenant[:], raw[:16])
	copy(t.secret[:], raw[16:])
	if t.tenant.IsZero() {
		return InvitationToken{}, ErrInvitationNotPending
	}
	return t, nil
}

// TenantID returns the store the link is for.
func (t InvitationToken) TenantID() shared.TenantID { return t.tenant }

// Hash returns the hash the invitation is stored under.
func (t InvitationToken) Hash() shared.SecretHash { return t.secret.Hash() }

// String hides the token, so no %v, %s or log line prints a working link.
func (InvitationToken) String() string { return shared.Redacted }

// GoString hides the token from %#v too.
func (InvitationToken) GoString() string { return "tenancy.InvitationToken(" + shared.Redacted + ")" }

// Reveal returns the token as 64 characters of unpadded URL-safe base64,
// for the link's fragment (…/convite#token=…). Only the mailer calls it.
func (t InvitationToken) Reveal() string {
	var raw [invitationTokenBytes]byte
	copy(raw[:16], t.tenant[:])
	copy(raw[16:], t.secret[:])
	return base64.RawURLEncoding.EncodeToString(raw[:])
}
