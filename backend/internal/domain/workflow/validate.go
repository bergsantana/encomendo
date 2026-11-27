package workflow

import "unicode/utf8"

const (
	maxLabelRunes = 60
	maxNameRunes  = 80
)

// requiredPhases are the phases every workflow must pass through
// (ADR 0004): the rules about the deposit, readiness and hand-over need them.
var requiredPhases = [...]Phase{PhaseDraft, PhaseAwaitingDeposit, PhaseReady, PhaseHandedOver}

// Validate checks that statuses, in list order, make a usable workflow. It
// returns nil, or an error matching ErrInvalidWorkflow that names the first
// broken rule:
//   - the list is not empty;
//   - keys are unique, are not "cancelled" and match ^[a-z][a-z0-9_]{1,62}$;
//   - labels and client labels have 1 to 60 characters;
//   - phases are known and never "cancelled", which is implicit;
//   - draft, awaiting_deposit, ready and handed_over each appear;
//   - the first status is in phase draft and the last in handed_over;
//   - a phase is never lower than the one before it.
//
// The database seed of the global presets follows the same rules (ADR 0004).
func Validate(statuses []Status) error {
	if len(statuses) == 0 {
		return invalid(errEmpty, "a workflow needs statuses")
	}
	seen := make(map[StatusKey]bool, len(statuses))
	for i, s := range statuses {
		if err := validateStatus(s, i); err != nil {
			return err
		}
		if seen[s.Key] {
			return invalid(errDuplicateKey, "status %d repeats key %q", i+1, s.Key)
		}
		seen[s.Key] = true
	}
	if err := validateShape(statuses); err != nil {
		return err
	}
	return validateOrder(statuses)
}

// validateStatus checks the fields of one status that need no other status.
func validateStatus(s Status, i int) error {
	switch {
	case s.Key == keyCancelled:
		return invalid(errReservedKey, "status %d", i+1)
	case !validKey(string(s.Key)):
		return invalid(errKeyFormat, "status %d has key %q", i+1, s.Key)
	case !validLength(s.Label, maxLabelRunes):
		return invalid(errLabelLength, "status %q", s.Key)
	case !validLength(s.ClientLabel, maxLabelRunes):
		return invalid(errClientLabelLength, "status %q", s.Key)
	case s.Phase == PhaseCancelled:
		return invalid(errCancelledPhase, "status %q", s.Key)
	case s.Phase.Rank() < 0:
		return invalid(errUnknownPhase, "status %q has phase %q", s.Key, s.Phase)
	}
	return nil
}

// validateShape checks the phases the list must hold, and where it starts
// and ends.
func validateShape(statuses []Status) error {
	for _, want := range requiredPhases {
		if !hasPhase(statuses, want) {
			return invalid(errMissingPhase, "no status in phase %s", want)
		}
	}
	if first := statuses[0]; first.Phase != PhaseDraft {
		return invalid(errFirstNotDraft, "status %q is in phase %s", first.Key, first.Phase)
	}
	if last := statuses[len(statuses)-1]; last.Phase != PhaseHandedOver {
		return invalid(errLastNotHandedOver, "status %q is in phase %s", last.Key, last.Phase)
	}
	return nil
}

// validateOrder checks that phases never go back along the list, so a
// forward move never lands in an earlier phase.
func validateOrder(statuses []Status) error {
	for i := 1; i < len(statuses); i++ {
		if statuses[i].Phase.Rank() < statuses[i-1].Phase.Rank() {
			return invalid(errPhaseGoesBack, "status %q (%s) follows %q (%s)",
				statuses[i].Key, statuses[i].Phase, statuses[i-1].Key, statuses[i-1].Phase)
		}
	}
	return nil
}

func hasPhase(statuses []Status, p Phase) bool {
	for _, s := range statuses {
		if s.Phase == p {
			return true
		}
	}
	return false
}

// validKey reports whether k matches ^[a-z][a-z0-9_]{1,62}$, the pattern the
// database checks on status and workflow keys.
func validKey(k string) bool {
	if len(k) < 2 || len(k) > 63 || k[0] < 'a' || k[0] > 'z' {
		return false
	}
	for i := 1; i < len(k); i++ {
		c := k[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' {
			return false
		}
	}
	return true
}

// validLength reports whether s has 1 to limit characters, counted as the
// database counts them.
func validLength(s string, limit int) bool {
	n := utf8.RuneCountInString(s)
	return n >= 1 && n <= limit
}
