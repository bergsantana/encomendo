package workflow

import (
	"errors"
	"fmt"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

var (
	// ErrInvalidWorkflow means a workflow breaks a rule of Validate or New.
	// It matches shared.ErrInvalid (400). The wrapped message says which rule.
	ErrInvalidWorkflow = fmt.Errorf("workflow: invalid workflow: %w", shared.ErrInvalid)
	// ErrUnknownStatus means a status key is not in the workflow. It matches
	// shared.ErrInvalid (400).
	ErrUnknownStatus = fmt.Errorf("workflow: unknown status: %w", shared.ErrInvalid)
	// ErrInvalidTransition means the move is not allowed by the move rules:
	// 409 invalid_transition. It matches shared.ErrConflict, because whether
	// a move is allowed depends on the status the order is in now.
	ErrInvalidTransition = fmt.Errorf("workflow: invalid status transition: %w", shared.ErrConflict)
)

// The rule behind an ErrInvalidWorkflow. They are unexported because callers
// only need to know the workflow is invalid; tests tell the rules apart.
var (
	errEmpty             = errors.New("no statuses")
	errDuplicateKey      = errors.New("duplicate status key")
	errReservedKey       = errors.New("key cancelled is reserved")
	errKeyFormat         = errors.New("status key must be a lowercase identifier of 2 to 63 characters")
	errLabelLength       = errors.New("label must have 1 to 60 characters")
	errClientLabelLength = errors.New("client label must have 1 to 60 characters")
	errCancelledPhase    = errors.New("phase cancelled is implicit and never a status")
	errUnknownPhase      = errors.New("unknown phase")
	errPhaseGoesBack     = errors.New("phase goes back along the list")
	errMissingPhase      = errors.New("required phase missing")
	errFirstNotDraft     = errors.New("first status must be in phase draft")
	errLastNotHandedOver = errors.New("last status must be in phase handed_over")
	errWorkflowID        = errors.New("workflow id is required")
	errWorkflowVersion   = errors.New("version must be 1 or more")
	errWorkflowKey       = errors.New("workflow key must be a lowercase identifier of 2 to 63 characters")
	errWorkflowName      = errors.New("name must have 1 to 80 characters")
)

// invalid returns ErrInvalidWorkflow wrapped with the broken rule and where.
func invalid(rule error, format string, args ...any) error {
	return fmt.Errorf("%w: %w: %s", ErrInvalidWorkflow, rule, fmt.Sprintf(format, args...))
}
