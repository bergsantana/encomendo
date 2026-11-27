package order

import (
	"errors"
	"fmt"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// Errors of the order context. The HTTP adapter maps each to its problem
// code (docs/plans/m4-code-design.md section 5.3).
var (
	// ErrInvalidTransition means the status is not one the order's workflow
	// allows from the current one. It is workflow.ErrInvalidTransition, so
	// either name matches.
	ErrInvalidTransition = workflow.ErrInvalidTransition
	// ErrGuardFailed means a phase the move enters is not ready. The error
	// is a *GuardError naming the guard.
	ErrGuardFailed = errors.New("order: phase guard failed")
	// ErrNotEditable means the order is in assembling or later (ready, in a
	// workflow without assembling) and its items and terms are fixed.
	ErrNotEditable = errors.New("order: not editable once assembling")
	// ErrApprovalRequired means the order was paid, so a change needs the
	// seller to confirm the customer's approval. It matches shared.ErrInvalid.
	ErrApprovalRequired = fmt.Errorf("order: customer approval required: %w", shared.ErrInvalid)
	// ErrPaymentExceedsDue means a payment is above what is still due. It
	// matches shared.ErrInvalid.
	ErrPaymentExceedsDue = fmt.Errorf("order: payment above the amount due: %w", shared.ErrInvalid)
	// ErrPaymentNotFound means the order has no payment with that id.
	ErrPaymentNotFound = fmt.Errorf("order: payment not found: %w", shared.ErrNotFound)
	// ErrItemNotFound means the order has no item with that id.
	ErrItemNotFound = fmt.Errorf("order: item not found: %w", shared.ErrNotFound)
	// ErrInvalidItemMove means the item cannot go to that state from where
	// it is, or the check given does not fit the move.
	ErrInvalidItemMove = errors.New("order: invalid item move")
	// ErrNoBackupSupplier means the item has no backup supplier in its
	// snapshot, is made in-house, or is no longer to be bought.
	ErrNoBackupSupplier = errors.New("order: no backup supplier to switch to")
	// ErrHandoverRequired means a move into handed_over did not say whether
	// it was a delivery or a pickup. The error is also a
	// *shared.ValidationError on the field "handover".
	ErrHandoverRequired = fmt.Errorf("order: handover required: %w", shared.ErrInvalid)
	// ErrDateInPast means a new delivery date is before today in the store's
	// time zone. The error is also a *shared.ValidationError on "date".
	ErrDateInPast = fmt.Errorf("order: delivery date in the past: %w", shared.ErrInvalid)
	// ErrOrderClosed means the order was handed over or cancelled, so
	// nothing about it changes any more.
	ErrOrderClosed = errors.New("order: handed over or cancelled")
	// ErrAlreadyReleased means the order's release date is already set,
	// by a paid deposit or an earlier release. It matches shared.ErrInvalid.
	ErrAlreadyReleased = fmt.Errorf("order: already released: %w", shared.ErrInvalid)
)

// Guard names a phase guard of Advance. The values are part of the API
// contract (Problem.guard).
type Guard string

// The guards.
const (
	// GuardDepositUnpaid: leaving awaiting_deposit needs the deposit paid or
	// the owner's release.
	GuardDepositUnpaid Guard = "deposit_unpaid"
	// GuardItemsNotReady: entering assembling, or ready without assembling,
	// needs every item arrived and checked.
	GuardItemsNotReady Guard = "items_not_ready"
	// GuardBalanceUnpaid: entering handed_over needs nothing left to pay.
	GuardBalanceUnpaid Guard = "balance_unpaid"
)

// GuardError is the error Advance returns when a guard does not hold.
type GuardError struct {
	Guard Guard
}

// Error names the guard.
func (e *GuardError) Error() string { return "order: guard " + string(e.Guard) + " failed" }

// Unwrap makes errors.Is(err, ErrGuardFailed) true.
func (e *GuardError) Unwrap() error { return ErrGuardFailed }

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

// pt-BR field messages, from docs/product/design-document.md (Milestone 4
// screens).
const (
	msgNoLines        = "Adicione ao menos um item ou kit."
	msgQuantity       = "Use uma quantidade de 1 a 99."
	msgCurrency       = "Use a moeda da loja."
	msgDiscount       = "O desconto não pode passar do valor dos itens."
	msgNegative       = "Use um valor de zero ou mais."
	msgFreightPickup  = "Retirada na loja não tem frete."
	msgAddress        = "Escolha o endereço de entrega."
	msgHandover       = "Escolha entre Entrega e Retirada na loja."
	msgDepositPercent = "Use um sinal de 0 a 100%."
	msgDays           = "Use de 0 a 60 dias."
	msgAmount         = "Informe um valor maior que zero."
	msgPaymentKind    = "Escolha Sinal ou Saldo."
	msgPaymentMethod  = "Escolha a forma de pagamento."
	msgReason         = "Informe o motivo."
	msgReasonTooLong  = "Use um motivo de até 500 caracteres."
	msgDateFromToday  = "Escolha uma data a partir de hoje."
	msgHandedOverAs   = "Escolha como o pedido foi entregue: Entrega ou Retirada na loja."
	msgLineShape      = "Um item avulso fica sozinho na linha."
	msgPaymentVoided  = "Este pagamento já foi anulado."
	msgItemName       = "Informe o nome do item."
	msgLeadDays       = "Use de 0 a 365 dias."
	msgSourcingMode   = "Escolha a origem do item."
	msgAmountTooHigh  = "Use um valor de até R$ 1.000.000.000,00."
)

// maxAmountCents is the largest price, freight or cost an order takes: the
// catalog's bound (R$ 1.000.000.000,00, the contract's Cents maximum). With
// at most 50 lines of 99 items, every sum stays far from int64 overflow.
const maxAmountCents = 100_000_000_000

// errMissingToday is the error a move gets without the store's day: a bug
// in the caller, never bad input from a person.
var errMissingToday = fmt.Errorf("order: today is required: %w", shared.ErrInvalid)
