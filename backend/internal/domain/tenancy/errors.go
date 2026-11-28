package tenancy

import (
	"errors"
	"fmt"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Errors a person can cause. Each matches the sentinel that gives its HTTP
// status; respond/problem.go gives the conflicts their own contract codes.
var (
	// ErrSlugTaken means another store uses the address: 409 slug_taken.
	ErrSlugTaken = fmt.Errorf("tenancy: slug already taken: %w", shared.ErrConflict)
	// ErrAlreadyMember means the person already belongs to the store: 409
	// already_member.
	ErrAlreadyMember = fmt.Errorf("tenancy: already a member of the store: %w", shared.ErrConflict)
	// ErrOwnerFixed means a change would remove the owner or change their
	// role: 403 forbidden (ADR 0011).
	ErrOwnerFixed = fmt.Errorf("tenancy: the owner cannot be removed or change role: %w", shared.ErrForbidden)
	// ErrInvitationNotPending means an invitation or its link is unknown,
	// expired, revoked or used. The cases are never told apart: 404
	// not_found (ADR 0011).
	ErrInvitationNotPending = fmt.Errorf("tenancy: invitation unknown, expired, revoked or used: %w", shared.ErrNotFound)
	// ErrTenantSuspended means the store is suspended: nothing in it may be
	// used or joined until the platform lifts the suspension. 403
	// tenant_suspended (ADR 0022).
	ErrTenantSuspended = fmt.Errorf("tenancy: store suspended: %w", shared.ErrForbidden)
)

// Errors the apps map by sentinel, or that signal a bug.
var (
	// ErrWorkflowNotFound means no published status workflow has the id and
	// version the owner chose: 404 workflow_not_found.
	ErrWorkflowNotFound = fmt.Errorf("tenancy: status workflow not found: %w", shared.ErrNotFound)
	// ErrInvalidTimeZone means a time zone outside Zones. Input is checked
	// by Editable.Validate, so reaching this is a bug and no sentinel
	// matches.
	ErrInvalidTimeZone = errors.New("tenancy: invalid time zone")
)

// Errors for records built from missing or malformed parts. Input reaches
// the constructors already parsed and rows reach the Restore functions from
// the database, so these are bugs: they match no sentinel and the API
// answers 500.
var (
	ErrInvalidTenant     = errors.New("tenancy: invalid tenant")
	ErrInvalidMembership = errors.New("tenancy: invalid membership")
	ErrInvalidInvitation = errors.New("tenancy: invalid invitation")
	ErrInvalidSettings   = errors.New("tenancy: invalid settings")
)
