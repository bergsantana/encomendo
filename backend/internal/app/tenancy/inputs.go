package tenancy

import (
	"errors"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

// ErrEmailUnavailable means an invitation was saved but its email could not
// be sent. It matches no sentinel: the API answers 503 email_unavailable,
// and inviting again replaces the link (ADR 0011).
var ErrEmailUnavailable = errors.New("tenancy: invitation saved, but its email could not be sent")

// CreateTenantInput creates a store (POST /api/v1/admin/tenants).
type CreateTenantInput struct {
	Name string
	Slug string
}

// UpdateSettingsInput replaces the editable settings, with the version read
// (PUT /api/v1/admin/settings).
type UpdateSettingsInput struct {
	Settings tenancy.Editable
	Version  int
}

// ChangeRoleInput gives a member another role (PUT
// /api/v1/admin/members/{userId}/role).
type ChangeRoleInput struct {
	UserID shared.UserID
	Role   string
}

// InviteMemberInput invites an email to the team (POST
// /api/v1/admin/invitations).
type InviteMemberInput struct {
	Email string
	Role  string
}

// TenantView is a store (the Tenant schema).
type TenantView struct {
	ID        shared.TenantID
	Name      string
	Slug      string
	CreatedAt time.Time
}

// SettingsView is a store's settings (the TenantSettings schema).
type SettingsView struct {
	Editable tenancy.Editable
	Currency string
	Workflow tenancy.WorkflowRef
	Version  int
}

// MemberView is one member of the team (the Member schema).
type MemberView struct {
	UserID   shared.UserID
	Name     string
	Email    string
	Role     shared.Role
	JoinedAt time.Time
}

// InvitationView is a pending invitation (the Invitation schema). The link
// is never in it: it is only in the email.
type InvitationView struct {
	ID        shared.InvitationID
	Email     string
	Role      shared.Role
	InvitedAt time.Time
	ExpiresAt time.Time
}

func tenantView(t *tenancy.Tenant) TenantView {
	return TenantView{ID: t.ID(), Name: t.Name().String(), Slug: t.Slug().String(), CreatedAt: t.CreatedAt()}
}

func settingsView(s *tenancy.Settings, version int) SettingsView {
	return SettingsView{Editable: s.Editable(), Currency: s.Currency(), Workflow: s.Workflow(), Version: version}
}

func invitationView(inv *tenancy.Invitation) InvitationView {
	return InvitationView{
		ID: inv.ID(), Email: inv.Email().String(), Role: inv.Role(), InvitedAt: inv.CreatedAt(), ExpiresAt: inv.ExpiresAt(),
	}
}
