package dto

import (
	"encoding/json"
	"time"

	apptenancy "github.com/bergsantana/encomendo/backend/internal/app/tenancy"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

// CreateTenantRequest is POST /api/v1/admin/tenants.
type CreateTenantRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// Tenant is the Tenant schema.
type Tenant struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"createdAt"`
}

// UpdateSettingsRequest is PUT /api/v1/admin/settings. The contract asks
// for every field, so each is a pointer and a missing one is reported
// instead of read as false or zero.
type UpdateSettingsRequest struct {
	StockEnabled          *bool `json:"stockEnabled"`
	ProductionEnabled     *bool `json:"productionEnabled"`
	PurchaseBeforeDeposit *bool `json:"purchaseBeforeDeposit"`
	DepositPercent        *int  `json:"depositPercent"`
	AssemblyDays          *int  `json:"assemblyDays"`
	SafetyMarginDays      *int  `json:"safetyMarginDays"`
	// WhatsAppPhone is required but may be null for "no number".
	WhatsAppPhone nullableString `json:"whatsappPhone"`
	Version       *int           `json:"version"`
}

// nullableString tells a JSON null from a missing field.
type nullableString struct {
	set   bool
	value *string
}

// UnmarshalJSON records that the field was sent, null or not.
func (n *nullableString) UnmarshalJSON(b []byte) error {
	n.set = true
	if string(b) == "null" {
		n.value = nil
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	n.value = &s
	return nil
}

// Input returns the use case's input, or a ValidationError naming every
// missing field.
func (r UpdateSettingsRequest) Input() (apptenancy.UpdateSettingsInput, error) {
	var missing []shared.FieldError
	need := func(set bool, field string) {
		if !set {
			missing = append(missing, shared.FieldError{Field: field, Message: "Obrigatório."})
		}
	}
	need(r.StockEnabled != nil, "stockEnabled")
	need(r.ProductionEnabled != nil, "productionEnabled")
	need(r.PurchaseBeforeDeposit != nil, "purchaseBeforeDeposit")
	need(r.DepositPercent != nil, "depositPercent")
	need(r.AssemblyDays != nil, "assemblyDays")
	need(r.SafetyMarginDays != nil, "safetyMarginDays")
	need(r.WhatsAppPhone.set, "whatsappPhone")
	need(r.Version != nil, "version")
	if len(missing) > 0 {
		return apptenancy.UpdateSettingsInput{}, shared.NewValidationError(missing...)
	}
	phone := ""
	if r.WhatsAppPhone.value != nil {
		phone = *r.WhatsAppPhone.value
	}
	return apptenancy.UpdateSettingsInput{
		Settings: tenancy.Editable{
			StockEnabled: *r.StockEnabled, ProductionEnabled: *r.ProductionEnabled,
			PurchaseBeforeDeposit: *r.PurchaseBeforeDeposit, DepositPercent: *r.DepositPercent,
			AssemblyDays: *r.AssemblyDays, SafetyMarginDays: *r.SafetyMarginDays, WhatsAppPhone: phone,
		},
		Version: *r.Version,
	}, nil
}

// TenantSettings is the TenantSettings schema.
type TenantSettings struct {
	StockEnabled          bool              `json:"stockEnabled"`
	ProductionEnabled     bool              `json:"productionEnabled"`
	PurchaseBeforeDeposit bool              `json:"purchaseBeforeDeposit"`
	DepositPercent        int               `json:"depositPercent"`
	AssemblyDays          int               `json:"assemblyDays"`
	SafetyMarginDays      int               `json:"safetyMarginDays"`
	WhatsAppPhone         *string           `json:"whatsappPhone"`
	Currency              string            `json:"currency"`
	StatusWorkflow        StatusWorkflowRef `json:"statusWorkflow"`
	Version               int               `json:"version"`
}

// StatusWorkflowRef is the StatusWorkflowRef schema.
type StatusWorkflowRef struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Name    string `json:"name"`
}

// ChangeRoleRequest is PUT /api/v1/admin/members/{userId}/role.
type ChangeRoleRequest struct {
	Role string `json:"role"`
}

// InviteMemberRequest is POST /api/v1/admin/invitations.
type InviteMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// Member is the Member schema.
type Member struct {
	UserID   string    `json:"userId"`
	Name     string    `json:"name"`
	Email    string    `json:"email"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joinedAt"`
}

// Invitation is the Invitation schema. It never carries the link: that is
// only in the email (ADR 0011).
type Invitation struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	InvitedAt time.Time `json:"invitedAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// Page is the MemberPage and InvitationPage schemas: nextCursor is absent
// on the last page, and items is never null.
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// NewTenant maps a store.
func NewTenant(t apptenancy.TenantView) Tenant {
	return Tenant{ID: t.ID.String(), Name: t.Name, Slug: t.Slug, CreatedAt: t.CreatedAt.UTC()}
}

// NewTenantSettings maps a store's settings; no WhatsApp number is null.
func NewTenantSettings(s apptenancy.SettingsView) TenantSettings {
	var phone *string
	if s.Editable.WhatsAppPhone != "" {
		p := s.Editable.WhatsAppPhone
		phone = &p
	}
	return TenantSettings{
		StockEnabled: s.Editable.StockEnabled, ProductionEnabled: s.Editable.ProductionEnabled,
		PurchaseBeforeDeposit: s.Editable.PurchaseBeforeDeposit, DepositPercent: s.Editable.DepositPercent,
		AssemblyDays: s.Editable.AssemblyDays, SafetyMarginDays: s.Editable.SafetyMarginDays,
		WhatsAppPhone: phone, Currency: s.Currency,
		StatusWorkflow: StatusWorkflowRef{ID: s.Workflow.ID.String(), Version: s.Workflow.Version, Name: s.Workflow.Name},
		Version:        s.Version,
	}
}

// NewMember maps a team member.
func NewMember(m apptenancy.MemberView) Member {
	return Member{UserID: m.UserID.String(), Name: m.Name, Email: m.Email, Role: string(m.Role), JoinedAt: m.JoinedAt.UTC()}
}

// NewInvitation maps a pending invitation.
func NewInvitation(i apptenancy.InvitationView) Invitation {
	return Invitation{
		ID: i.ID.String(), Email: i.Email, Role: string(i.Role), InvitedAt: i.InvitedAt.UTC(), ExpiresAt: i.ExpiresAt.UTC(),
	}
}

// NewPage maps one page of a list with each.
func NewPage[V, T any](p shared.Page[V], each func(V) T) Page[T] {
	out := Page[T]{Items: make([]T, 0, len(p.Items)), NextCursor: p.NextCursor}
	for _, v := range p.Items {
		out.Items = append(out.Items, each(v))
	}
	return out
}
