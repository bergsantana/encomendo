package fake

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	apptenancy "github.com/bergsantana/encomendo/backend/internal/app/tenancy"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

// ErrRowSecurity is what the fake returns for a write RLS would refuse, as
// Postgres does with SQLSTATE 42501. It matches no sentinel: a use case
// that writes in the wrong scope is a bug and a 500.
var ErrRowSecurity = errors.New("fake: new row violates row-level security policy")

type memberKey struct {
	tenant shared.TenantID
	user   shared.UserID
}

// Store is an in-memory Tenants, Members and Invitations with the RLS of
// 0007_rls_policies emulated from the scope in the context: a tenant's rows
// are visible only in that tenant's scope, writes elsewhere fail with
// ErrRowSecurity, and a scope of the user alone sees that user's
// memberships and stores and nothing else. It keeps the keys of
// 0004_tenancy too: unique slugs, one membership per user and store, one
// owner per store, one open invitation per email and store.
//
// Like adapter/postgres it refuses calls outside a fake transaction, and a
// failed transaction undoes its writes (sharedfake.KeepOnRollback; ADR
// 0009).
type Store struct {
	mu          sync.Mutex
	tenants     map[shared.TenantID]tenancy.TenantRecord
	settings    map[shared.TenantID]tenancy.SettingsRecord
	members     map[memberKey]tenancy.MembershipRecord
	invitations map[shared.InvitationID]tenancy.InvitationRecord
	faults      map[string]error
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{
		tenants:     map[shared.TenantID]tenancy.TenantRecord{},
		settings:    map[shared.TenantID]tenancy.SettingsRecord{},
		members:     map[memberKey]tenancy.MembershipRecord{},
		invitations: map[shared.InvitationID]tenancy.InvitationRecord{},
		faults:      map[string]error{},
	}
}

// Fail makes every later call of method return err; nil heals it.
func (s *Store) Fail(method string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.faults[method] = err
}

// enter checks the transaction and the fault for method and returns the
// scope. On success it returns with s.mu held.
func (s *Store) enter(ctx context.Context, method string) (appshared.Scope, error) {
	if !sharedfake.InTx(ctx) {
		return appshared.Scope{}, appshared.ErrNoTransaction
	}
	s.mu.Lock()
	if err := s.faults[method]; err != nil {
		s.mu.Unlock()
		return appshared.Scope{}, err
	}
	return appshared.ScopeFrom(ctx), nil
}

// sees reports whether RLS shows tenant's rows in scope.
func sees(scope appshared.Scope, tenant shared.TenantID) bool {
	return !tenant.IsZero() && scope.TenantID == tenant
}

// ---- Tenants ----

// InsertTenant implements Tenants.
func (s *Store) InsertTenant(ctx context.Context, t *tenancy.Tenant) error {
	scope, err := s.enter(ctx, "InsertTenant")
	if err != nil {
		return err
	}
	defer s.mu.Unlock()
	if !sees(scope, t.ID()) {
		return ErrRowSecurity
	}
	for _, other := range s.tenants { // a unique index sees every row, whatever RLS shows
		if other.ID == t.ID() || other.Slug == t.Slug().String() {
			return fmt.Errorf("tenants_slug_key: %w", shared.ErrConflict)
		}
	}
	sharedfake.KeepOnRollback(ctx, &s.mu, s.tenants, t.ID())
	s.tenants[t.ID()] = tenancy.TenantRecord{
		ID: t.ID(), Slug: t.Slug().String(), Name: t.Name().String(), Status: t.Status(), Version: t.Version(), CreatedAt: t.CreatedAt(),
	}
	return nil
}

// TenantByID implements Tenants.
func (s *Store) TenantByID(ctx context.Context, id shared.TenantID) (*tenancy.Tenant, error) {
	scope, err := s.enter(ctx, "TenantByID")
	if err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	r, ok := s.tenants[id]
	if !ok || (!sees(scope, id) && !s.memberStore(scope, id)) {
		return nil, shared.ErrNotFound
	}
	return tenancy.RestoreTenant(r)
}

