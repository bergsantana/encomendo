package tenancy

import (
	"context"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

// UseCases is the driving port: what the tenancy and members handlers call.
type UseCases interface {
	CreateTenant(ctx context.Context, in CreateTenantInput) (TenantView, error)
	Settings(ctx context.Context) (SettingsView, error)
	UpdateSettings(ctx context.Context, in UpdateSettingsInput) (SettingsView, error)
	Members(ctx context.Context, page shared.PageRequest) (shared.Page[MemberView], error)
	ChangeRole(ctx context.Context, in ChangeRoleInput) (MemberView, error)
	RemoveMember(ctx context.Context, user shared.UserID) error
	Invitations(ctx context.Context, page shared.PageRequest) (shared.Page[InvitationView], error)
	InviteMember(ctx context.Context, in InviteMemberInput) (InvitationView, error)
	RevokeInvitation(ctx context.Context, id shared.InvitationID) error
}

// The repository ports. Every table behind them is tenant-scoped under RLS
// (0007_rls_policies), so a method sees only the rows of the tenant in the
// transaction's scope: with another tenant's id it finds nothing and writes
// nothing. The methods also take the tenant explicitly, so the SQL filters
// on it too. Every method runs inside TxManager.WithinTx and returns
// appshared.ErrNoTransaction outside it. A missing row is shared.ErrNotFound;
// other failures pass through unchanged.

// Tenants stores stores and their settings.
type Tenants interface {
	// InsertTenant stores a new store, in a scope of that store. It returns
	// an error matching shared.ErrConflict when the slug is taken.
	InsertTenant(ctx context.Context, t *tenancy.Tenant) error
	// TenantByID returns the store.
	TenantByID(ctx context.Context, id shared.TenantID) (*tenancy.Tenant, error)
	// InsertSettings stores a new store's settings.
	InsertSettings(ctx context.Context, s *tenancy.Settings) error
	// SettingsOf returns the store's settings, with its workflow's name.
	SettingsOf(ctx context.Context, tenant shared.TenantID) (*tenancy.Settings, error)
	// UpdateSettings stores s's editable settings while the stored version
	// is still s.Version(), and stores the next version. It returns
	// shared.ErrVersionConflict otherwise.
	UpdateSettings(ctx context.Context, s *tenancy.Settings) error
}

// StoreMembership is one of a user's memberships, with its store.
type StoreMembership struct {
	Tenant     *tenancy.Tenant
	Membership *tenancy.Membership
}

// Members stores the teams.
type Members interface {
	// InsertMembership stores a new member. It returns an error matching
	// shared.ErrConflict when the user is already a member, or when a
	// second owner would result.
	InsertMembership(ctx context.Context, m *tenancy.Membership) error
	// MembershipOf returns the user's membership in the store.
	MembershipOf(ctx context.Context, tenant shared.TenantID, user shared.UserID) (*tenancy.Membership, error)
	// Memberships returns one page of the store's team, oldest member
	// first.
	Memberships(ctx context.Context, tenant shared.TenantID, page shared.PageRequest) (shared.Page[*tenancy.Membership], error)
	// MembershipsOfUser returns every membership of the user with its
	// store, sorted by store name, then id. Only a scope of that user alone
	// shows all of them (ADR 0010).
	MembershipsOfUser(ctx context.Context, user shared.UserID) ([]StoreMembership, error)
	// UpdateRole stores m's role while the stored version is still
	// m.Version(), and stores the next version. It returns
	// shared.ErrVersionConflict otherwise.
	UpdateRole(ctx context.Context, m *tenancy.Membership) error
	// DeleteMembership removes the user from the store. It returns
	// shared.ErrNotFound when they were not a member.
	DeleteMembership(ctx context.Context, tenant shared.TenantID, user shared.UserID) error
}

// Invitations stores invitations. An invitation is open until it is
// accepted or revoked, and pending while it is open and unexpired; a store
// has at most one open invitation per email (0004_tenancy).
type Invitations interface {
	// InsertInvitation stores a new invitation. It returns an error
	// matching shared.ErrConflict when the email has an open one.
	InsertInvitation(ctx context.Context, inv *tenancy.Invitation) error
	// OpenInvitationByEmail returns the email's open invitation, expired or
	// not.
	OpenInvitationByEmail(ctx context.Context, tenant shared.TenantID, email identity.Email) (*tenancy.Invitation, error)
	// InvitationByID returns the invitation, in any state.
	InvitationByID(ctx context.Context, tenant shared.TenantID, id shared.InvitationID) (*tenancy.Invitation, error)
	// InvitationByHash returns the invitation whose link's secret has hash,
	// in any state.
	InvitationByHash(ctx context.Context, tenant shared.TenantID, hash shared.SecretHash) (*tenancy.Invitation, error)
	// PendingInvitations returns one page of the invitations pending at
	// now, newest first.
	PendingInvitations(ctx context.Context, tenant shared.TenantID, now time.Time, page shared.PageRequest) (shared.Page[*tenancy.Invitation], error)
	// MarkInvitationRevoked revokes the invitation at at, only while it is
	// open, and reports whether it did.
	MarkInvitationRevoked(ctx context.Context, tenant shared.TenantID, id shared.InvitationID, at time.Time) (bool, error)
	// MarkInvitationAccepted uses the invitation up at at, only while it is
	// open and unexpired at at, and reports whether it did.
	MarkInvitationAccepted(ctx context.Context, tenant shared.TenantID, id shared.InvitationID, at time.Time) (bool, error)
	// RevokeInvitationsBy revokes every open invitation the user sent and
	// returns their ids.
	RevokeInvitationsBy(ctx context.Context, tenant shared.TenantID, user shared.UserID, at time.Time) ([]shared.InvitationID, error)
}

// UserSummary is a staff account as tenancy shows it.
type UserSummary struct {
	ID    shared.UserID
	Name  string
	Email string
}

// UserDirectory reads staff accounts (identity, through adapter/inproc; ADR
// 0011). Its methods run inside the caller's transaction.
type UserDirectory interface {
	// UserByEmail returns the account with this email, ignoring case, or an
	// error matching shared.ErrNotFound.
	UserByEmail(ctx context.Context, email identity.Email) (UserSummary, error)
	// UsersByID returns the accounts with these ids that exist, in any
	// order.
	UsersByID(ctx context.Context, ids []shared.UserID) ([]UserSummary, error)
}

// WorkflowPresets reads the global status workflow presets (0003_workflows;
// the workflow context of milestone 4 takes this over).
type WorkflowPresets interface {
	// DefaultWorkflow returns the preset new stores start with (ADR 0006).
	DefaultWorkflow(ctx context.Context) (tenancy.WorkflowRef, error)
}

// InvitationEmail is the "Convite" message. The mailer builds the link
// (https://{admin host}/convite#token=…) from Token; the fragment never
// reaches a server log (ADR 0011).
type InvitationEmail struct {
	To        identity.Email
	Store     string
	InvitedBy string
	Role      shared.Role
	Token     tenancy.InvitationToken
	ExpiresAt time.Time
}

// InvitationMailer sends invitations (adapter/notify/smtp.go).
type InvitationMailer interface {
	// SendInvitation sends the email before it returns, and returns an
	// error when it could not be sent.
	SendInvitation(ctx context.Context, e InvitationEmail) error
}
