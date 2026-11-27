package workflow

import (
	"context"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// UseCases is the driving port: what the status workflow handler calls.
type UseCases interface {
	// Presets returns the latest published version of every preset, sorted
	// by key. It needs settings:read, which every role has.
	Presets(ctx context.Context) ([]View, error)
}

// Source is what other contexts (tenancy, order) use through inproc ports.
// It does no authorization, because the data is global and read-only, and it
// may run inside the caller's transaction or start its own.
type Source interface {
	// Get returns the published version, or an error matching
	// shared.ErrNotFound.
	Get(ctx context.Context, id shared.WorkflowID, version int) (workflow.Workflow, error)
	// Default returns the preset new stores start with (ADR 0006).
	Default(ctx context.Context) (workflow.Workflow, error)
}

// Repository reads status_workflows and workflow_statuses. The tables are
// global, with no RLS, and encomendo_app may only read them. Every method
// runs inside TxManager.WithinTx and returns appshared.ErrNoTransaction
// outside it.
type Repository interface {
	// Published returns the latest version of each preset, sorted by key.
	Published(ctx context.Context) ([]workflow.Workflow, error)
	// ByVersion returns one published version, or an error matching
	// shared.ErrNotFound.
	ByVersion(ctx context.Context, id shared.WorkflowID, version int) (workflow.Workflow, error)
	// Default returns the default preset's latest version.
	Default(ctx context.Context) (workflow.Workflow, error)
}