// memberStore is the store picker policy: with no tenant set, a user sees
// the stores they belong to.
func (s *Store) memberStore(scope appshared.Scope, id shared.TenantID) bool {
	if !scope.TenantID.IsZero() || scope.UserID.IsZero() {
		return false
	}
	_, ok := s.members[memberKey{id, scope.UserID}]
	return ok
}

// InsertSettings implements Tenants.
func (s *Store) InsertSettings(ctx context.Context, st *tenancy.Settings) error {
	scope, err := s.enter(ctx, "InsertSettings")
	if err != nil {
		return err
	}
	defer s.mu.Unlock()
	if !sees(scope, st.TenantID()) {
		return ErrRowSecurity
	}
	if _, ok := s.tenants[st.TenantID()]; !ok {
		return fmt.Errorf("tenant_settings_tenant_id_fkey: %w", shared.ErrInvalid)
	}
	if _, ok := s.settings[st.TenantID()]; ok {
		return fmt.Errorf("tenant_settings_pkey: %w", shared.ErrConflict)
	}
	sharedfake.KeepOnRollback(ctx, &s.mu, s.settings, st.TenantID())
	s.settings[st.TenantID()] = tenancy.SettingsRecord{
		Tenant: st.TenantID(), Workflow: st.Workflow(), Editable: st.Editable(), Currency: st.Currency(), Version: st.Version(),
	}
	return nil
}

// SettingsOf implements Tenants.
func (s *Store) SettingsOf(ctx context.Context, tenant shared.TenantID) (*tenancy.Settings, error) {
	scope, err := s.enter(ctx, "SettingsOf")
	if err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	r, ok := s.settings[tenant]
	if !ok || !sees(scope, tenant) {
		return nil, shared.ErrNotFound
	}
	return tenancy.RestoreSettings(r)
}

// UpdateSettings implements Tenants.
func (s *Store) UpdateSettings(ctx context.Context, st *tenancy.Settings) error {
	scope, err := s.enter(ctx, "UpdateSettings")
	if err != nil {
		return err
	}
	defer s.mu.Unlock()
	r, ok := s.settings[st.TenantID()]
	if !ok || !sees(scope, st.TenantID()) || r.Version != st.Version() {
		return shared.ErrVersionConflict
	}
	sharedfake.KeepOnRollback(ctx, &s.mu, s.settings, st.TenantID())
	r.Editable, r.Version = st.Editable(), r.Version+1
	s.settings[st.TenantID()] = r
	return nil
}

// ---- Members ----

// InsertMembership implements Members.
func (s *Store) InsertMembership(ctx context.Context, m *tenancy.Membership) error {
	scope, err := s.enter(ctx, "InsertMembership")
	if err != nil {
		return err
	}
	defer s.mu.Unlock()
	if !sees(scope, m.TenantID()) {
		return ErrRowSecurity
	}
	if _, ok := s.tenants[m.TenantID()]; !ok {
		return fmt.Errorf("memberships_tenant_id_fkey: %w", shared.ErrInvalid)
	}
	key := memberKey{m.TenantID(), m.UserID()}
	if _, ok := s.members[key]; ok {
		return fmt.Errorf("memberships_pkey: %w", shared.ErrConflict)
	}
	for k, r := range s.members {
		if k.tenant == m.TenantID() && r.Role == shared.RoleOwner && m.Role() == shared.RoleOwner {
			return fmt.Errorf("memberships_one_owner: %w", shared.ErrConflict)
		}
	}
	sharedfake.KeepOnRollback(ctx, &s.mu, s.members, key)
	s.members[key] = tenancy.MembershipRecord{Tenant: m.TenantID(), User: m.UserID(), Role: m.Role(), Version: m.Version(), JoinedAt: m.JoinedAt()}
	return nil
}

// MembershipOf implements Members.
func (s *Store) MembershipOf(ctx context.Context, tenant shared.TenantID, user shared.UserID) (*tenancy.Membership, error) {
	return s.membershipOf(ctx, "MembershipOf", tenant, user)
}

// LockMembershipOf implements Members. The fake has nothing to lock.
func (s *Store) LockMembershipOf(ctx context.Context, tenant shared.TenantID, user shared.UserID) (*tenancy.Membership, error) {
	return s.membershipOf(ctx, "LockMembershipOf", tenant, user)
}

