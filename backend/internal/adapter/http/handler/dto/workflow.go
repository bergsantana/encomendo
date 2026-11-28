package dto

import (
	apptenancy "github.com/bergsantana/encomendo/backend/internal/app/tenancy"
	appworkflow "github.com/bergsantana/encomendo/backend/internal/app/workflow"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// StatusWorkflow is the StatusWorkflow schema: a preset, or the workflow an
// order was made with, at one version. Statuses are in order and never
// null; `cancelled` is implicit and never listed.
type StatusWorkflow struct {
	ID       shared.WorkflowID `json:"id"`
	Version  int               `json:"version"`
	Key      string            `json:"key"`
	Name     string            `json:"name"`
	Statuses []OrderStatus     `json:"statuses"`
}

// NewStatusWorkflow maps a preset.
func NewStatusWorkflow(v appworkflow.View) StatusWorkflow {
	out := StatusWorkflow{ID: v.ID, Version: v.Version, Key: v.Key, Name: v.Name, Statuses: make([]OrderStatus, 0, len(v.Statuses))}
	for _, s := range v.Statuses {
		out.Statuses = append(out.Statuses, orderStatus(string(s.Key), s.Label, s.ClientLabel, s.Phase))
	}
	return out
}

func orderStatus(key, label, clientLabel string, phase workflow.Phase) OrderStatus {
	return OrderStatus{Key: key, Label: label, ClientLabel: clientLabel, Phase: string(phase)}
}

// ChooseWorkflowRequest is PUT /api/v1/admin/settings/status-workflow.
type ChooseWorkflowRequest struct {
	WorkflowID      *string `json:"workflowId"`
	Version         *int    `json:"version"`
	ExpectedVersion *int    `json:"expectedVersion"`
}

// Input returns the use case's input, or a ValidationError naming every
// missing field.
func (r ChooseWorkflowRequest) Input() (apptenancy.ChooseWorkflowInput, error) {
	var f fields
	in := apptenancy.ChooseWorkflowInput{
		Workflow:        needID[shared.WorkflowID](&f, r.WorkflowID, "workflowId"),
		Version:         need(&f, r.Version, "version"),
		ExpectedVersion: need(&f, r.ExpectedVersion, "expectedVersion"),
	}
	return in, f.err()
}
