package workflow

import (
	"errors"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// preset is one global workflow exactly as migration 0003_workflows seeds it.
// The integration test of the Postgres adapter checks the database still
// matches; the data is copied by hand so a drifting seed fails there.
type preset struct {
	id       string
	key      string
	name     string
	statuses []Status
}

// st is shorthand for one seeded status row.
func st(key, label, clientLabel string, phase Phase) Status {
	return Status{Key: StatusKey(key), Label: label, ClientLabel: clientLabel, Phase: phase}
}

// seededPresets returns the four launch presets, in the seed's order.
func seededPresets() []preset {
	return []preset{
		{
			id: "01a20ada-2c00-7000-8000-000000000001", key: "encomenda_montagem", name: "Encomenda com montagem",
			statuses: []Status{
				st("draft", "Rascunho", "Pedido recebido", PhaseDraft),
				st("awaiting_deposit", "Aguardando sinal", "Aguardando o sinal", PhaseAwaitingDeposit),
				st("purchasing", "Compras", "Pedido confirmado", PhaseSourcing),
				st("items_arriving", "Itens chegando", "Itens a caminho", PhaseSourcing),
				st("assembling", "Em montagem", "Em montagem", PhaseAssembling),
				st("ready", "Pronto", "Pronto", PhaseReady),
				st("delivered", "Entregue", "Entregue", PhaseHandedOver),
			},
		},
		{
			id: "01a20ada-2c00-7000-8000-000000000002", key: "orcamento_primeiro", name: "Orçamento primeiro",
			statuses: []Status{
				st("quote", "Orçamento", "Orçamento", PhaseDraft),
				st("awaiting_deposit", "Aguardando sinal", "Aguardando o sinal", PhaseAwaitingDeposit),
				st("approved", "Aprovado", "Pedido aprovado", PhaseConfirmed),
				st("preparing", "Em preparação", "Em preparação", PhaseSourcing),
				st("ready", "Pronto", "Pronto", PhaseReady),
				st("delivered", "Entregue", "Entregue", PhaseHandedOver),
			},
		},
		{
			id: "01a20ada-2c00-7000-8000-000000000003", key: "producao_propria", name: "Produção própria",
			statuses: []Status{
				st("draft", "Rascunho", "Pedido recebido", PhaseDraft),
				st("awaiting_deposit", "Aguardando sinal", "Aguardando o sinal", PhaseAwaitingDeposit),
				st("queued", "Na fila", "Na fila de produção", PhaseSourcing),
				st("in_production", "Em produção", "Em produção", PhaseSourcing),
				st("finishing", "Acabamento", "Acabamento", PhaseAssembling),
				st("ready", "Pronto", "Pronto", PhaseReady),
				st("delivered", "Entregue", "Entregue", PhaseHandedOver),
			},
		},
		{
			id: "01a20ada-2c00-7000-8000-000000000004", key: "pronta_entrega", name: "Pronta entrega",
			statuses: []Status{
				st("draft", "Rascunho", "Pedido recebido", PhaseDraft),
				st("awaiting_deposit", "Aguardando sinal", "Aguardando o sinal", PhaseAwaitingDeposit),
				st("picking", "Separando", "Separando seu pedido", PhaseSourcing),
				st("ready_for_pickup", "Pronto para retirada", "Pronto para retirada", PhaseReady),
				st("picked_up", "Retirado", "Retirado", PhaseHandedOver),
			},
		},
	}
}

// build turns a preset into a Workflow, failing the test when it is invalid.
func (p preset) build(t *testing.T) Workflow {
	t.Helper()
	id, err := shared.ParseWorkflowID(p.id)
	if err != nil {
		t.Fatalf("preset %s: id: %v", p.key, err)
	}
	w, err := New(id, 1, p.key, p.name, p.statuses)
	if err != nil {
		t.Fatalf("preset %s: New: %v", p.key, err)
	}
	return w
}

func TestPresets_Valid(t *testing.T) {
	t.Parallel()
	presets := seededPresets()
	if len(presets) != 4 {
		t.Fatalf("got %d presets, want the 4 launch presets", len(presets))
	}
	for _, p := range presets {
		t.Run(p.key, func(t *testing.T) {
			t.Parallel()
			if err := Validate(p.statuses); err != nil {
				t.Fatalf("Validate: %v", err)
			}
			w := p.build(t)
			if w.Key() != p.key || w.Name() != p.name || w.Version() != 1 || w.ID().String() != p.id {
				t.Errorf("identity = %s %q v%d %s", w.Key(), w.Name(), w.Version(), w.ID())
			}
			if got := w.First(); got.Phase != PhaseDraft || got.Key != p.statuses[0].Key {
				t.Errorf("First() = %+v, want the draft status", got)
			}
			for _, phase := range []Phase{PhaseDraft, PhaseAwaitingDeposit, PhaseReady, PhaseHandedOver} {
				if !w.HasPhase(phase) {
					t.Errorf("HasPhase(%s) = false", phase)
				}
			}
		})
	}
}

func TestPresets_HasAssembling(t *testing.T) {
	t.Parallel()
	want := map[string]bool{
		"encomenda_montagem": true,
		"orcamento_primeiro": false,
		"producao_propria":   true,
		"pronta_entrega":     false,
	}
	for _, p := range seededPresets() {
		if got := p.build(t).HasPhase(PhaseAssembling); got != want[p.key] {
			t.Errorf("%s: HasPhase(assembling) = %v, want %v", p.key, got, want[p.key])
		}
	}
}

// expectedMove is the written rule, computed from list positions and phases
// without calling CanMove: forward to any later status, back only within the
// same phase, to cancelled before handed_over, and nothing from handed_over
// or cancelled. from and to are positions; -1 stands for cancelled.
func expectedMove(statuses []Status, from, to int) bool {
	switch {
	case from == to || from < 0:
		return false
	case statuses[from].Phase == PhaseHandedOver:
		return false
	case to < 0:
		return true
	case to > from:
		return true
	default:
		return statuses[to].Phase == statuses[from].Phase
	}
}

// keyAt returns the key at position i, with -1 meaning cancelled.
func keyAt(statuses []Status, i int) StatusKey {
	if i < 0 {
		return "cancelled"
	}
	return statuses[i].Key
}

// expectedEntered is the phases a forward move enters: the phases of the
// statuses after from up to to, each once, without from's own phase.
func expectedEntered(statuses []Status, from, to int) []Phase {
	if to < 0 || to <= from || !expectedMove(statuses, from, to) {
		return nil
	}
	seen := map[Phase]bool{statuses[from].Phase: true}
	var out []Phase
	for i := from + 1; i <= to; i++ {
		if p := statuses[i].Phase; !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

func TestPresets_Moves(t *testing.T) {
	t.Parallel()
	for _, p := range seededPresets() {
		t.Run(p.key, func(t *testing.T) {
			t.Parallel()
			w := p.build(t)
			n := len(p.statuses)
			for from := -1; from < n; from++ {
				var wantMoves []StatusKey
				for to := 0; to < n; to++ {
					if expectedMove(p.statuses, from, to) {
						wantMoves = append(wantMoves, keyAt(p.statuses, to))
					}
				}
				if expectedMove(p.statuses, from, -1) {
					wantMoves = append(wantMoves, "cancelled")
				}
				for to := -1; to < n; to++ {
					checkPair(t, w, p.statuses, from, to)
				}
				checkMoves(t, w, keyAt(p.statuses, from), wantMoves)
			}
		})
	}
}

// checkPair checks CanMove and PhasesEntered for one (from, to) pair.
func checkPair(t *testing.T, w Workflow, statuses []Status, from, to int) {
	t.Helper()
	f, to2 := keyAt(statuses, from), keyAt(statuses, to)
	err := w.CanMove(f, to2)
	if want := expectedMove(statuses, from, to); want != (err == nil) {
		t.Errorf("CanMove(%s, %s) = %v, want allowed=%v", f, to2, err, want)
	}
	if err != nil && !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("CanMove(%s, %s) = %v, want ErrInvalidTransition", f, to2, err)
	}
	got, want := w.PhasesEntered(f, to2), expectedEntered(statuses, from, to)
	if !samePhases(got, want) {
		t.Errorf("PhasesEntered(%s, %s) = %v, want %v", f, to2, got, want)
	}
}

func checkMoves(t *testing.T, w Workflow, from StatusKey, want []StatusKey) {
	t.Helper()
	var got []StatusKey
	for _, s := range w.Moves(from) {
		got = append(got, s.Key)
	}
	if len(got) != len(want) {
		t.Errorf("Moves(%s) = %v, want %v", from, got, want)
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("Moves(%s) = %v, want %v", from, got, want)
			return
		}
	}
}

func samePhases(a, b []Phase) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestPresets_MovesByHand pins a few answers written out by hand for the
// default preset, so the table above cannot agree with a wrong rule.
func TestPresets_MovesByHand(t *testing.T) {
	t.Parallel()
	w := seededPresets()[0].build(t)
	tests := []struct {
		name     string
		from, to StatusKey
		ok       bool
		entered  []Phase
	}{
		{"draft to awaiting deposit", "draft", "awaiting_deposit", true, []Phase{PhaseAwaitingDeposit}},
		{"draft skips to purchasing", "draft", "purchasing", true, []Phase{PhaseAwaitingDeposit, PhaseSourcing}},
		{"draft skips to ready", "draft", "ready", true,
			[]Phase{PhaseAwaitingDeposit, PhaseSourcing, PhaseAssembling, PhaseReady}},
		{"purchasing to items arriving stays in sourcing", "purchasing", "items_arriving", true, nil},
		{"items arriving back to purchasing", "items_arriving", "purchasing", true, nil},
		{"items arriving to assembling", "items_arriving", "assembling", true, []Phase{PhaseAssembling}},
		{"assembling back to purchasing crosses a phase", "assembling", "purchasing", false, nil},
		{"ready back to draft", "ready", "draft", false, nil},
		{"purchasing cancelled", "purchasing", "cancelled", true, nil},
		{"ready cancelled", "ready", "cancelled", true, nil},
		{"delivered cancelled", "delivered", "cancelled", false, nil},
		{"delivered back", "delivered", "ready", false, nil},
		{"cancelled never moves", "cancelled", "draft", false, nil},
		{"same status", "draft", "draft", false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := w.CanMove(tt.from, tt.to) == nil; got != tt.ok {
				t.Errorf("CanMove(%s, %s) allowed = %v, want %v", tt.from, tt.to, got, tt.ok)
			}
			if got := w.PhasesEntered(tt.from, tt.to); !samePhases(got, tt.entered) {
				t.Errorf("PhasesEntered(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.entered)
			}
		})
	}
}
