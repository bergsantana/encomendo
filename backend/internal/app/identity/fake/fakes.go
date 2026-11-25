package fake

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

// ErrUnreadableHash is what Hasher.Verify returns for a hash it did not
// make, as argon2id does for a malformed PHC string.
var ErrUnreadableHash = errors.New("fake hasher: unreadable hash")

const hashPrefix = "fake-argon2id$"

// Hasher is a PasswordHasher that is fast and readable: a hash is
// "fake-argon2id$<n>$<password>", salted by a counter so two hashes of one
// password differ. It counts calls, so a test can check that an unknown
// email still costs one Verify (ADR 0010).
type Hasher struct {
	mu       sync.Mutex
	n        int
	hashes   int
	verifies int
	fail     error
}

// NewHasher returns a hasher with no calls counted.
func NewHasher() *Hasher { return &Hasher{} }

// Hash implements PasswordHasher.
func (h *Hasher) Hash(_ context.Context, password string) (identity.PasswordHash, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hashes++
	if h.fail != nil {
		return "", h.fail
	}
	h.n++
	return identity.PasswordHash(fmt.Sprintf("%s%d$%s", hashPrefix, h.n, password)), nil
}

// Verify implements PasswordHasher.
func (h *Hasher) Verify(_ context.Context, hash identity.PasswordHash, password string) (bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.verifies++
	if h.fail != nil {
		return false, h.fail
	}
	if hash == "" {
		return false, nil
	}
	rest, ok := strings.CutPrefix(string(hash), hashPrefix)
	if !ok {
		return false, ErrUnreadableHash
	}
	_, stored, ok := strings.Cut(rest, "$")
	if !ok {
		return false, ErrUnreadableHash
	}
	return stored == password, nil
}

// Fail makes every later call return err; nil heals it.
func (h *Hasher) Fail(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.fail = err
}

// Hashes returns how many times Hash was called.
func (h *Hasher) Hashes() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.hashes
}

// Verifies returns how many times Verify was called.
func (h *Hasher) Verifies() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.verifies
}

// Tokens is an AccessTokenIssuer that records what it signed. A token is
// "access." followed by its jti.
type Tokens struct {
	mu     sync.Mutex
	issued []appidentity.AccessClaims
	fail   error
}

// NewTokens returns an issuer with nothing issued.
func NewTokens() *Tokens { return &Tokens{} }

// Issue implements AccessTokenIssuer.
func (t *Tokens) Issue(_ context.Context, c appidentity.AccessClaims) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.fail != nil {
		return "", t.fail
	}
	if err := appidentity.CheckAccessClaims(c); err != nil {
		return "", err
	}
	t.issued = append(t.issued, c)
	return "access." + c.JTI.String(), nil
}

// Fail makes every later Issue return err; nil heals it.
func (t *Tokens) Fail(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.fail = err
}

// Issued returns the claims of every token issued, oldest first.
func (t *Tokens) Issued() []appidentity.AccessClaims {
	t.mu.Lock()
	defer t.mu.Unlock()
	return slices.Clone(t.issued)
}

// Denied is one Denylist.Deny call. CtxErr is the context's error at the
// call, which must be nil: a revoke's denylist write outlives the request.
type Denied struct {
	JTIs   []shared.ID
	Until  time.Time
	CtxErr error
}

// Denylist is a Denylist that records what it was told.
type Denylist struct {
	mu     sync.Mutex
	denied []Denied
}

// NewDenylist returns an empty denylist.
func NewDenylist() *Denylist { return &Denylist{} }

// Deny implements Denylist.
func (d *Denylist) Deny(ctx context.Context, jtis []shared.ID, until time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.denied = append(d.denied, Denied{JTIs: slices.Clone(jtis), Until: until, CtxErr: ctx.Err()})
}

// Denied returns every Deny call, oldest first.
func (d *Denylist) Denied() []Denied {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.denied)
}

// JTIs returns every jti denied so far.
func (d *Denylist) JTIs() []shared.ID {
	var out []shared.ID
	for _, call := range d.Denied() {
		out = append(out, call.JTIs...)
	}
	return out
}

// Mailer is a ResetCodeMailer that keeps what it was asked to send.
type Mailer struct {
	mu   sync.Mutex
	sent []appidentity.ResetCodeEmail
}

// NewMailer returns a mailer with nothing sent.
func NewMailer() *Mailer { return &Mailer{} }

// SendResetCode implements ResetCodeMailer.
func (m *Mailer) SendResetCode(_ context.Context, e appidentity.ResetCodeEmail) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, e)
}

// Sent returns every email, oldest first.
func (m *Mailer) Sent() []appidentity.ResetCodeEmail {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.sent)
}

// Memberships is a Memberships port over an in-memory list, with the RLS of
// 0007_rls_policies: in a scope of the user alone it shows all of the
// user's memberships; in a store's scope only the one in that store; in any
// other scope none.
type Memberships struct {
	mu     sync.Mutex
	byUser map[shared.UserID][]appidentity.Membership
	fail   error
}

// NewMemberships returns a list with no memberships.
func NewMemberships() *Memberships {
	return &Memberships{byUser: map[shared.UserID][]appidentity.Membership{}}
}

