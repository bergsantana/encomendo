package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler/dto"
	appworkflow "github.com/bergsantana/encomendo/backend/internal/app/workflow"
)

// WorkflowsDeps is what the status workflow handler needs.
type WorkflowsDeps struct {
	UseCases appworkflow.UseCases
	Log      *slog.Logger
}

// Workflows serves GET /api/v1/admin/status-workflows: the presets a store
// can choose. Choosing one is Tenancy.ChooseWorkflow.
type Workflows struct {
	uc  appworkflow.UseCases
	log *slog.Logger
}

// NewWorkflows returns the status workflow handler, or an error when a
// dependency is missing.
func NewWorkflows(d WorkflowsDeps) (*Workflows, error) {
	if d.UseCases == nil || d.Log == nil {
		return nil, errors.New("handler: the status workflow handler needs use cases and a logger")
	}
	return &Workflows{uc: d.UseCases, log: d.Log}, nil
}

// List is GET /api/v1/admin/status-workflows.
func (h *Workflows) List(w http.ResponseWriter, r *http.Request) {
	v, err := h.uc.Presets(r.Context())
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewList(v, dto.NewStatusWorkflow) })
}
