package testkit

import (
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// The seeded preset keys, as migration 0003_workflows names them.
const (
	PresetEncomendaMontagem = "encomenda_montagem"
	PresetOrcamentoPrimeiro = "orcamento_primeiro"
	PresetProducaoPropria   = "producao_propria"
	PresetProntaEntrega     = "pronta_entrega"
)

// PresetSpec is one launch preset exactly as migration 0003_workflows seeds
// it. domain/workflow keeps its own copy in presets_test.go (it cannot
// import this package without a cycle); the Postgres integration test
// checks this one against the database, so a drifting seed fails there.
type PresetSpec struct {
	ID       string
	Key      string
	Name     string
	Statuses []workflow.Status
}

func st(key, label, clientLabel string, phase workflow.Phase) workflow.Status {
	return workflow.Status{Key: workflow.StatusKey(key), Label: label, ClientLabel: clientLabel, Phase: phase}
}

// PresetSpecs returns the four launch presets at version 1, in the seed's
// order.
func PresetSpecs() []PresetSpec {
	return []PresetSpec{
		{
			ID: "01a20ada-2c00-7000-8000-000000000001", Key: PresetEncomendaMontagem, Name: "Encomenda com montagem",
			Statuses: []workflow.Status{
				st("draft", "Rascunho", "Pedido recebido", workflow.PhaseDraft),
				st("awaiting_deposit", "Aguardando sinal", "Aguardando o sinal", workflow.PhaseAwaitingDeposit),
				st("purchasing", "Compras", "Pedido confirmado", workflow.PhaseSourcing),
				st("items_arriving", "Itens chegando", "Itens a caminho", workflow.PhaseSourcing),
				st("assembling", "Em montagem", "Em montagem", workflow.PhaseAssembling),
				st("ready", "Pronto", "Pronto", workflow.PhaseReady),
				st("delivered", "Entregue", "Entregue", workflow.PhaseHandedOver),
			},
		},
		{
			ID: "01a20ada-2c00-7000-8000-000000000002", Key: PresetOrcamentoPrimeiro, Name: "Orçamento primeiro",
			Statuses: []workflow.Status{
				st("quote", "Orçamento", "Orçamento", workflow.PhaseDraft),
				st("awaiting_deposit", "Aguardando sinal", "Aguardando o sinal", workflow.PhaseAwaitingDeposit),
				st("approved", "Aprovado", "Pedido aprovado", workflow.PhaseConfirmed),
				st("preparing", "Em preparação", "Em preparação", workflow.PhaseSourcing),
				st("ready", "Pronto", "Pronto", workflow.PhaseReady),
				st("delivered", "Entregue", "Entregue", workflow.PhaseHandedOver),
			},
		},
		{
			ID: "01a20ada-2c00-7000-8000-000000000003", Key: PresetProducaoPropria, Name: "Produção própria",
			Statuses: []workflow.Status{
				st("draft", "Rascunho", "Pedido recebido", workflow.PhaseDraft),
				st("awaiting_deposit", "Aguardando sinal", "Aguardando o sinal", workflow.PhaseAwaitingDeposit),
				st("queued", "Na fila", "Na fila de produção", workflow.PhaseSourcing),
				st("in_production", "Em produção", "Em produção", workflow.PhaseSourcing),
				st("finishing", "Acabamento", "Acabamento", workflow.PhaseAssembling),
				st("ready", "Pronto", "Pronto", workflow.PhaseReady),
				st("delivered", "Entregue", "Entregue", workflow.PhaseHandedOver),
			},
		},
		{
			ID: "01a20ada-2c00-7000-8000-000000000004", Key: PresetProntaEntrega, Name: "Pronta entrega",
			Statuses: []workflow.Status{
				st("draft", "Rascunho", "Pedido recebido", workflow.PhaseDraft),
				st("awaiting_deposit", "Aguardando sinal", "Aguardando o sinal", workflow.PhaseAwaitingDeposit),
				st("picking", "Separando", "Separando seu pedido", workflow.PhaseSourcing),
				st("ready_for_pickup", "Pronto para retirada", "Pronto para retirada", workflow.PhaseReady),
				st("picked_up", "Retirado", "Retirado", workflow.PhaseHandedOver),
			},
		},
	}
}

// Presets returns the four launch presets as workflows, in the seed's
// order, failing t if one does not build.
func Presets(t testing.TB) []workflow.Workflow {
	t.Helper()
	specs := PresetSpecs()
	out := make([]workflow.Workflow, 0, len(specs))
	for _, s := range specs {
		id, err := shared.ParseWorkflowID(s.ID)
		if err != nil {
			t.Fatalf("preset %s: %v", s.Key, err)
		}
		w, err := workflow.New(id, 1, s.Key, s.Name, s.Statuses)
		if err != nil {
			t.Fatalf("preset %s: %v", s.Key, err)
		}
		out = append(out, w)
	}
	return out
}

// Preset returns the launch preset with key, failing t when there is none.
func Preset(t testing.TB, key string) workflow.Workflow {
	t.Helper()
	for _, w := range Presets(t) {
		if w.Key() == key {
			return w
		}
	}
	t.Fatalf("no preset %q", key)
	return workflow.Workflow{}
}
