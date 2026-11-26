package postgres

import (
	"cmp"
	"context"
	"slices"
	"time"

	"gorm.io/gorm"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres/models"
	apptenancy "github.com/bergsantana/encomendo/backend/internal/app/tenancy"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

// Tenants implements apptenancy.Tenants on tenants and tenant_settings. RLS
// shows a store only to a scope of that store, or of a member's own user
// (0007), and every query filters on the tenant besides.
type Tenants struct{}

// NewTenants returns the stores repository.
func NewTenants() *Tenants { return &Tenants{} }

// InsertTenant stores a new store; a taken slug is an error matching
// shared.ErrConflict.
func (*Tenants) InsertTenant(ctx context.Context, t *tenancy.Tenant) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	m := models.TenantFromDomain(t)
	return wrap("insert tenant", db.Create(&m).Error)
}

// TenantByID returns the store.
func (*Tenants) TenantByID(ctx context.Context, id shared.TenantID) (*tenancy.Tenant, error) {
	db, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	var m models.Tenant
	if err := db.Where("id = ?", id.String()).Take(&m).Error; err != nil {
		return nil, wrap("read tenant", err)
	}
	return m.ToDomain()
}

// InsertSettings stores a new store's settings.
func (*Tenants) InsertSettings(ctx context.Context, s *tenancy.Settings) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	m := models.SettingsFromDomain(s)
	return wrap("insert settings", db.Create(&m).Error)
}

// SettingsOf returns the store's settings with its workflow's name.
func (*Tenants) SettingsOf(ctx context.Context, tenant shared.TenantID) (*tenancy.Settings, error) {
	db, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	var m models.Settings
	err = db.Table("tenant_settings AS s").
		Select("s.*, w.name AS workflow_name").
		Joins("JOIN status_workflows w ON w.id = s.status_workflow_id AND w.version = s.status_workflow_version").
		Where("s.tenant_id = ?", tenant.String()).Take(&m).Error
	if err != nil {
		return nil, wrap("read settings", err)
	}
	return m.ToDomain()
}

// UpdateSettings stores the editable settings while the stored version is
// still s.Version(), and the next one.
func (*Tenants) UpdateSettings(ctx context.Context, s *tenancy.Settings) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	m := models.SettingsFromDomain(s)
	res := db.Model(&models.Settings{}).
		Where("tenant_id = ? AND version = ?", m.TenantID, s.Version()).
		Updates(map[string]any{
			"stock_enabled": m.StockEnabled, "production_enabled": m.ProductionEnabled,
			"purchase_before_deposit": m.PurchaseBeforeDeposit, "deposit_percent": m.DepositPercent,
			"assembly_days": m.AssemblyDays, "safety_margin_days": m.SafetyMarginDays,
			"whatsapp_phone": m.WhatsAppPhone, "version": s.Version() + 1, "updated_at": gorm.Expr("now()"),
		})
	ok, err := affected(res, "update settings")
	if err != nil {
		return err
	}
	if !ok {
		return shared.ErrVersionConflict
	}
	return nil
}

// Members implements apptenancy.Members on memberships.
type Members struct{}

// NewMembers returns the teams repository.
func NewMembers() *Members { return &Members{} }

// InsertMembership stores a new member; being a member already, or a second
// owner, is an error matching shared.ErrConflict.
func (*Members) InsertMembership(ctx context.Context, m *tenancy.Membership) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	row := models.MembershipFromDomain(m)
	return wrap("insert membership", db.Create(&row).Error)
}

// MembershipOf returns the user's membership in the store.
func (*Members) MembershipOf(ctx context.Context, tenant shared.TenantID, user shared.UserID) (*tenancy.Membership, error) {
	return takeMembership(ctx, false, tenant, user)
}

// LockMembershipOf is MembershipOf with the row locked FOR SHARE.
func (*Members) LockMembershipOf(ctx context.Context, tenant shared.TenantID, user shared.UserID) (*tenancy.Membership, error) {
	return takeMembership(ctx, true, tenant, user)
}

func takeMembership(ctx context.Context, lock bool, tenant shared.TenantID, user shared.UserID) (*tenancy.Membership, error) {
	db, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	if lock {
		db = db.Clauses(forShare)
	}
	var m models.Membership
	if err := db.Where("tenant_id = ? AND user_id = ?", tenant.String(), user.String()).Take(&m).Error; err != nil {
		return nil, wrap("read membership", err)
	}
	return m.ToDomain()
}

// Memberships returns one page of the team, oldest member first.
func (*Members) Memberships(ctx context.Context, tenant shared.TenantID, page shared.PageRequest) (shared.Page[*tenancy.Membership], error) {
	db, err := conn(ctx)
	if err != nil {
		return shared.Page[*tenancy.Membership]{}, err
	}
	after, paged, err := parseCursor(page.Cursor())
	if err != nil {
		return shared.Page[*tenancy.Membership]{}, err
	}
	q := db.Where("tenant_id = ?", tenant.String()).Order("created_at, user_id").Limit(page.Limit() + 1)
	if paged {
		q = q.Where("(created_at, user_id) > (?, ?)", after.time(), after.id)
	}
	var rows []models.Membership
	if err := q.Find(&rows).Error; err != nil {
		return shared.Page[*tenancy.Membership]{}, wrap("list memberships", err)
	}
	return pageOf(rows, page, func(m models.Membership) position { return positionOf(m.CreatedAt, m.UserID) },
		models.Membership.ToDomain)
}

