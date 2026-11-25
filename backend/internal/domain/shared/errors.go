package shared

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors shared by every context. Contexts wrap them with their own
// errors (fmt.Errorf("...: %w", shared.ErrNotFound)) so that adapters can map
// any error to a status code with errors.Is, without knowing the context.
var (
	// ErrNotFound means the thing asked for does not exist, or the caller may
	// not know it exists (RLS hides other tenants' rows the same way).
	ErrNotFound = errors.New("not found")
	// ErrConflict means the change clashes with the current state, such as a
	// stale version or a duplicate unique value.
	ErrConflict = errors.New("conflict")
	// ErrForbidden means the caller is known but may not do this.
	ErrForbidden = errors.New("forbidden")
	// ErrInvalid means the input breaks a rule. Prefer *ValidationError when
	// the problem can be pinned to fields.
	ErrInvalid = errors.New("invalid")
	// ErrUnauthenticated means the caller could not be identified: no token,
	// a bad or expired token, or wrong credentials. Adapters answer 401.
	ErrUnauthenticated = errors.New("unauthenticated")
)

// ErrVersionConflict means an optimistic lock failed: the row changed since
// the caller read it. It matches ErrConflict, and the API reports it with its
// own code, version_conflict (docs/architecture.md section 6).
var ErrVersionConflict = fmt.Errorf("version conflict: %w", ErrConflict)

// FieldError describes one invalid input field. Message is pt-BR text that
// clients may show next to the field.
type FieldError struct {
	Field   string
	Message string
}

// ValidationError lists every invalid field of one input. It matches
// ErrInvalid with errors.Is.
type ValidationError struct {
	Fields []FieldError
}

// NewValidationError returns a ValidationError for the given fields.
func NewValidationError(fields ...FieldError) *ValidationError {
	return &ValidationError{Fields: fields}
}

// Error lists the invalid fields, for logs.
func (e *ValidationError) Error() string {
	names := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		names = append(names, f.Field)
	}
	return "invalid input: " + strings.Join(names, ", ")
}

// Unwrap makes errors.Is(err, ErrInvalid) true for every ValidationError.
func (e *ValidationError) Unwrap() error { return ErrInvalid }
