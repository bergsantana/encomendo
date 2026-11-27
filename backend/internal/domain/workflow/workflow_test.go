package workflow

import (
	"errors"
	"strings"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func testID(t *testing.T) shared.WorkflowID {
	t.Helper()
	id, err := shared.ParseWorkflowID("01a20ada-2c00-7000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestNew_Rules(t *testing.T) {
	t.Parallel()
	id := testID(t)
	tests := []struct {
		name    string
		id      shared.WorkflowID
		version int
		key     string
		wfName  string
		rule    error
	}{
		{"zero id", shared.WorkflowID{}, 1, "ok_key", "Nome", errWorkflowID},
		{"version zero", id, 0, "ok_key", "Nome", errWorkflowVersion},
		{"version negative", id, -1, "ok_key", "Nome", errWorkflowVersion},
		{"key uppercase", id, 1, "Ok_key", "Nome", errWorkflowKey},
		{"key empty", id, 1, "", "Nome", errWorkflowKey},
		{"name empty", id, 1, "ok_key", "", errWorkflowName},
		{"name too long", id, 1, "ok_key", strings.Repeat("a", 81), errWorkflowName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := New(tt.id, tt.version, tt.key, tt.wfName, valid())
			wantRule(t, err, tt.rule)
		})
	}
	t.Run("invalid statuses", func(t *testing.T) {
		t.Parallel()
		_, err := New(id, 1, "ok_key", "Nome", nil)
		wantRule(t, err, errEmpty)
	})
}

func TestNew_CopiesStatuses(t *testing.T) {
	t.Parallel()
	in := valid()
	w, err := New(testID(t), 2, "ok_key", "Nome", in)
	if err != nil {
		t.Fatal(err)
	}
	in[0].Label = "mudou"
	out := w.Statuses()
	if out[0].Label != "Rascunho" {
		t.Errorf("caller's slice changed the workflow: %q", out[0].Label)
	}
	out[1].Label = "mudou"
	if got := w.Statuses()[1].Label; got != "Aguardando sinal" {
		t.Errorf("Statuses() leaks the workflow's slice: %q", got)
	}
	if w.Version() != 2 {
		t.Errorf("Version = %d", w.Version())
	}
}

func TestWorkflow_Status(t *testing.T) {
	t.Parallel()
	w := seededPresets()[0].build(t)
	got, ok := w.Status("purchasing")
	if !ok || got.Phase != PhaseSourcing || got.ClientLabel != "Pedido confirmado" {
		t.Errorf("Status(purchasing) = %+v, %v", got, ok)
	}
	got, ok = w.Status("cancelled")
	if !ok || got != Cancelled || got.Phase != PhaseCancelled {
		t.Errorf("Status(cancelled) = %+v, %v", got, ok)
	}
	if _, ok := w.Status("quote"); ok {
		t.Error("Status(quote) found a key from another workflow")
	}
	if _, ok := w.Status(""); ok {
		t.Error("Status(\"\") found a status")
	}
}

func TestWorkflow_First(t *testing.T) {
	t.Parallel()
	for _, p := range seededPresets() {
		if got := p.build(t).First(); got != p.statuses[0] {
			t.Errorf("%s: First() = %+v", p.key, got)
		}
	}
}

func TestWorkflow_HasPhase(t *testing.T) {
	t.Parallel()
	w := seededPresets()[1].build(t) // orcamento_primeiro: has confirmed, no assembling
	for phase, want := range map[Phase]bool{
		PhaseDraft: true, PhaseConfirmed: true, PhaseSourcing: true,
		PhaseAssembling: false, PhaseCancelled: false, Phase("shipped"): false,
	} {
		if got := w.HasPhase(phase); got != want {
			t.Errorf("HasPhase(%s) = %v, want %v", phase, got, want)
		}
	}
}

func TestWorkflow_UnknownStatus(t *testing.T) {
	t.Parallel()
	w := seededPresets()[0].build(t)
	for _, pair := range [][2]StatusKey{{"nope", "draft"}, {"draft", "nope"}, {"nope", "cancelled"}} {
		err := w.CanMove(pair[0], pair[1])
		if !errors.Is(err, ErrUnknownStatus) || !errors.Is(err, shared.ErrInvalid) {
			t.Errorf("CanMove(%s, %s) = %v, want ErrUnknownStatus", pair[0], pair[1], err)
		}
		if errors.Is(err, ErrInvalidTransition) {
			t.Errorf("CanMove(%s, %s) = %v, must not be ErrInvalidTransition", pair[0], pair[1], err)
		}
	}
	if got := w.PhasesEntered("nope", "ready"); got != nil {
		t.Errorf("PhasesEntered from unknown = %v", got)
	}
	if got := w.Moves("nope"); got != nil {
		t.Errorf("Moves from unknown = %v", got)
	}
}

func TestErrInvalidTransition_IsConflict(t *testing.T) {
	t.Parallel()
	w := seededPresets()[0].build(t)
	err := w.CanMove("ready", "draft")
	if !errors.Is(err, ErrInvalidTransition) || !errors.Is(err, shared.ErrConflict) {
		t.Errorf("err = %v, want ErrInvalidTransition matching ErrConflict", err)
	}
}

func TestWorkflow_Moves(t *testing.T) {
	t.Parallel()
	w := seededPresets()[0].build(t)
	keys := func(s []Status) []StatusKey {
		var out []StatusKey
		for _, x := range s {
			out = append(out, x.Key)
		}
		return out
	}
	tests := []struct {
		from StatusKey
		want []StatusKey
	}{
		{"draft", []StatusKey{"awaiting_deposit", "purchasing", "items_arriving", "assembling", "ready", "delivered", "cancelled"}},
		{"items_arriving", []StatusKey{"purchasing", "assembling", "ready", "delivered", "cancelled"}},
		{"ready", []StatusKey{"delivered", "cancelled"}},
		{"delivered", nil},
		{"cancelled", nil},
	}
	for _, tt := range tests {
		got := keys(w.Moves(tt.from))
		if len(got) != len(tt.want) {
			t.Errorf("Moves(%s) = %v, want %v", tt.from, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("Moves(%s) = %v, want %v", tt.from, got, tt.want)
				break
			}
		}
	}
	last := w.Moves("draft")
	if last[len(last)-1] != Cancelled {
		t.Errorf("Moves(draft) must end with Cancelled, got %+v", last[len(last)-1])
	}
}
