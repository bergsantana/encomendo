package tenancy

import (
	"context"
	"errors"
	"fmt"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

// Deps are the Service's and the Lookup's ports. Every one is required.
type Deps struct {
	Tenants     Tenants
	Members     Members
	Invitations Invitations
	Users       UserDirectory
	Workflows   WorkflowPresets
	Mailer      InvitationMailer
	Tx          appshared.TxManager
	Clock       appshared.Clock
	IDs         appshared.IDGenerator
	Secrets     appshared.SecretGenerator
	Events      appshared.EventPublisher
	// Slugs refuses the reserved labels, the configured hosts' included.
	Slugs tenancy.SlugPolicy
}

// Service implements UseCases: creating a store, its settings, its team and
// invitations.
type Service struct {
	tenants     Tenants
	members     Members
	invitations Invitations
	users       UserDirectory
	workflows   WorkflowPresets
	mailer      InvitationMailer
	tx          appshared.TxManager
	clock       appshared.Clock
	ids         appshared.IDGenerator
	secrets     appshared.SecretGenerator
	events      appshared.EventPublisher
	slugs       tenancy.SlugPolicy
}

// NewService returns a Service over d.
func NewService(d Deps) *Service {
	return &Service{
		tenants: d.Tenants, members: d.Members, invitations: d.Invitations,
		users: d.Users, workflows: d.Workflows, mailer: d.Mailer,
		tx: d.Tx, clock: d.Clock, ids: d.IDs, secrets: d.Secrets, events: d.Events,
		slugs: d.Slugs,
	}
}

// CreateTenant creates a store with the caller as its owner, the default
// settings and the default status workflow (ADR 0006). It works with or
// without a store selected, and leaves the session as it is: the app
// switches to the new store next (ADR 0010). It returns a ValidationError
// for the name or slug and tenancy.ErrSlugTaken when another store has the
// slug.
func (s *Service) CreateTenant(ctx context.Context, in CreateTenantInput) (TenantView, error) {
	by, err := authz.RequireUser(ctx)
	if err != nil {
		return TenantView{}, err
	}
	name, nameErr := tenancy.ParseStoreName(in.Name)
	slug, slugErr := s.slugs.Parse(in.Slug)
	if err := shared.MergeValidation(nameErr, slugErr); err != nil {
		return TenantView{}, err
	}
	t, err := tenancy.NewTenant(shared.TenantID(s.ids.NewID()), name, slug, s.clock.Now())
	if err != nil {
		return TenantView{}, err
	}
	// The new store's own scope, which tenant creation alone may set (ADR
	// 0009); the user is there for the audit trail.
	ctx = appshared.WithScope(ctx, appshared.Scope{TenantID: t.ID(), UserID: by.UserID()})
	if err := s.tx.WithinTx(ctx, func(ctx context.Context) error { return s.createTenant(ctx, t, by.UserID()) }); err != nil {
		return TenantView{}, err
	}
	return tenantView(t), nil
}

func (s *Service) createTenant(ctx context.Context, t *tenancy.Tenant, owner shared.UserID) error {
	workflow, err := s.workflows.DefaultWorkflow(ctx)
	if err != nil {
		return fmt.Errorf("read default workflow: %w", err)
	}
	if err := s.tenants.InsertTenant(ctx, t); err != nil {
		if errors.Is(err, shared.ErrConflict) {
			return tenancy.ErrSlugTaken
		}
		return fmt.Errorf("insert tenant: %w", err)
	}
	settings, err := tenancy.DefaultSettings(t.ID(), workflow)
	if err != nil {
		return err
	}
	if err := s.tenants.InsertSettings(ctx, settings); err != nil {
		return fmt.Errorf("insert settings: %w", err)
	}
	m, err := tenancy.NewOwnerMembership(t.ID(), owner, t.CreatedAt())
	if err != nil {
		return err
	}
	if err := s.members.InsertMembership(ctx, m); err != nil {
		return fmt.Errorf("insert owner: %w", err)
	}
	return s.events.Publish(ctx, tenancy.TenantCreated{Owner: owner, Slug: t.Slug().String()})
}

// Settings returns the selected store's settings. Every role may read them
// (settings:read), because the apps hide Produção and Estoque when they are
// off.
func (s *Service) Settings(ctx context.Context) (SettingsView, error) {
	by, err := authz.Require(ctx, identity.PermSettingsRead)
	if err != nil {
		return SettingsView{}, err
	}
	var st *tenancy.Settings
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		st, err = s.tenants.SettingsOf(ctx, by.TenantID())
		if err != nil {
			return fmt.Errorf("read settings: %w", err)
		}
		return nil
	})
	if err != nil {
		return SettingsView{}, err
	}
	return settingsView(st, st.Version()), nil
}

// UpdateSettings replaces the editable settings (owner only:
// settings:write). It returns a ValidationError for a value out of range
// and shared.ErrVersionConflict when in.Version is not the stored version.
func (s *Service) UpdateSettings(ctx context.Context, in UpdateSettingsInput) (SettingsView, error) {
	by, err := authz.Require(ctx, identity.PermSettingsWrite)
	if err != nil {
		return SettingsView{}, err
	}
	if err := in.Settings.Validate(); err != nil {
		return SettingsView{}, err
	}
	var st *tenancy.Settings
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		st, err = s.tenants.SettingsOf(ctx, by.TenantID())
		if err != nil {
			return fmt.Errorf("read settings: %w", err)
		}
		if err := st.Change(in.Settings, in.Version); err != nil {
			return err
		}
		if err := s.tenants.UpdateSettings(ctx, st); err != nil {
			return fmt.Errorf("save settings: %w", err)
		}
		return s.events.Publish(ctx, tenancy.SettingsUpdated{Version: st.Version() + 1, By: by.UserID()})
	})
	if err != nil {
		return SettingsView{}, err
	}
	return settingsView(st, st.Version()+1), nil // the repository stored the next version
}
