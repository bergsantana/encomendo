package workflow

import (
	"errors"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func TestParsePhase(t *testing.T) {
	t.Parallel()
	valid := []Phase{
		PhaseDraft, PhaseAwaitingDeposit, PhaseConfirmed, PhaseSourcing,
		PhaseAssembling, PhaseReady, PhaseHandedOver, PhaseCancelled,
	}
	for _, p := range valid {
		got, err := ParsePhase(string(p))
		if err != nil || got != p {
			t.Errorf("ParsePhase(%q) = %q, %v", p, got, err)
		}
	}
	for _, s := range []string{"", "Draft", "shipped", " draft", "draft "} {
		got, err := ParsePhase(s)
		if !errors.Is(err, shared.ErrInvalid) || got != "" {
			t.Errorf("ParsePhase(%q) = %q, %v; want ErrInvalid", s, got, err)
		}
	}
}

func TestPhase_Rank(t *testing.T) {
	t.Parallel()
	tests := []struct {
		phase Phase
		want  int
	}{
		{PhaseCancelled, 0},
		{PhaseDraft, 1},
		{PhaseAwaitingDeposit, 2},
		{PhaseConfirmed, 3},
		{PhaseSourcing, 4},
		{PhaseAssembling, 5},
		{PhaseReady, 6},
		{PhaseHandedOver, 7},
		{Phase("shipped"), -1},
		{Phase(""), -1},
	}
	for _, tt := range tests {
		if got := tt.phase.Rank(); got != tt.want {
			t.Errorf("Rank(%q) = %d, want %d", tt.phase, got, tt.want)
		}
	}
}