func (s *Store) membershipOf(ctx context.Context, method string, tenant shared.TenantID, user shared.UserID) (*tenancy.Membership, error) {
	scope, err := s.enter(ctx, method)
	if err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	r, ok := s.members[memberKey{tenant, user}]
	if !ok || (!sees(scope, tenant) && !ownMembership(scope, user)) {
		return nil, shared.ErrNotFound
	}
	return tenancy.RestoreMembership(r)
}

// ownMembership is the store picker policy on memberships: with no tenant
// set, a user sees their own memberships (ADR 0010).
func ownMembership(scope appshared.Scope, user shared.UserID) bool {
	return scope.TenantID.IsZero() && scope.ClientAccountID.IsZero() && scope.UserID == user
}

// Memberships implements Members.
func (s *Store) Memberships(ctx context.Context, tenant shared.TenantID, page shared.PageRequest) (shared.Page[*tenancy.Membership], error) {
	scope, err := s.enter(ctx, "Memberships")
	if err != nil {
		return shared.Page[*tenancy.Membership]{}, err
	}
	defer s.mu.Unlock()
	var team []tenancy.MembershipRecord
	for k, r := range s.members {
		if k.tenant == tenant && sees(scope, tenant) {
			team = append(team, r)
		}
	}
	slices.SortFunc(team, func(a, b tenancy.MembershipRecord) int {
		return cmp.Or(a.JoinedAt.Compare(b.JoinedAt), strings.Compare(a.User.String(), b.User.String()))
	})
	return pageOf(team, page, func(r tenancy.MembershipRecord) string { return r.User.String() }, tenancy.RestoreMembership)
}

// MembershipsOfUser implements Members.
func (s *Store) MembershipsOfUser(ctx context.Context, user shared.UserID) ([]apptenancy.StoreMembership, error) {
	scope, err := s.enter(ctx, "MembershipsOfUser")
	if err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	own := ownMembership(scope, user)
	out := []apptenancy.StoreMembership{}
	for k, r := range s.members {
		if k.user != user || (!own && !sees(scope, k.tenant)) {
			continue
		}
		t, err := tenancy.RestoreTenant(s.tenants[k.tenant])
		if err != nil {
			return nil, err
		}
		m, err := tenancy.RestoreMembership(r)
		if err != nil {
			return nil, err
		}
		out = append(out, apptenancy.StoreMembership{Tenant: t, Membership: m})
	}
	slices.SortFunc(out, func(a, b apptenancy.StoreMembership) int {
		return cmp.Or(strings.Compare(a.Tenant.Name().String(), b.Tenant.Name().String()), strings.Compare(a.Tenant.ID().String(), b.Tenant.ID().String()))
	})
	return out, nil
}

// UpdateRole implements Members.
func (s *Store) UpdateRole(ctx context.Context, m *tenancy.Membership) error {
	scope, err := s.enter(ctx, "UpdateRole")
	if err != nil {
		return err
	}
	defer s.mu.Unlock()
	key := memberKey{m.TenantID(), m.UserID()}
	r, ok := s.members[key]
	if !ok || !sees(scope, m.TenantID()) || r.Version != m.Version() {
		return shared.ErrVersionConflict
	}
	sharedfake.KeepOnRollback(ctx, &s.mu, s.members, key)
	r.Role, r.Version = m.Role(), r.Version+1
	s.members[key] = r
	return nil
}

// DeleteMembership implements Members.
func (s *Store) DeleteMembership(ctx context.Context, tenant shared.TenantID, user shared.UserID) error {
	scope, err := s.enter(ctx, "DeleteMembership")
	if err != nil {
		return err
	}
	defer s.mu.Unlock()
	key := memberKey{tenant, user}
	if _, ok := s.members[key]; !ok || !sees(scope, tenant) {
		return shared.ErrNotFound
	}
	sharedfake.KeepOnRollback(ctx, &s.mu, s.members, key)
	delete(s.members, key)
	return nil
}

