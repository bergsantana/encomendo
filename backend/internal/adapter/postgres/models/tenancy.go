package models

import (
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

// Tenant maps tenants.
type Tenant struct {
	ID        string    `gorm:"column:id;primaryKey"`
	Slug      string    `gorm:"column:slug"`
	Name      string    `gorm:"column:name"`
	Status    string    `gorm:"column:status"`
	Version   int       `gorm:"column:version"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

// TableName returns tenants.
func (Tenant) TableName() string { return "tenants" }

// TenantFromDomain returns the row of t.
func TenantFromDomain(t *tenancy.Tenant) Tenant {
	return Tenant{
		ID: t.ID().String(), Slug: t.Slug().String(), Name: t.Name().String(), Status: string(t.Status()),
		Version: t.Version(), CreatedAt: t.CreatedAt(),
	}
}

// ToDomain rebuilds the store.
func (m Tenant) ToDomain() (*tenancy.Tenant, error) {
	id, err := parseID[shared.TenantID](m.ID)
	if err != nil {
		return nil, err
	}
	return tenancy.RestoreTenant(tenancy.TenantRecord{
		ID: id, Slug: m.Slug, Name: m.Name, Status: tenancy.Status(m.Status), Version: m.Version, CreatedAt: m.CreatedAt.UTC(),
	})
}

// Settings maps tenant_settings. WorkflowName is read through a join and
// never written.
type Settings struct {
	TenantID              string  `gorm:"column:tenant_id;primaryKey"`
	StatusWorkflowID      string  `gorm:"column:status_workflow_id"`
	StatusWorkflowVersion int     `gorm:"column:status_workflow_version"`
	StockEnabled          bool    `gorm:"column:stock_enabled"`
	ProductionEnabled     bool    `gorm:"column:production_enabled"`
	PurchaseBeforeDeposit bool    `gorm:"column:purchase_before_deposit"`
	DepositPercent        int     `gorm:"column:deposit_percent"`
	AssemblyDays          int     `gorm:"column:assembly_days"`
	SafetyMarginDays      int     `gorm:"column:safety_margin_days"`
	Currency              string  `gorm:"column:currency"`
	WhatsAppPhone         *string `gorm:"column:whatsapp_phone"`
	Version               int     `gorm:"column:version"`
	WorkflowName          string  `gorm:"->;column:workflow_name"`
}

// TableName returns tenant_settings.
func (Settings) TableName() string { return "tenant_settings" }

// SettingsFromDomain returns the row of s.
func SettingsFromDomain(s *tenancy.Settings) Settings {
	e := s.Editable()
	m := Settings{
		TenantID: s.TenantID().String(), StatusWorkflowID: shared.ID(s.Workflow().ID).String(),
		StatusWorkflowVersion: s.Workflow().Version, StockEnabled: e.StockEnabled, ProductionEnabled: e.ProductionEnabled,
		PurchaseBeforeDeposit: e.PurchaseBeforeDeposit, DepositPercent: e.DepositPercent, AssemblyDays: e.AssemblyDays,
		SafetyMarginDays: e.SafetyMarginDays, Currency: s.Currency(), Version: s.Version(),
	}
	if e.WhatsAppPhone != "" {
		m.WhatsAppPhone = &e.WhatsAppPhone
	}
	return m
}

// ToDomain rebuilds the settings, with the workflow's name.
func (m Settings) ToDomain() (*tenancy.Settings, error) {
	tenant, err := parseID[shared.TenantID](m.TenantID)
	if err != nil {
		return nil, err
	}
	wf, err := parseID[shared.WorkflowID](m.StatusWorkflowID)
	if err != nil {
		return nil, err
	}
	e := tenancy.Editable{
		StockEnabled: m.StockEnabled, ProductionEnabled: m.ProductionEnabled, PurchaseBeforeDeposit: m.PurchaseBeforeDeposit,
		DepositPercent: m.DepositPercent, AssemblyDays: m.AssemblyDays, SafetyMarginDays: m.SafetyMarginDays,
	}
	if m.WhatsAppPhone != nil {
		e.WhatsAppPhone = *m.WhatsAppPhone
	}
	return tenancy.RestoreSettings(tenancy.SettingsRecord{
		Tenant: tenant, Workflow: tenancy.WorkflowRef{ID: wf, Version: m.StatusWorkflowVersion, Name: m.WorkflowName},
		Editable: e, Currency: m.Currency, Version: m.Version,
	})
}

// Membership maps memberships.
type Membership struct {
	TenantID  string    `gorm:"column:tenant_id;primaryKey"`
	UserID    string    `gorm:"column:user_id;primaryKey"`
	Role      string    `gorm:"column:role"`
	Version   int       `gorm:"column:version"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

// TableName returns memberships.
func (Membership) TableName() string { return "memberships" }

// MembershipFromDomain returns the row of m.
func MembershipFromDomain(m *tenancy.Membership) Membership {
	return Membership{
		TenantID: m.TenantID().String(), UserID: m.UserID().String(), Role: string(m.Role()),
		Version: m.Version(), CreatedAt: m.JoinedAt(),
	}
}

// ToDomain rebuilds the membership.
func (m Membership) ToDomain() (*tenancy.Membership, error) {
	tenant, err := parseID[shared.TenantID](m.TenantID)
	if err != nil {
		return nil, err
	}
	user, err := parseID[shared.UserID](m.UserID)
	if err != nil {
		return nil, err
	}
	return tenancy.RestoreMembership(tenancy.MembershipRecord{
		Tenant: tenant, User: user, Role: shared.Role(m.Role), Version: m.Version, JoinedAt: m.CreatedAt.UTC(),
	})
}

// Invitation maps invitations.
type Invitation struct {
	TenantID   string     `gorm:"column:tenant_id"`
	ID         string     `gorm:"column:id;primaryKey"`
	Email      string     `gorm:"column:email"`
	Role       string     `gorm:"column:role"`
	TokenHash  []byte     `gorm:"column:token_hash"`
	InvitedBy  string     `gorm:"column:invited_by"`
	ExpiresAt  time.Time  `gorm:"column:expires_at"`
	AcceptedAt *time.Time `gorm:"column:accepted_at"`
	RevokedAt  *time.Time `gorm:"column:revoked_at"`
	CreatedAt  time.Time  `gorm:"column:created_at"`
}

// TableName returns invitations.
func (Invitation) TableName() string { return "invitations" }

// InvitationFromDomain returns the row of inv.
func InvitationFromDomain(inv *tenancy.Invitation) Invitation {
	h := inv.Hash()
	return Invitation{
		TenantID: inv.TenantID().String(), ID: shared.ID(inv.ID()).String(), Email: inv.Email().String(),
		Role: string(inv.Role()), TokenHash: h[:], InvitedBy: inv.InvitedBy().String(), ExpiresAt: inv.ExpiresAt(),
		AcceptedAt: nullTime(inv.AcceptedAt()), RevokedAt: nullTime(inv.RevokedAt()), CreatedAt: inv.CreatedAt(),
	}
}

// ToDomain rebuilds the invitation.
func (m Invitation) ToDomain() (*tenancy.Invitation, error) {
	id, err := parseID[shared.InvitationID](m.ID)
	if err != nil {
		return nil, err
	}
	tenant, err := parseID[shared.TenantID](m.TenantID)
	if err != nil {
		return nil, err
	}
	by, err := parseID[shared.UserID](m.InvitedBy)
	if err != nil {
		return nil, err
	}
	rec := tenancy.InvitationRecord{
		ID: id, Tenant: tenant, Email: m.Email, Role: shared.Role(m.Role), InvitedBy: by,
		CreatedAt: m.CreatedAt.UTC(), ExpiresAt: m.ExpiresAt.UTC(), AcceptedAt: zeroTime(m.AcceptedAt), RevokedAt: zeroTime(m.RevokedAt),
	}
	copy(rec.Hash[:], m.TokenHash)
	return tenancy.RestoreInvitation(rec)
}

// Workflow maps the columns of status_workflows a store needs.
type Workflow struct {
	ID      string `gorm:"column:id"`
	Version int    `gorm:"column:version"`
	Name    string `gorm:"column:name"`
}

// TableName returns status_workflows.
func (Workflow) TableName() string { return "status_workflows" }

// ToDomain returns the workflow reference.
func (m Workflow) ToDomain() (tenancy.WorkflowRef, error) {
	id, err := parseID[shared.WorkflowID](m.ID)
	if err != nil {
		return tenancy.WorkflowRef{}, err
	}
	return tenancy.WorkflowRef{ID: id, Version: m.Version, Name: m.Name}, nil
}
