package postgres

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres/models"
	appworkflow "github.com/bergsantana/encomendo/backend/internal/app/workflow"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// Workflows implements appworkflow.Repository on status_workflows and
// workflow_statuses. The tables are global: no tenant filter and no RLS, and
// encomendo_app may only read them.
type Workflows struct{}

var _ appworkflow.Repository = (*Workflows)(nil)

// NewWorkflows returns the status workflow repository.
func NewWorkflows() *Workflows { return &Workflows{} }

// latestVersion keeps the highest version of each preset.
const latestVersion = "version = (SELECT max(l.version) FROM status_workflows l WHERE l.id = status_workflows.id)"

// Published returns the latest version of each preset, sorted by key.
func (*Workflows) Published(ctx context.Context) ([]workflow.Workflow, error) {
	db, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	var rows []models.StatusWorkflow
	if err := db.Where(latestVersion).Order("key").Find(&rows).Error; err != nil {
		return nil, wrap("list published workflows", err)
	}
	return buildWorkflows(db, rows)
}

// ByVersion returns one published version.
func (*Workflows) ByVersion(ctx context.Context, id shared.WorkflowID, version int) (workflow.Workflow, error) {
	db, err := conn(ctx)
	if err != nil {
		return workflow.Workflow{}, err
	}
	var row models.StatusWorkflow
	if err := db.Where("id = ? AND version = ?", id.String(), version).Take(&row).Error; err != nil {
		return workflow.Workflow{}, wrap("read workflow", err)
	}
	return buildOne(db, row)
}

// Default returns the latest version of the default preset (ADR 0006).
func (*Workflows) Default(ctx context.Context) (workflow.Workflow, error) {
	db, err := conn(ctx)
	if err != nil {
		return workflow.Workflow{}, err
	}
	var row models.StatusWorkflow
	if err := db.Where("is_default").Take(&row).Error; err != nil {
		return workflow.Workflow{}, wrap("read default workflow", err)
	}
	return buildOne(db, row)
}

func buildOne(db *gorm.DB, row models.StatusWorkflow) (workflow.Workflow, error) {
	all, err := buildWorkflows(db, []models.StatusWorkflow{row})
	if err != nil {
		return workflow.Workflow{}, err
	}
	return all[0], nil
}

// buildWorkflows loads the statuses of rows with one query and rebuilds each
// workflow through workflow.New. A stored workflow that does not validate
// is a corrupt row, a 500: the error is flattened to text so it does not
// match shared.ErrInvalid and answer 400.
func buildWorkflows(db *gorm.DB, rows []models.StatusWorkflow) ([]workflow.Workflow, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	var statuses []models.WorkflowStatus
	err := db.Where("workflow_id IN ?", ids).Order("workflow_id, workflow_version, position").Find(&statuses).Error
	if err != nil {
		return nil, wrap("list workflow statuses", err)
	}
	type version struct {
		id string
		v  int
	}
	byVersion := map[version][]models.WorkflowStatus{}
	for _, s := range statuses {
		k := version{s.WorkflowID, s.WorkflowVersion}
		byVersion[k] = append(byVersion[k], s)
	}
	out := make([]workflow.Workflow, 0, len(rows))
	for _, r := range rows {
		w, err := models.WorkflowToDomain(r, byVersion[version{r.ID, r.Version}])
		if err != nil {
			return nil, fmt.Errorf("stored workflow %s v%d is invalid: %s", r.Key, r.Version, err.Error())
		}
		out = append(out, w)
	}
	return out, nil
}