// Member returns the stored membership, for assertions.
func (s *Store) Member(tenant shared.TenantID, user shared.UserID) (tenancy.MembershipRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.members[memberKey{tenant, user}]
	return r, ok
}

// pageOf cuts one page from sorted rows. The cursor is the key of the last
// row of the previous page; an unknown one is a ValidationError, as a real
// repository answers a cursor it did not issue.
func pageOf[R, T any](rows []R, page shared.PageRequest, key func(R) string, restore func(R) (T, error)) (shared.Page[T], error) {
	start := 0
	if c := page.Cursor(); c != "" {
		i := slices.IndexFunc(rows, func(r R) bool { return key(r) == c })
		if i < 0 {
			return shared.Page[T]{}, shared.NewValidationError(shared.FieldError{Field: "cursor", Message: "Cursor inválido."})
		}
		start = i + 1
	}
	end := min(start+page.Limit(), len(rows))
	out := shared.Page[T]{Items: make([]T, 0, end-start)}
	for _, r := range rows[start:end] {
		v, err := restore(r)
		if err != nil {
			return shared.Page[T]{}, err
		}
		out.Items = append(out.Items, v)
	}
	if end < len(rows) {
		out.NextCursor = key(rows[end-1])
	}
	return out, nil
}

// ---- Invitations ----

func invitationRecord(inv *tenancy.Invitation) tenancy.InvitationRecord {
	return tenancy.InvitationRecord{
		ID: inv.ID(), Tenant: inv.TenantID(), Email: inv.Email().String(), Role: inv.Role(), Hash: inv.Hash(),
		InvitedBy: inv.InvitedBy(), CreatedAt: inv.CreatedAt(), ExpiresAt: inv.ExpiresAt(),
		AcceptedAt: inv.AcceptedAt(), RevokedAt: inv.RevokedAt(),
	}
}

func open(r tenancy.InvitationRecord) bool { return r.AcceptedAt.IsZero() && r.RevokedAt.IsZero() }

func sameEmail(stored string, email identity.Email) bool {
	e, err := identity.ParseEmail(stored)
	return err == nil && e.Equal(email)
}

// InsertInvitation implements Invitations.
func (s *Store) InsertInvitation(ctx context.Context, inv *tenancy.Invitation) error {
	scope, err := s.enter(ctx, "InsertInvitation")
	if err != nil {
		return err
	}
	defer s.mu.Unlock()
	if !sees(scope, inv.TenantID()) {
		return ErrRowSecurity
	}
	for _, r := range s.invitations {
		if r.ID == inv.ID() || (r.Tenant == inv.TenantID() && (r.Hash == inv.Hash() || (open(r) && sameEmail(r.Email, inv.Email())))) {
			return fmt.Errorf("invitations unique key: %w", shared.ErrConflict)
		}
	}
	sharedfake.KeepOnRollback(ctx, &s.mu, s.invitations, inv.ID())
	s.invitations[inv.ID()] = invitationRecord(inv)
	return nil
}

// OpenInvitationByEmail implements Invitations.
func (s *Store) OpenInvitationByEmail(ctx context.Context, tenant shared.TenantID, email identity.Email) (*tenancy.Invitation, error) {
	return s.findInvitation(ctx, "OpenInvitationByEmail", tenant, func(r tenancy.InvitationRecord) bool {
		return open(r) && sameEmail(r.Email, email)
	})
}

// InvitationByID implements Invitations.
func (s *Store) InvitationByID(ctx context.Context, tenant shared.TenantID, id shared.InvitationID) (*tenancy.Invitation, error) {
	return s.findInvitation(ctx, "InvitationByID", tenant, func(r tenancy.InvitationRecord) bool { return r.ID == id })
}

// InvitationByHash implements Invitations.
func (s *Store) InvitationByHash(ctx context.Context, tenant shared.TenantID, hash shared.SecretHash) (*tenancy.Invitation, error) {
	return s.findInvitation(ctx, "InvitationByHash", tenant, func(r tenancy.InvitationRecord) bool { return r.Hash == hash })
}

