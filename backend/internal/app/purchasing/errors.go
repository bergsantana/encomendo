package purchasing

import (
	"fmt"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Errors of the purchasing use cases. The domain's own are in
// internal/domain/purchasing.
var (
	// ErrItemNotToBuy means a picked item is not in "A comprar" for the
	// supplier: it was sent by someone else first, its order was cancelled,
	// or it belongs to another supplier. It matches shared.ErrConflict; the
	// API answers 409 item_not_to_buy.
	ErrItemNotToBuy = fmt.Errorf("purchasing: item not to buy: %w", shared.ErrConflict)
	// ErrIdempotencyKeyRequired means a creating call came without an
	// Idempotency-Key. It matches shared.ErrInvalid; the API answers
	// idempotency_key_required.
	ErrIdempotencyKeyRequired = fmt.Errorf("purchasing: idempotency key required: %w", shared.ErrInvalid)
	// ErrReceiptNotFound means the store has no receipt with that id. It
	// matches shared.ErrNotFound.
	ErrReceiptNotFound = fmt.Errorf("purchasing: receipt not found: %w", shared.ErrNotFound)
)

// refusal is a rule's sentinel that is also a ValidationError on one field,
// so callers match the rule with errors.Is and the API still answers 400
// invalid with the field's pt-BR message (as the domain's refuse does).
type refusal struct {
	rule  error
	field *shared.ValidationError
}

func (e *refusal) Error() string   { return e.rule.Error() }
func (e *refusal) Unwrap() []error { return []error{e.rule, e.field} }

func refuse(rule error, field, message string) error {
	return &refusal{rule: rule, field: shared.NewValidationError(shared.FieldError{Field: field, Message: message})}
}

func invalid(field, message string) error {
	return shared.NewValidationError(shared.FieldError{Field: field, Message: message})
}

// The wording of docs/product/design-document.md, "Milestone 5 screens".
const (
	msgNoItems      = "Escolha ao menos um item."
	msgPickedTwice  = "Este item já está na lista."
	msgLineRepeated = "Esta linha já está na lista."
	msgEntryTarget  = "Informe a linha ou o item a receber."
	msgPhotoType    = "Use uma foto JPEG, PNG ou WebP."
)
