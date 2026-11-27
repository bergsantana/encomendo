package models

import (
	"fmt"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// StatusWorkflow maps status_workflows: one published version of a preset.
// The tables are global and read-only for encomendo_app (0003_workflows).
type StatusWorkflow struct {
	ID      string `gorm:"column:id;primaryKey"`
	Version int    `gorm:"column:version;primaryKey"`
	Key     string `gorm:"column:key"`
	Name    string `gorm:"column:name"`
}

// TableName returns status_workflows.
func (StatusWorkflow) TableName() string { return "status_workflows" }

// WorkflowStatus maps workflow_statuses: one status of a version, at its
// position in the list.
type WorkflowStatus struct {
	WorkflowID      string `gorm:"column:workflow_id"`
	WorkflowVersion int    `gorm:"column:workflow_version"`
	Position        int    `gorm:"column:position"`
	Key             string `gorm:"column:key"`
	Label           string `gorm:"column:label"`
	ClientLabel     string `gorm:"column:client_label"`
	Phase           string `gorm:"column:phase"`
}

// TableName returns workflow_statuses.
func (WorkflowStatus) TableName() string { return "workflow_statuses" }

// WorkflowToDomain rebuilds a workflow from its row and its statuses, which
// must be in position order. It goes through workflow.New, so a stored
// workflow that breaks a rule of workflow.Validate is refused instead of
// served.
func WorkflowToDomain(m StatusWorkflow, rows []WorkflowStatus) (workflow.Workflow, error) {
	id, err := parseID[shared.WorkflowID](m.ID)
	if err != nil {
		return workflow.Workflow{}, err
	}
	statuses := make([]workflow.Status, 0, len(rows))
	for _, r := range rows {
		phase, err := workflow.ParsePhase(r.Phase)
		if err != nil {
			return workflow.Workflow{}, fmt.Errorf("workflow %s v%d status %s: %w", m.Key, m.Version, r.Key, err)
		}
		statuses = append(statuses, workflow.Status{
			Key: workflow.StatusKey(r.Key), Label: r.Label, ClientLabel: r.ClientLabel, Phase: phase,
		})
	}
	return workflow.New(id, m.Version, m.Key, m.Name, statuses)
}
