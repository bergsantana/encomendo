package workflow

import (
	"fmt"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Phase is a fixed stage of an order's life (ADR 0004). Every order rule
// hangs on a phase, so stores can name and count their statuses freely.
type Phase string

// The phases, in the order an order passes through them. PhaseCancelled is
// outside that order: any order that has not been handed over can end there.
const (
	PhaseDraft           Phase = "draft"
	PhaseAwaitingDeposit Phase = "awaiting_deposit"
	PhaseConfirmed       Phase = "confirmed"
	PhaseSourcing        Phase = "sourcing"
	PhaseAssembling      Phase = "assembling"
	PhaseReady           Phase = "ready"
	PhaseHandedOver      Phase = "handed_over"
	PhaseCancelled       Phase = "cancelled"
)

// phases lists every phase at the index of its rank, cancelled first.
var phases = [...]Phase{
	PhaseCancelled, PhaseDraft, PhaseAwaitingDeposit, PhaseConfirmed,
	PhaseSourcing, PhaseAssembling, PhaseReady, PhaseHandedOver,
}

// ParsePhase returns the phase named s, cancelled included. It returns an
// error matching shared.ErrInvalid for any other text.
func ParsePhase(s string) (Phase, error) {
	for _, p := range phases {
		if string(p) == s {
			return p, nil
		}
	}
	return "", fmt.Errorf("workflow: unknown phase %q: %w", s, shared.ErrInvalid)
}

// Rank orders the phases along an order's life: draft is 1 and handed_over
// is 7. Cancelled is 0, outside that line. It returns -1 for a value that is
// not a phase.
func (p Phase) Rank() int {
	for i, q := range phases {
		if q == p {
			return i
		}
	}
	return -1
}
