package fake

import (
	"context"
	"slices"
	"strings"
	"sync"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	appworkflow "github.com/bergsantana/encomendo/backend/internal/app/workflow"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

type versionKey struct {
	id      shared.WorkflowID
	version int
}

// Repository is an in-memory appworkflow.Repository. Like adapter/postgres
// it refuses calls outside a fake transaction. The workflows are global, so
// no scope hides any of them.
type Repository struct {
	mu        sync.Mutex
	versions  map[versionKey]workflow.Workflow
	defaultID shared.WorkflowID
	faults    map[string]error
	calls     map[string]int
}

var _ appworkflow.Repository = (*Repository)(nil)

// NewRepository returns a repository with no workflows.
func NewRepository() *Repository {
	return &Repository{
		versions: map[versionKey]workflow.Workflow{},
		faults:   map[string]error{},
		calls:    map[string]int{},
	}
}

// Add publishes w. Making it the default moves the default flag to its id,
// as publishing a new version of the default preset does in SQL.
func (r *Repository) Add(w workflow.Workflow, isDefault bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.versions[versionKey{w.ID(), w.Version()}] = w
	if isDefault {
		r.defaultID = w.ID()
	}
}

// Fail makes every later call of method ("Published", "ByVersion" or
// "Default") return err; nil heals it.
func (r *Repository) Fail(method string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.faults[method] = err
}

// Calls returns how many times method has been called, failed calls
// included, so a test can tell a cache hit from a read.
func (r *Repository) Calls(method string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls[method]
}

// enter checks the transaction and the fault for method. On success it
// returns with r.mu held.
func (r *Repository) enter(ctx context.Context, method string) error {
	if !sharedfake.InTx(ctx) {
		return appshared.ErrNoTransaction
	}
	r.mu.Lock()
	r.calls[method]++
	if err := r.faults[method]; err != nil {
		r.mu.Unlock()
		return err
	}
	return nil
}

// latest returns the highest version of the workflow id.
func (r *Repository) latest(id shared.WorkflowID) (workflow.Workflow, bool) {
	var best workflow.Workflow
	found := false
	for k, w := range r.versions {
		if k.id == id && (!found || w.Version() > best.Version()) {
			best, found = w, true
		}
	}
	return best, found
}

// Published implements Repository.
func (r *Repository) Published(ctx context.Context) ([]workflow.Workflow, error) {
	if err := r.enter(ctx, "Published"); err != nil {
		return nil, err
	}
	defer r.mu.Unlock()
	seen := map[shared.WorkflowID]bool{}
	var out []workflow.Workflow
	for k := range r.versions {
		if seen[k.id] {
			continue
		}
		seen[k.id] = true
		w, _ := r.latest(k.id)
		out = append(out, w)
	}
	slices.SortFunc(out, func(a, b workflow.Workflow) int { return strings.Compare(a.Key(), b.Key()) })
	return out, nil
}

// ByVersion implements Repository.
func (r *Repository) ByVersion(ctx context.Context, id shared.WorkflowID, version int) (workflow.Workflow, error) {
	if err := r.enter(ctx, "ByVersion"); err != nil {
		return workflow.Workflow{}, err
	}
	defer r.mu.Unlock()
	w, ok := r.versions[versionKey{id, version}]
	if !ok {
		return workflow.Workflow{}, shared.ErrNotFound
	}
	return w, nil
}

// Default implements Repository.
func (r *Repository) Default(ctx context.Context) (workflow.Workflow, error) {
	if err := r.enter(ctx, "Default"); err != nil {
		return workflow.Workflow{}, err
	}
	defer r.mu.Unlock()
	w, ok := r.latest(r.defaultID)
	if !ok {
		return workflow.Workflow{}, shared.ErrNotFound
	}
	return w, nil
}