// Add makes user a member of m.Tenant with m.Role, replacing an earlier role
// there.
func (f *Memberships) Add(user shared.UserID, m appidentity.Membership) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removeLocked(user, m.Tenant.ID)
	f.byUser[user] = append(f.byUser[user], m)
}

// Remove takes user out of tenant.
func (f *Memberships) Remove(user shared.UserID, tenant shared.TenantID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removeLocked(user, tenant)
}

func (f *Memberships) removeLocked(user shared.UserID, tenant shared.TenantID) {
	f.byUser[user] = slices.DeleteFunc(f.byUser[user], func(m appidentity.Membership) bool { return m.Tenant.ID == tenant })
}

// Fail makes every later call return err; nil heals it.
func (f *Memberships) Fail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fail = err
}

// MembershipsOf implements Memberships.
func (f *Memberships) MembershipsOf(ctx context.Context, user shared.UserID) ([]appidentity.Membership, error) {
	if !sharedfake.InTx(ctx) {
		return nil, appshared.ErrNoTransaction
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return nil, f.fail
	}
	scope := appshared.ScopeFrom(ctx)
	out := []appidentity.Membership{}
	for _, m := range f.byUser[user] {
		ownScope := scope.TenantID.IsZero() && scope.UserID == user && scope.ClientAccountID.IsZero()
		if ownScope || (!scope.TenantID.IsZero() && scope.TenantID == m.Tenant.ID) {
			out = append(out, m)
		}
	}
	slices.SortFunc(out, func(a, b appidentity.Membership) int {
		if c := strings.Compare(a.Tenant.Name, b.Tenant.Name); c != 0 {
			return c
		}
		return strings.Compare(a.Tenant.ID.String(), b.Tenant.ID.String())
	})
	return out, nil
}

type invitationKey struct {
	tenant shared.TenantID
	hash   shared.SecretHash
}

// Invitations is an Invitations port over in-memory pending invitations,
// with RLS: a call whose scope is not the invitation's store finds nothing.
// Joining adds the membership to the Memberships it was built with.
type Invitations struct {
	mu          sync.Mutex
	pending     map[invitationKey]appidentity.PendingInvitation
	members     map[shared.TenantID]map[shared.UserID]bool
	memberships *Memberships
	joins       []appidentity.Join
	fail        error
}

// NewInvitations returns invitations that record joins in memberships.
func NewInvitations(memberships *Memberships) *Invitations {
	return &Invitations{
		pending:     map[invitationKey]appidentity.PendingInvitation{},
		members:     map[shared.TenantID]map[shared.UserID]bool{},
		memberships: memberships,
	}
}

// Add stores a pending invitation under the hash of its link's secret.
func (f *Invitations) Add(hash shared.SecretHash, p appidentity.PendingInvitation) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pending[invitationKey{p.Tenant.ID, hash}] = p
}

// AddMember marks user as already in tenant's team.
func (f *Invitations) AddMember(tenant shared.TenantID, user shared.UserID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.members[tenant] == nil {
		f.members[tenant] = map[shared.UserID]bool{}
	}
	f.members[tenant][user] = true
}

// Fail makes every later call return err; nil heals it.
func (f *Invitations) Fail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fail = err
}

// Joins returns every successful join, oldest first.
func (f *Invitations) Joins() []appidentity.Join {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.joins)
}

// PendingInvitation implements Invitations.
func (f *Invitations) PendingInvitation(ctx context.Context, tenant shared.TenantID, hash shared.SecretHash, now time.Time) (appidentity.PendingInvitation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.find(ctx, tenant, hash, now)
}

func (f *Invitations) find(ctx context.Context, tenant shared.TenantID, hash shared.SecretHash, now time.Time) (appidentity.PendingInvitation, error) {
	if !sharedfake.InTx(ctx) {
		return appidentity.PendingInvitation{}, appshared.ErrNoTransaction
	}
	if f.fail != nil {
		return appidentity.PendingInvitation{}, f.fail
	}
	p, ok := f.pending[invitationKey{tenant, hash}]
	if !ok || appshared.ScopeFrom(ctx).TenantID != tenant || !now.Before(p.ExpiresAt) {
		return appidentity.PendingInvitation{}, tenancy.ErrInvitationNotPending
	}
	return p, nil
}

// JoinByInvitation implements Invitations.
func (f *Invitations) JoinByInvitation(ctx context.Context, j appidentity.Join) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, err := f.find(ctx, j.Tenant, j.Hash, j.At)
	if err != nil {
		return err
	}
	if !p.Email.Equal(j.Email) {
		return tenancy.ErrInvitationNotPending
	}
	if f.members[j.Tenant][j.User] {
		return tenancy.ErrAlreadyMember
	}
	key := invitationKey{j.Tenant, j.Hash}
	delete(f.pending, key)
	if f.members[j.Tenant] == nil {
		f.members[j.Tenant] = map[shared.UserID]bool{}
	}
	f.members[j.Tenant][j.User] = true
	f.joins = append(f.joins, j)
	if f.memberships != nil {
		f.memberships.Add(j.User, appidentity.Membership{Tenant: p.Tenant, Role: p.Role})
	}
	sharedfake.OnRollback(ctx, func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.pending[key] = p
		delete(f.members[j.Tenant], j.User)
		f.joins = f.joins[:len(f.joins)-1]
		if f.memberships != nil {
			f.memberships.Remove(j.User, j.Tenant)
		}
	})
	return nil
}
