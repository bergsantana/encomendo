package shared

import "fmt"

// Page size limits for every list endpoint (docs/architecture.md section 6).
const (
	DefaultPageLimit = 20
	MaxPageLimit     = 100
	// MaxCursorLength bounds the opaque cursor so a client cannot make the
	// server decode arbitrarily large input.
	MaxCursorLength = 512
)

// PageRequest asks for one page of a list: up to Limit items after Cursor.
// The cursor is opaque here; the repository that issued it decodes it.
type PageRequest struct {
	cursor string
	limit  int
}

// NewPageRequest validates a page request. A zero limit means
// DefaultPageLimit. It returns a ValidationError for a limit outside
// 1..MaxPageLimit or a cursor longer than MaxCursorLength.
func NewPageRequest(cursor string, limit int) (PageRequest, error) {
	if limit == 0 {
		limit = DefaultPageLimit
	}
	if limit < 1 || limit > MaxPageLimit {
		return PageRequest{}, NewValidationError(FieldError{
			Field:   "limit",
			Message: fmt.Sprintf("Use um limite entre 1 e %d.", MaxPageLimit),
		})
	}
	if len(cursor) > MaxCursorLength {
		return PageRequest{}, NewValidationError(FieldError{Field: "cursor", Message: "Cursor inválido."})
	}
	return PageRequest{cursor: cursor, limit: limit}, nil
}

// Cursor returns the position to continue from; empty means the first page.
func (p PageRequest) Cursor() string { return p.cursor }

// Limit returns the maximum number of items, between 1 and MaxPageLimit.
// The zero PageRequest asks for DefaultPageLimit, never for zero items.
func (p PageRequest) Limit() int {
	if p.limit == 0 {
		return DefaultPageLimit
	}
	return p.limit
}

// Page is one page of a list. NextCursor is empty on the last page.
type Page[T any] struct {
	Items      []T
	NextCursor string
}

// HasMore reports whether another page follows.
func (p Page[T]) HasMore() bool { return p.NextCursor != "" }