func (s *Store) findInvitation(ctx context.Context, method string, tenant shared.TenantID, match func(tenancy.InvitationRecord) bool) (*tenancy.Invitation, error) {
	scope, err := s.enter(ctx, method)
	if err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	for _, r := range s.invitations {
		if r.Tenant == tenant && sees(scope, tenant) && match(r) {
			return tenancy.RestoreInvitation(r)
		}
	}
	return nil, shared.ErrNotFound
}

// PendingInvitations implements Invitations.
func (s *Store) PendingInvitations(ctx context.Context, tenant shared.TenantID, now time.Time, page shared.PageRequest) (shared.Page[*tenancy.Invitation], error) {
	scope, err := s.enter(ctx, "PendingInvitations")
	if err != nil {
		return shared.Page[*tenancy.Invitation]{}, err
	}
	defer s.mu.Unlock()
	var pending []tenancy.InvitationRecord
	for _, r := range s.invitations {
		if r.Tenant == tenant && sees(scope, tenant) && open(r) && now.Before(r.ExpiresAt) {
			pending = append(pending, r)
		}
	}
	slices.SortFunc(pending, func(a, b tenancy.InvitationRecord) int {
		return cmp.Or(b.CreatedAt.Compare(a.CreatedAt), strings.Compare(b.ID.String(), a.ID.String()))
	})
	return pageOf(pending, page, func(r tenancy.InvitationRecord) string { return r.ID.String() }, tenancy.RestoreInvitation)
}

// MarkInvitationRevoked implements Invitations.
func (s *Store) MarkInvitationRevoked(ctx context.Context, tenant shared.TenantID, id shared.InvitationID, at time.Time) (bool, error) {
	return s.markInvitation(ctx, "MarkInvitationRevoked", tenant, id, func(r *tenancy.InvitationRecord) bool {
		r.RevokedAt = at
		return true
	})
}

// MarkInvitationAccepted implements Invitations.
func (s *Store) MarkInvitationAccepted(ctx context.Context, tenant shared.TenantID, id shared.InvitationID, at time.Time) (bool, error) {
	return s.markInvitation(ctx, "MarkInvitationAccepted", tenant, id, func(r *tenancy.InvitationRecord) bool {
		if !at.Before(r.ExpiresAt) {
			return false
		}
		r.AcceptedAt = at
		return true
	})
}

func (s *Store) markInvitation(ctx context.Context, method string, tenant shared.TenantID, id shared.InvitationID, change func(*tenancy.InvitationRecord) bool) (bool, error) {
	scope, err := s.enter(ctx, method)
	if err != nil {
		return false, err
	}
	defer s.mu.Unlock()
	r, ok := s.invitations[id]
	if !ok || r.Tenant != tenant || !sees(scope, tenant) || !open(r) || !change(&r) {
		return false, nil
	}
	sharedfake.KeepOnRollback(ctx, &s.mu, s.invitations, id)
	s.invitations[id] = r
	return true, nil
}

// RevokeInvitationsBy implements Invitations.
func (s *Store) RevokeInvitationsBy(ctx context.Context, tenant shared.TenantID, user shared.UserID, at time.Time) ([]shared.InvitationID, error) {
	scope, err := s.enter(ctx, "RevokeInvitationsBy")
	if err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	var ids []shared.InvitationID
	for id, r := range s.invitations {
		if r.Tenant == tenant && sees(scope, tenant) && r.InvitedBy == user && open(r) {
			sharedfake.KeepOnRollback(ctx, &s.mu, s.invitations, id)
			r.RevokedAt = at
			s.invitations[id] = r
			ids = append(ids, id)
		}
	}
	slices.SortFunc(ids, func(a, b shared.InvitationID) int { return strings.Compare(a.String(), b.String()) })
	return ids, nil
}

// Invitation returns a stored invitation, for assertions.
func (s *Store) Invitation(id shared.InvitationID) (tenancy.InvitationRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.invitations[id]
	return r, ok
}

// InvitationsOf returns every stored invitation of tenant, for assertions.
func (s *Store) InvitationsOf(tenant shared.TenantID) []tenancy.InvitationRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []tenancy.InvitationRecord
	for _, r := range s.invitations {
		if r.Tenant == tenant {
			out = append(out, r)
		}
	}
	return out
}