// MembershipsOfUser returns every membership of the user with its store,
// sorted by store name, then id.
func (*Members) MembershipsOfUser(ctx context.Context, user shared.UserID) ([]apptenancy.StoreMembership, error) {
	db, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	var rows []models.Membership
	if err := db.Where("user_id = ?", user.String()).Find(&rows).Error; err != nil {
		return nil, wrap("list memberships of user", err)
	}
	if len(rows) == 0 {
		return []apptenancy.StoreMembership{}, nil
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.TenantID
	}
	var stores []models.Tenant
	if err := db.Where("id IN ?", ids).Find(&stores).Error; err != nil {
		return nil, wrap("read stores of user", err)
	}
	return joinStores(rows, stores)
}

// joinStores pairs each membership with its store and sorts them by store
// name, then id. A store the scope cannot read is no membership to show.
func joinStores(rows []models.Membership, stores []models.Tenant) ([]apptenancy.StoreMembership, error) {
	byID := make(map[string]models.Tenant, len(stores))
	for _, s := range stores {
		byID[s.ID] = s
	}
	out := make([]apptenancy.StoreMembership, 0, len(rows))
	for _, r := range rows {
		s, ok := byID[r.TenantID]
		if !ok {
			continue
		}
		t, err := s.ToDomain()
		if err != nil {
			return nil, err
		}
		m, err := r.ToDomain()
		if err != nil {
			return nil, err
		}
		out = append(out, apptenancy.StoreMembership{Tenant: t, Membership: m})
	}
	slices.SortFunc(out, func(a, b apptenancy.StoreMembership) int {
		if c := cmp.Compare(a.Tenant.Name().String(), b.Tenant.Name().String()); c != 0 {
			return c
		}
		return cmp.Compare(a.Tenant.ID().String(), b.Tenant.ID().String())
	})
	return out, nil
}

// UpdateRole stores m's role while the stored version is still m.Version().
func (*Members) UpdateRole(ctx context.Context, m *tenancy.Membership) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	res := db.Model(&models.Membership{}).
		Where("tenant_id = ? AND user_id = ? AND version = ?", m.TenantID().String(), m.UserID().String(), m.Version()).
		Updates(map[string]any{"role": string(m.Role()), "version": m.Version() + 1, "updated_at": gorm.Expr("now()")})
	ok, err := affected(res, "update role")
	if err != nil {
		return err
	}
	if !ok {
		return shared.ErrVersionConflict
	}
	return nil
}

// DeleteMembership removes the user from the store.
func (*Members) DeleteMembership(ctx context.Context, tenant shared.TenantID, user shared.UserID) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	res := db.Where("tenant_id = ? AND user_id = ?", tenant.String(), user.String()).Delete(&models.Membership{})
	ok, err := affected(res, "delete membership")
	if err != nil {
		return err
	}
	if !ok {
		return notFound("delete membership")
	}
	return nil
}

// Invitations implements apptenancy.Invitations on invitations.
type Invitations struct{}

// NewInvitations returns the invitations repository.
func NewInvitations() *Invitations { return &Invitations{} }

const openInvitation = "accepted_at IS NULL AND revoked_at IS NULL"

// InsertInvitation stores a new invitation; an open one for the same email
// is an error matching shared.ErrConflict.
func (*Invitations) InsertInvitation(ctx context.Context, inv *tenancy.Invitation) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	m := models.InvitationFromDomain(inv)
	return wrap("insert invitation", db.Create(&m).Error)
}

// OpenInvitationByEmail returns the email's open invitation, expired or not.
func (*Invitations) OpenInvitationByEmail(ctx context.Context, tenant shared.TenantID, email identity.Email) (*tenancy.Invitation, error) {
	return takeInvitation(ctx, "tenant_id = ? AND email = ? AND "+openInvitation, tenant.String(), email.String())
}

// InvitationByID returns the invitation in any state.
func (*Invitations) InvitationByID(ctx context.Context, tenant shared.TenantID, id shared.InvitationID) (*tenancy.Invitation, error) {
	return takeInvitation(ctx, "tenant_id = ? AND id = ?", tenant.String(), shared.ID(id).String())
}

// InvitationByHash returns the invitation whose link's secret has hash.
func (*Invitations) InvitationByHash(ctx context.Context, tenant shared.TenantID, hash shared.SecretHash) (*tenancy.Invitation, error) {
	return takeInvitation(ctx, "tenant_id = ? AND token_hash = ?", tenant.String(), hash[:])
}

func takeInvitation(ctx context.Context, where string, args ...any) (*tenancy.Invitation, error) {
	db, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	var m models.Invitation
	if err := db.Where(where, args...).Take(&m).Error; err != nil {
		return nil, wrap("read invitation", err)
	}
	return m.ToDomain()
}

