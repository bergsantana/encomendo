package catalog

import (
	"fmt"

	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// ErrCostForbidden means a role that does not see costs sent a cost field,
// even a null one (ADR 0025). It matches shared.ErrForbidden (403
// forbidden).
var ErrCostForbidden = fmt.Errorf("catalog: only the owner may send costs: %w", shared.ErrForbidden)

// Reference names a row a refusal is about: a kit that holds an item, or an
// archived item a kit holds. It is the contract's Reference schema.
type Reference struct {
	ID   shared.ID
	Name string
}

// RefusalError is a refusal that names the rows in the way. Err is the
// domain sentinel (catalog.ErrItemInKit or catalog.ErrItemArchived), so
// errors.Is(err, catalog.ErrItemInKit) and errors.Is(err,
// shared.ErrConflict) hold; the API maps Err to its code and References to
// the problem's references.
type RefusalError struct {
	Err        error
	References []Reference
}

// Error names the refusal and how many rows it names, for logs.
func (e *RefusalError) Error() string {
	return fmt.Sprintf("%v (%d reference(s))", e.Err, len(e.References))
}

// Unwrap returns the domain sentinel.
func (e *RefusalError) Unwrap() error { return e.Err }

// archivedRefusal turns the domain's list of archived items into a
// RefusalError that names them, taking the names from facts.
func archivedRefusal(ae *catalog.ArchivedItemsError, facts map[shared.ItemID]ItemFacts) *RefusalError {
	refs := make([]Reference, 0, len(ae.Items))
	for _, id := range ae.Items {
		refs = append(refs, Reference{ID: shared.ID(id), Name: facts[id].Name})
	}
	return &RefusalError{Err: catalog.ErrItemArchived, References: refs}
}
