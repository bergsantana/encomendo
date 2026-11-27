package workflow

import (
	"slices"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Workflow is a published status workflow: an ordered list of statuses, each
// in one phase. It is immutable: a published (id, version) never changes, and
// orders keep the version they were created with (ADR 0004).
type Workflow struct {
	id       shared.WorkflowID
	version  int
	key      string
	name     string
	statuses []Status
}

// New builds a workflow from its parts. It returns an error matching
// ErrInvalidWorkflow when the id is zero, the version is below 1, the key is
// not a lowercase identifier, the name is not 1 to 80 characters, or the
// statuses fail Validate. It keeps its own copy of statuses.
func New(id shared.WorkflowID, version int, key, name string, statuses []Status) (Workflow, error) {
	switch {
	case id.IsZero():
		return Workflow{}, invalid(errWorkflowID, "workflow %q", key)
	case version < 1:
		return Workflow{}, invalid(errWorkflowVersion, "workflow %q has version %d", key, version)
	case !validKey(key):
		return Workflow{}, invalid(errWorkflowKey, "workflow key %q", key)
	case !validLength(name, maxNameRunes):
		return Workflow{}, invalid(errWorkflowName, "workflow %q", key)
	}
	if err := Validate(statuses); err != nil {
		return Workflow{}, err
	}
	return Workflow{id: id, version: version, key: key, name: name, statuses: slices.Clone(statuses)}, nil
}

// ID returns the workflow's id, shared by all its versions.
func (w Workflow) ID() shared.WorkflowID { return w.id }

// Version returns the published version, starting at 1.
func (w Workflow) Version() int { return w.version }

// Key returns the preset's key, such as "encomenda_montagem".
func (w Workflow) Key() string { return w.key }

// Name returns the workflow's name for staff, in pt-BR.
func (w Workflow) Name() string { return w.name }

// Statuses returns a copy of the statuses in list order. Cancelled is not
// among them.
func (w Workflow) Statuses() []Status { return slices.Clone(w.statuses) }

// Status returns the status with key k. The key "cancelled" returns
// Cancelled in every workflow. The second result is false for any other key
// the workflow does not hold.
func (w Workflow) Status(k StatusKey) (Status, bool) {
	if k == keyCancelled {
		return Cancelled, true
	}
	if i := w.index(k); i >= 0 {
		return w.statuses[i], true
	}
	return Status{}, false
}

// First returns the status a new order starts in. Validate guarantees it is
// in phase draft.
func (w Workflow) First() Status { return w.statuses[0] }

// HasPhase reports whether any status of the workflow is in phase p. The
// deadline uses it: a workflow without assembling has 0 assembly days.
// PhaseCancelled is never held by a status, so it reports false.
func (w Workflow) HasPhase(p Phase) bool { return hasPhase(w.statuses, p) }

// index returns the position of k in the list, or -1. Cancelled has none.
func (w Workflow) index(k StatusKey) int {
	return slices.IndexFunc(w.statuses, func(s Status) bool { return s.Key == k })
}
