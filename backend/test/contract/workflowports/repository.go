package workflowports

import (
	"context"
	"errors"
	"slices"
	"testing"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	appworkflow "github.com/bergsantana/encomendo/backend/internal/app/workflow"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// Harness is a repository holding exactly the published workflows in
// Published, at the versions listed, with the one whose key is DefaultKey as
// the default preset.
type Harness struct {
	Repo appworkflow.Repository
	Tx   appshared.TxManager
	// Published is the latest version of every preset the repository holds,
	// in any order.
	Published []workflow.Workflow
	// DefaultKey is the key of the default preset.
	DefaultKey string
}

// in runs fn inside a transaction in the zero scope: the tables are global,
// so no tenant is needed.
func (h Harness) in(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	if err := h.Tx.WithinTx(context.Background(), fn); err != nil {
		t.Fatal(err)
	}
}

// same reports whether a and b are the same published version: id, version,
// key, name and every status in order.
func same(a, b workflow.Workflow) bool {
	return a.ID() == b.ID() && a.Version() == b.Version() && a.Key() == b.Key() && a.Name() == b.Name() &&
		slices.Equal(a.Statuses(), b.Statuses())
}

// RunRepositoryContract checks that any appworkflow.Repository behaves like
// the Postgres one: Published lists the latest version of each preset sorted
// by key, ByVersion finds one published version or returns ErrNotFound,
// Default is the default preset, and every method needs a transaction.
func RunRepositoryContract(t *testing.T, newHarness func(t *testing.T) Harness) {
	t.Helper()
	cases := map[string]func(*testing.T, Harness){
		"Published lists the latest version of each preset sorted by key": published,
		"ByVersion finds a published version with its statuses in order":  byVersion,
		"ByVersion of an unknown version or id is ErrNotFound":            byVersionMissing,
		"Default is the default preset":                                   defaultPreset,
		"every method needs a transaction":                                needsTransaction,
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) { run(t, newHarness(t)) })
	}
}

func published(t *testing.T, h Harness) {
	h.in(t, func(ctx context.Context) error {
		got, err := h.Repo.Published(ctx)
		if err != nil {
			return err
		}
		if len(got) != len(h.Published) {
			t.Fatalf("Published returned %d workflows, want %d", len(got), len(h.Published))
		}
		if !slices.IsSortedFunc(got, func(a, b workflow.Workflow) int { return compareKeys(a.Key(), b.Key()) }) {
			t.Errorf("Published is not sorted by key")
		}
		for _, want := range h.Published {
			i := slices.IndexFunc(got, func(w workflow.Workflow) bool { return w.Key() == want.Key() })
			if i < 0 || !same(got[i], want) {
				t.Errorf("Published has no %s equal to the expected one", want.Key())
			}
		}
		return nil
	})
}

func compareKeys(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func byVersion(t *testing.T, h Harness) {
	h.in(t, func(ctx context.Context) error {
		for _, want := range h.Published {
			got, err := h.Repo.ByVersion(ctx, want.ID(), want.Version())
			if err != nil || !same(got, want) {
				t.Errorf("ByVersion(%s, %d) = %v, %v; want the published %s", want.ID(), want.Version(), got.Key(), err, want.Key())
			}
		}
		return nil
	})
}

func byVersionMissing(t *testing.T, h Harness) {
	known := h.Published[0]
	h.in(t, func(ctx context.Context) error {
		if _, err := h.Repo.ByVersion(ctx, known.ID(), known.Version()+100); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("an unpublished version: %v, want ErrNotFound", err)
		}
		if _, err := h.Repo.ByVersion(ctx, shared.WorkflowID{}, 1); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the zero id: %v, want ErrNotFound", err)
		}
		return nil
	})
}

func defaultPreset(t *testing.T, h Harness) {
	h.in(t, func(ctx context.Context) error {
		got, err := h.Repo.Default(ctx)
		if err != nil || got.Key() != h.DefaultKey {
			t.Errorf("Default = %q, %v; want %q", got.Key(), err, h.DefaultKey)
		}
		return nil
	})
}

func needsTransaction(t *testing.T, h Harness) {
	ctx := context.Background()
	if _, err := h.Repo.Published(ctx); !errors.Is(err, appshared.ErrNoTransaction) {
		t.Errorf("Published outside a transaction: %v, want ErrNoTransaction", err)
	}
	if _, err := h.Repo.ByVersion(ctx, h.Published[0].ID(), 1); !errors.Is(err, appshared.ErrNoTransaction) {
		t.Errorf("ByVersion outside a transaction: %v, want ErrNoTransaction", err)
	}
	if _, err := h.Repo.Default(ctx); !errors.Is(err, appshared.ErrNoTransaction) {
		t.Errorf("Default outside a transaction: %v, want ErrNoTransaction", err)
	}
}