// PendingInvitations returns one page of the invitations pending at now,
// newest first.
func (*Invitations) PendingInvitations(ctx context.Context, tenant shared.TenantID, now time.Time, page shared.PageRequest) (shared.Page[*tenancy.Invitation], error) {
	db, err := conn(ctx)
	if err != nil {
		return shared.Page[*tenancy.Invitation]{}, err
	}
	after, paged, err := parseCursor(page.Cursor())
	if err != nil {
		return shared.Page[*tenancy.Invitation]{}, err
	}
	q := db.Where("tenant_id = ? AND "+openInvitation+" AND expires_at > ?", tenant.String(), now).
		Order("created_at DESC, id DESC").Limit(page.Limit() + 1)
	if paged {
		q = q.Where("(created_at, id) < (?, ?)", after.time(), after.id)
	}
	var rows []models.Invitation
	if err := q.Find(&rows).Error; err != nil {
		return shared.Page[*tenancy.Invitation]{}, wrap("list invitations", err)
	}
	return pageOf(rows, page, func(m models.Invitation) position { return positionOf(m.CreatedAt, m.ID) },
		models.Invitation.ToDomain)
}

// MarkInvitationRevoked revokes the invitation while it is open.
func (*Invitations) MarkInvitationRevoked(ctx context.Context, tenant shared.TenantID, id shared.InvitationID, at time.Time) (bool, error) {
	db, err := conn(ctx)
	if err != nil {
		return false, err
	}
	res := db.Model(&models.Invitation{}).Where("tenant_id = ? AND id = ? AND "+openInvitation, tenant.String(), shared.ID(id).String()).
		Updates(map[string]any{"revoked_at": at, "updated_at": gorm.Expr("now()")})
	return affected(res, "revoke invitation")
}

// MarkInvitationAccepted uses the invitation up while it is open and
// unexpired at at.
func (*Invitations) MarkInvitationAccepted(ctx context.Context, tenant shared.TenantID, id shared.InvitationID, at time.Time) (bool, error) {
	db, err := conn(ctx)
	if err != nil {
		return false, err
	}
	res := db.Model(&models.Invitation{}).Where("tenant_id = ? AND id = ? AND "+openInvitation+" AND expires_at > ?", tenant.String(), shared.ID(id).String(), at).
		Updates(map[string]any{"accepted_at": at, "updated_at": gorm.Expr("now()")})
	return affected(res, "accept invitation")
}

// RevokeInvitationsBy revokes every open invitation the user sent and
// returns their ids.
func (*Invitations) RevokeInvitationsBy(ctx context.Context, tenant shared.TenantID, user shared.UserID, at time.Time) ([]shared.InvitationID, error) {
	db, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	// Raw, because the builder has no UPDATE ... RETURNING that scans ids.
	var raw []string
	err = db.Raw(`UPDATE invitations SET revoked_at = ?, updated_at = now()
		WHERE tenant_id = ? AND invited_by = ? AND `+openInvitation+` RETURNING id`, at, tenant.String(), user.String()).Scan(&raw).Error
	if err != nil {
		return nil, wrap("revoke invitations by sender", err)
	}
	slices.Sort(raw)
	out := make([]shared.InvitationID, 0, len(raw))
	for _, s := range raw {
		id, err := shared.ParseInvitationID(s)
		if err != nil {
			return nil, wrap("revoke invitations by sender", err)
		}
		out = append(out, id)
	}
	return out, nil
}

// WorkflowPresets implements apptenancy.WorkflowPresets on status_workflows.
type WorkflowPresets struct{}

// NewWorkflowPresets returns the preset reader.
func NewWorkflowPresets() *WorkflowPresets { return &WorkflowPresets{} }

// DefaultWorkflow returns the preset new stores start with (ADR 0006).
func (*WorkflowPresets) DefaultWorkflow(ctx context.Context) (tenancy.WorkflowRef, error) {
	db, err := conn(ctx)
	if err != nil {
		return tenancy.WorkflowRef{}, err
	}
	var m models.Workflow
	if err := db.Where("is_default").Take(&m).Error; err != nil {
		return tenancy.WorkflowRef{}, wrap("read default workflow", err)
	}
	return m.ToDomain()
}

// pageOf turns a keyset query's rows, fetched with limit+1, into a page:
// the cursor is the position of the last row kept.
func pageOf[R, T any](rows []R, page shared.PageRequest, pos func(R) position, restore func(R) (T, error)) (shared.Page[T], error) {
	more := len(rows) > page.Limit()
	if more {
		rows = rows[:page.Limit()]
	}
	out := shared.Page[T]{Items: make([]T, 0, len(rows))}
	for _, r := range rows {
		v, err := restore(r)
		if err != nil {
			return shared.Page[T]{}, err
		}
		out.Items = append(out.Items, v)
	}
	if more {
		out.NextCursor = pos(rows[len(rows)-1]).cursor()
	}
	return out, nil
}
