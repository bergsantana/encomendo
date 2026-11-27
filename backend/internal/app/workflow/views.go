package workflow

import (
	"fmt"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// StatusView is one status of a preset. ClientLabel is what the store page
// shows, so it never names suppliers (docs/product/design-document.md,
// System rules). The json tags are for the cache.
type StatusView struct {
	Key         workflow.StatusKey `json:"key"`
	Label       string             `json:"label"`
	ClientLabel string             `json:"clientLabel"`
	Phase       workflow.Phase     `json:"phase"`
}

// View is a published workflow version with its statuses in order. It is the
// cached form too: Source rebuilds the domain value from it.
type View struct {
	ID       shared.WorkflowID `json:"id"`
	Version  int               `json:"version"`
	Key      string            `json:"key"`
	Name     string            `json:"name"`
	Statuses []StatusView      `json:"statuses"`
}

// presetList is the cached value of g:workflow:presets.
type presetList struct {
	Items []View `json:"items"`
}

func viewOf(w workflow.Workflow) View {
	statuses := w.Statuses()
	v := View{ID: w.ID(), Version: w.Version(), Key: w.Key(), Name: w.Name(), Statuses: make([]StatusView, 0, len(statuses))}
	for _, s := range statuses {
		v.Statuses = append(v.Statuses, StatusView{Key: s.Key, Label: s.Label, ClientLabel: s.ClientLabel, Phase: s.Phase})
	}
	return v
}

// workflowOf rebuilds the domain value through workflow.New, so a corrupt
// cache entry is refused like a corrupt row.
func (v View) workflowOf() (workflow.Workflow, error) {
	statuses := make([]workflow.Status, 0, len(v.Statuses))
	for _, s := range v.Statuses {
		statuses = append(statuses, workflow.Status{Key: s.Key, Label: s.Label, ClientLabel: s.ClientLabel, Phase: s.Phase})
	}
	w, err := workflow.New(v.ID, v.Version, v.Key, v.Name, statuses)
	if err != nil {
		return workflow.Workflow{}, fmt.Errorf("rebuild workflow %s v%d: %w", v.ID, v.Version, err)
	}
	return w, nil
}
