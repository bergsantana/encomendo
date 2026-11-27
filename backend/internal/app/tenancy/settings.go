package tenancy

import (
	"context"
	"errors"
	"fmt"

	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

// Settings returns the selected store's settings, with the statuses of its
// workflow. Every role may read them (settings:read), because the apps hide
// Produção and Estoque when they are off.
func (s *Service) Settings(ctx context.Context) (SettingsView, error) {
	by, err := authz.Require(ctx, identity.PermSettingsRead)
	if err != nil {
		return SettingsView{}, err
	}
	var out SettingsView
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		st, err := s.tenants.SettingsOf(ctx, by.TenantID())
		if err != nil {
			return mustExist("read settings", err)
		}
		out, err = s.viewOf(ctx, st, st.Version())
		return err
	})
	return out, err
}

// UpdateSettings replaces the editable settings (owner only:
// settings:write). An empty in.Settings.TimeZone keeps the stored zone, so
// an older client that does not send it never moves the store's clock. It
// returns a ValidationError for a value out of range and
// shared.ErrVersionConflict when in.Version is not the stored version.
func (s *Service) UpdateSettings(ctx context.Context, in UpdateSettingsInput) (SettingsView, error) {
	by, err := authz.Require(ctx, identity.PermSettingsWrite)
	if err != nil {
		return SettingsView{}, err
	}
	check := in.Settings
	if check.TimeZone == "" {
		check.TimeZone = tenancy.ZoneSaoPaulo // a placeholder: the stored zone is used below
	}
	if err := check.Validate(); err != nil {
		return SettingsView{}, err
	}
	var out SettingsView
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		st, err := s.tenants.SettingsOf(ctx, by.TenantID())
		if err != nil {
			return mustExist("read settings", err)
		}
		next := in.Settings
		if next.TimeZone == "" {
			next.TimeZone = st.Editable().TimeZone
		}
		if err := st.Change(next, in.Version); err != nil {
			return err
		}
		if err := s.tenants.UpdateSettings(ctx, st); err != nil {
			return fmt.Errorf("save settings: %w", err)
		}
		if err := s.events.Publish(ctx, tenancy.SettingsUpdated{Version: st.Version() + 1, By: by.UserID()}); err != nil {
			return err
		}
		out, err = s.viewOf(ctx, st, st.Version()+1) // the repository stored the next version
		return err
	})
	return out, err
}

// ChooseStatusWorkflow makes the preset at in.Version the workflow new
// orders take (owner only: settings:write). Orders already made keep theirs
// (ADR 0004). It returns tenancy.ErrWorkflowNotFound when no preset has
// that id and version, and shared.ErrVersionConflict when
// in.ExpectedVersion is not the stored settings version.
func (s *Service) ChooseStatusWorkflow(ctx context.Context, in ChooseWorkflowInput) (SettingsView, error) {
	by, err := authz.Require(ctx, identity.PermSettingsWrite)
	if err != nil {
		return SettingsView{}, err
	}
	var out SettingsView
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		ref, err := s.workflows.Exists(ctx, in.Workflow, in.Version)
		if errors.Is(err, shared.ErrNotFound) {
			return tenancy.ErrWorkflowNotFound
		}
		if err != nil {
			return fmt.Errorf("check workflow %s v%d: %w", in.Workflow, in.Version, err)
		}
		st, err := s.tenants.SettingsOf(ctx, by.TenantID())
		if err != nil {
			return mustExist("read settings", err)
		}
		if err := st.ChooseWorkflow(ref, in.ExpectedVersion); err != nil {
			return err
		}
		if err := s.tenants.UpdateSettings(ctx, st); err != nil {
			return fmt.Errorf("save settings: %w", err)
		}
		if err := s.events.Publish(ctx, tenancy.WorkflowChosen{Workflow: ref.ID, Version: ref.Version}); err != nil {
			return err
		}
		out, err = s.viewOf(ctx, st, st.Version()+1)
		return err
	})
	return out, err
}

// viewOf builds the settings view with the statuses of the workflow st
// points at, inside the caller's transaction. A store always points at a
// published version, so a missing one is a bug (500).
func (s *Service) viewOf(ctx context.Context, st *tenancy.Settings, version int) (SettingsView, error) {
	w := st.Workflow()
	statuses, err := s.workflows.Statuses(ctx, w.ID, w.Version)
	if err != nil {
		return SettingsView{}, mustExist("read workflow statuses", err)
	}
	return settingsView(st, statuses, version), nil
}
