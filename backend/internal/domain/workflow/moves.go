package workflow

import "fmt"

// CanMove returns nil when an order may move from status from to status to,
// and otherwise:
//   - ErrUnknownStatus when either key is not in the workflow (cancelled is
//     always known);
//   - ErrInvalidTransition when the move breaks the rules.
//
// The rules: forward to any later status, back only to an earlier status of
// the same phase, to cancelled from any phase before handed_over, and
// nothing from handed_over or cancelled. Staying in the same status is not a
// move. Skipping statuses is allowed here; the order checks the guard of
// every phase the move enters (PhasesEntered).
func (w Workflow) CanMove(from, to StatusKey) error {
	f, ok := w.Status(from)
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownStatus, from)
	}
	t, ok := w.Status(to)
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownStatus, to)
	}
	if from == to || f.Phase == PhaseHandedOver || f.Phase == PhaseCancelled {
		return fmt.Errorf("%w: %q to %q", ErrInvalidTransition, from, to)
	}
	if t.Phase == PhaseCancelled || w.index(to) > w.index(from) || t.Phase == f.Phase {
		return nil
	}
	return fmt.Errorf("%w: %q to %q", ErrInvalidTransition, from, to)
}

// PhasesEntered returns the phases a move from from to to enters, in order,
// each once: the phases of the statuses after from, up to and including to,
// without the phase the order is already in. A move that skips statuses
// enters the phases of the statuses it skips, so the order checks their
// guards too. It returns nil for a move back, a move to cancelled, a move
// within one phase and any move CanMove refuses.
func (w Workflow) PhasesEntered(from, to StatusKey) []Phase {
	if w.CanMove(from, to) != nil || to == keyCancelled {
		return nil
	}
	fi, ti := w.index(from), w.index(to)
	if ti < fi {
		return nil
	}
	current := w.statuses[fi].Phase
	var entered []Phase
	for _, s := range w.statuses[fi+1 : ti+1] {
		if s.Phase != current {
			entered = append(entered, s.Phase)
			current = s.Phase
		}
	}
	return entered
}

// Moves returns every status CanMove allows from from, in list order, with
// Cancelled last unless the order cannot be cancelled. It returns nil for an
// unknown key or when nothing is allowed.
func (w Workflow) Moves(from StatusKey) []Status {
	var out []Status
	for _, s := range w.statuses {
		if w.CanMove(from, s.Key) == nil {
			out = append(out, s)
		}
	}
	if w.CanMove(from, keyCancelled) == nil {
		out = append(out, Cancelled)
	}
	return out
}
