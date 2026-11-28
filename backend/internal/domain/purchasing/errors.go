package purchasing

import (
	"errors"
	"fmt"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Errors of the purchasing context. The HTTP adapter maps each to its
// problem code (docs/plans/m5-code-design.md section 5).
var (
	// ErrEmptyPick means a supplier order was started with no line. It is
	// also a *shared.ValidationError on "lines".
	ErrEmptyPick = fmt.Errorf("purchasing: no item picked: %w", shared.ErrInvalid)
	// ErrItemPickedTwice means one order item is in two lines of a pick. It
	// is also a *shared.ValidationError on "lines[i].orderItemId".
	ErrItemPickedTwice = fmt.Errorf("purchasing: order item picked twice: %w", shared.ErrInvalid)
	// ErrSupplierOrderReceived means a line of the supplier order was
	// received or found defective, so it cannot be undone. It matches
	// shared.ErrConflict.
	ErrSupplierOrderReceived = fmt.Errorf("purchasing: supplier order already received: %w", shared.ErrConflict)
	// ErrUndone means the supplier order was already undone. It matches
	// shared.ErrConflict.
	ErrUndone = fmt.Errorf("purchasing: supplier order undone: %w", shared.ErrConflict)
	// ErrNothingOpen means no line is in the state the move needs. It
	// matches shared.ErrConflict.
	ErrNothingOpen = fmt.Errorf("purchasing: no line to move: %w", shared.ErrConflict)
	// ErrLineNotFound means the supplier order has no line with that id. It
	// matches shared.ErrNotFound.
	ErrLineNotFound = fmt.Errorf("purchasing: line not found: %w", shared.ErrNotFound)
	// ErrLineClosed means the line is no longer open (received, defective
	// or cancelled), so the change is refused. It matches shared.ErrConflict.
	ErrLineClosed = fmt.Errorf("purchasing: line closed: %w", shared.ErrConflict)
	// ErrDateInPast means a promised date is before today in the store's
	// time zone. It is also a *shared.ValidationError on "date" or
	// "lines[i].date".
	ErrDateInPast = fmt.Errorf("purchasing: date in the past: %w", shared.ErrInvalid)
	// ErrEmptyReceipt means a receipt has no entry. It is also a
	// *shared.ValidationError on "entries".
	ErrEmptyReceipt = fmt.Errorf("purchasing: empty receipt: %w", shared.ErrInvalid)
	// ErrLineTwice means a receipt checks one line twice. It is also a
	// *shared.ValidationError on "entries[i].lineId".
	ErrLineTwice = fmt.Errorf("purchasing: line checked twice: %w", shared.ErrInvalid)
	// ErrPhotoLimit means a receipt has more than 4 invoice photos or an
	// entry more than 2. It is also a *shared.ValidationError on
	// "invoicePhotoKeys" or "entries[i].photoKeys".
	ErrPhotoLimit = fmt.Errorf("purchasing: too many photos: %w", shared.ErrInvalid)
	// ErrPhotoKey means a photo key is not the one this store's receiving
	// photos have. It matches shared.ErrInvalid and tells nothing about
	// whether another key exists.
	ErrPhotoKey = fmt.Errorf("purchasing: not a receiving photo key: %w", shared.ErrInvalid)
	// ErrSupplierOrderNotFound means the store has no supplier order with
	// that id. It matches shared.ErrNotFound.
	ErrSupplierOrderNotFound = fmt.Errorf("purchasing: supplier order not found: %w", shared.ErrNotFound)
	// ErrInvalidRecord means a stored purchasing row cannot be restored: a
	// bug or a broken row, never bad input, so it does not match
	// shared.ErrInvalid and never becomes a 400.
	ErrInvalidRecord = errors.New("purchasing: invalid stored record")
	// errMissing is a required argument left out: a bug in the caller, never
	// bad input from a person, so it matches no sentinel and becomes a
	// logged 500.
	errMissing = errors.New("purchasing: a required argument is missing")
)

// fieldRefusal is a sentinel error that is also a ValidationError on one
// field, so callers match the rule with errors.Is and the HTTP adapter
// still answers invalid with the field's pt-BR message.
type fieldRefusal struct {
	rule  error
	field *shared.ValidationError
}

func (e *fieldRefusal) Error() string   { return e.rule.Error() }
func (e *fieldRefusal) Unwrap() []error { return []error{e.rule, e.field} }

func refuse(rule error, field, message string) error {
	return &fieldRefusal{rule: rule, field: shared.NewValidationError(shared.FieldError{Field: field, Message: message})}
}

// fieldError is a ValidationError on one field.
func fieldError(field, message string) error {
	return shared.NewValidationError(shared.FieldError{Field: field, Message: message})
}

// pt-BR field messages, from docs/product/design-document.md (Milestone 5
// screens).
const (
	msgNoLines       = "Escolha ao menos um item."
	msgPickedTwice   = "Este item já está na lista."
	msgQuantity      = "Use uma quantidade de 1 a 99."
	msgItemName      = "Informe o nome do item."
	msgSupplierName  = "Informe o nome do fornecedor."
	msgReceiptPhotos = "Use até 20 fotos por recebimento."
	msgNegative      = "Use um valor de zero ou mais."
	msgAmountTooHigh = "Use um valor de até R$ 1.000.000.000,00."
	msgDateFromToday = "Escolha uma data a partir de hoje."
	msgDateRequired  = "Informe a data."
	msgNoEntries     = "Marque ao menos um item como recebido."
	msgTooManyLines  = "Use até 200 itens por recebimento."
	msgLineTwice     = "Este item já está no recebimento."
	msgCheck         = "Escolha Tudo certo ou Com defeito."
	msgDefectNote    = "Descreva o defeito em até 500 caracteres."
	msgInvoicePhotos = "Use até 4 fotos da nota."
	msgEntryPhotos   = "Use até 2 fotos por item."
	msgPhoto         = "Use uma foto JPEG, PNG ou WebP de até 10 MB."
)

// maxAmountCents is the largest cost or price paid a line takes: the
// catalog's and the contract's bound (R$ 1.000.000.000,00).
const maxAmountCents = 100_000_000_000
