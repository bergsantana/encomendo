package workflowports

import (
	"testing"

	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/app/workflow/fake"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

func TestFakeRepository_Contract(t *testing.T) {
	RunRepositoryContract(t, func(t *testing.T) Harness {
		repo := fake.NewRepository()
		presets := testkit.Presets(t)
		for _, w := range presets {
			repo.Add(w, w.Key() == testkit.PresetEncomendaMontagem)
		}
		return Harness{Repo: repo, Tx: sharedfake.NewTxManager(), Published: presets, DefaultKey: testkit.PresetEncomendaMontagem}
	})
}
