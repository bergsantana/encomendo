package shared

import "github.com/bergsantana/encomendo/backend/internal/domain/shared"

// IDGenerator issues primary keys. Ids are generated in Go, never by the
// database (docs/architecture.md section 7), so a use case knows an
// aggregate's id before it is stored. Convert the result to the typed id the
// aggregate needs: shared.TenantID(s.ids.NewID()).
type IDGenerator interface {
	// NewID returns a UUIDv7 that no earlier call returned and that sorts
	// after or equal to every id this generator issued before.
	NewID() shared.ID
}
