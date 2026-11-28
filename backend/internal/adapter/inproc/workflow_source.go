package inproc

import (
	"context"
	"errors"
	"fmt"

	apptenancy "github.com/bergsantana/encomendo/backend/internal/app/tenancy"
	appworkflow "github.com/bergsantana/encomendo/backend/internal/app/workflow"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

var _ apptenancy.WorkflowPresets = (*TenancyWorkflows)(nil)

// TenancyWorkflows implements apptenancy.WorkflowPresets over the cached
// app/workflow Source. It replaces the Postgres reader tenancy used in
// milestone 2.
type TenancyWorkflows struct{ source appworkflow.Source }

// NewTenancyWorkflows returns tenancy's workflow port.
func NewTenancyWorkflows(s appworkflow.Source) *TenancyWorkflows { return &TenancyWorkflows{source: s} }

// DefaultWorkflow returns the preset new stores start with (ADR 0006).
func (t *TenancyWorkflows) DefaultWorkflow(ctx context.Context) (tenancy.WorkflowRef, error) {
	w, err := t.source.Default(ctx)
	if err != nil {
		return tenancy.WorkflowRef{}, err
	}
	return refOf(w), nil
}

// Exists returns the reference of the published version, or an error
// matching shared.ErrNotFound.
func (t *TenancyWorkflows) Exists(ctx context.Context, id shared.WorkflowID, version int) (tenancy.WorkflowRef, error) {
	w, err := t.source.Get(ctx, id, version)
	if err != nil {
		return tenancy.WorkflowRef{}, err
	}
	return refOf(w), nil
}

// Statuses returns the statuses of the published version in order, or an
// error matching shared.ErrNotFound.
func (t *TenancyWorkflows) Statuses(ctx context.Context, id shared.WorkflowID, version int) ([]apptenancy.WorkflowStatus, error) {
	w, err := t.source.Get(ctx, id, version)
	if err != nil {
		return nil, err
	}
	statuses := w.Statuses()
	out := make([]apptenancy.WorkflowStatus, 0, len(statuses))
	for _, s := range statuses {
		out = append(out, apptenancy.WorkflowStatus{Key: s.Key, Label: s.Label, ClientLabel: s.ClientLabel, Phase: s.Phase})
	}
	return out, nil
}

func refOf(w workflow.Workflow) tenancy.WorkflowRef {
	return tenancy.WorkflowRef{ID: w.ID(), Version: w.Version(), Name: w.Name()}
}

// OrderWorkflows is the workflow port of app/order: the workflow new orders
// take, and any published version an existing order points at. The order
// context's port (Current, Get) is asserted where app/order lands (step
// 4.10).
type OrderWorkflows struct {
	settings TenancyDefaults
	source   appworkflow.Source
}

// NewOrderWorkflows returns the order context's workflow port.
func NewOrderWorkflows(settings TenancyDefaults, s appworkflow.Source) *OrderWorkflows {
	return &OrderWorkflows{settings: settings, source: s}
}

// Current returns the workflow of the store in the scope, at the version its
// settings point at: the one a new order takes (ADR 0004). It needs only a
// tenant in the scope, like StoreDefaults, and returns its errors. A store
// whose settings name an unpublished version is a bug, so the not-found
// error is flattened and answers 500, not 404.
func (o *OrderWorkflows) Current(ctx context.Context) (workflow.Workflow, error) {
	d, err := o.settings.StoreDefaults(ctx)
	if err != nil {
		return workflow.Workflow{}, err
	}
	w, err := o.source.Get(ctx, d.Workflow.ID, d.Workflow.Version)
	if errors.Is(err, shared.ErrNotFound) {
		return workflow.Workflow{}, fmt.Errorf("current workflow %s v%d of the store is not published", d.Workflow.ID, d.Workflow.Version)
	}
	if err != nil {
		return workflow.Workflow{}, fmt.Errorf("current workflow %s v%d of the store: %w", d.Workflow.ID, d.Workflow.Version, err)
	}
	return w, nil
}

// Get returns a published version, for an order made with it, or an error
// matching shared.ErrNotFound.
func (o *OrderWorkflows) Get(ctx context.Context, id shared.WorkflowID, version int) (workflow.Workflow, error) {
	return o.source.Get(ctx, id, version)
}
