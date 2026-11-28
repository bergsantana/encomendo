//go:build integration

package postgres_test

import (
	"context"
	"slices"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
	"github.com/bergsantana/encomendo/backend/test/contract/workflowports"
)

// The same suite runs against the fake in test/contract/workflowports.
func TestWorkflowRepository_Contract(t *testing.T) {
	tx := postgres.NewTxManager(openApp(t))
	workflowports.RunRepositoryContract(t, func(t *testing.T) workflowports.Harness {
		return workflowports.Harness{
			Repo: postgres.NewWorkflows(), Tx: tx,
			Published: testkit.Presets(t), DefaultKey: testkit.PresetEncomendaMontagem,
		}
	})
}

// TestWorkflowSeed_LoadsThroughValidateAndMatchesTheTestkit loads every
// preset 0003_workflows seeds through workflow.New, so workflow.Validate
// judges it (ADR 0006), and compares it with testkit.PresetSpecs, the copy
// the domain and use case tests build on: a seed that breaks a rule, or
// drifts from the copy, fails here.
func TestWorkflowSeed_LoadsThroughValidateAndMatchesTheTestkit(t *testing.T) {
	t.Parallel()
	tx := postgres.NewTxManager(openApp(t))
	repo := postgres.NewWorkflows()
	var got []workflow.Workflow
	err := tx.WithinTx(context.Background(), func(ctx context.Context) error {
		var err error
		got, err = repo.Published(ctx)
		return err
	})
	if err != nil {
		t.Fatalf("a seeded preset does not load through workflow.New: %v", err)
	}

	specs := testkit.PresetSpecs()
	if len(got) != len(specs) {
		t.Fatalf("%d seeded presets, want %d", len(got), len(specs))
	}
	for _, spec := range specs {
		i := slices.IndexFunc(got, func(w workflow.Workflow) bool { return w.Key() == spec.Key })
		if i < 0 {
			t.Errorf("preset %s is not seeded", spec.Key)
			continue
		}
		w := got[i]
		if w.ID().String() != spec.ID || w.Version() != 1 || w.Name() != spec.Name {
			t.Errorf("%s = id %s v%d %q, want id %s v1 %q", spec.Key, w.ID(), w.Version(), w.Name(), spec.ID, spec.Name)
		}
		if !slices.Equal(w.Statuses(), spec.Statuses) {
			t.Errorf("%s statuses differ from testkit.PresetSpecs:\n seed %+v\n kit  %+v", spec.Key, w.Statuses(), spec.Statuses)
		}
	}
}
