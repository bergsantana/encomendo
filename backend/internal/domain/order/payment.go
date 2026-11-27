package order

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// PaymentKind says what a payment pays: Sinal or Saldo. The values are part
// of the API contract.
type PaymentKind string

// The payment kinds.
const (
	PaymentDeposit PaymentKind = "deposit"
	PaymentBalance PaymentKind = "balance"
)

// Valid reports whether k is a known kind.
func (k PaymentKind) Valid() bool { return k == PaymentDeposit || k == PaymentBalance }

// PaymentMethod is how the store received a payment. Payments are recorded
// by hand, with no payment provider (ADR 0023). The values are part of the
// API contract.
type PaymentMethod string

// The payment methods.
const (
	MethodPix          PaymentMethod = "pix"
	MethodCash         PaymentMethod = "dinheiro"
	MethodCreditCard   PaymentMethod = "cartao_credito" //nolint:gosec // G101 reads "credito" as a credential; it is a payment method name of the API contract
	MethodDebitCard    PaymentMethod = "cartao_debito"
	MethodBankTransfer PaymentMethod = "transferencia"
	MethodOther        PaymentMethod = "outro"
)

// Valid reports whether m is a known method.
func (m PaymentMethod) Valid() bool {
	switch m {
	case MethodPix, MethodCash, MethodCreditCard, MethodDebitCard, MethodBankTransfer, MethodOther:
		return true
	}
	return false
}

// Payment is money the store received for an order. A wrong one is voided,
// never deleted.
type Payment struct {
	id         shared.PaymentID
	kind       PaymentKind
	amount     shared.Money
	method     PaymentMethod
	by         shared.UserID
	at         time.Time
	voidedBy   shared.UserID
	voidedAt   time.Time
	voidReason string
}

// ID returns the payment's id.
func (p Payment) ID() shared.PaymentID { return p.id }

// Kind returns what the payment pays.
func (p Payment) Kind() PaymentKind { return p.kind }

// Amount returns how much was received.
func (p Payment) Amount() shared.Money { return p.amount }

// Method returns how it was received.
func (p Payment) Method() PaymentMethod { return p.method }

// RecordedBy returns who recorded it and when.
func (p Payment) RecordedBy() (shared.UserID, time.Time) { return p.by, p.at }

// Voided reports whether the payment was voided.
func (p Payment) Voided() bool { return !p.voidedAt.IsZero() }

// Void returns who voided the payment, when and why, and false when it was
// not voided.
func (p Payment) Void() (by shared.UserID, at time.Time, reason string, ok bool) {
	return p.voidedBy, p.voidedAt, p.voidReason, p.Voided()
}

// PaymentSpec is a payment to record.
type PaymentSpec struct {
	ID     shared.PaymentID
	Kind   PaymentKind
	Amount shared.Money
	Method PaymentMethod
	By     shared.Principal
	At     time.Time
	Today  shared.CivilDate // At in the store's time zone
}

// RecordPayment records a payment and reports whether it confirmed the
// deposit: the deposit payments that are not void reach the deposit due,
// when they did not before. The deposit due is then fixed at that amount.
// Unless the owner released the order, the release date becomes Today and
// every date counts from it; the promise follows unless it was changed by
// hand (ADR 0030). It returns ErrOrderClosed after
// handover or cancel, a ValidationError for an unknown kind or method or an
// amount that is not above zero, and ErrPaymentExceedsDue for an amount
// above the balance due.
func (o *Order) RecordPayment(s PaymentSpec) (depositConfirmed bool, err error) {
	if o.Closed() {
		return false, ErrOrderClosed
	}
	if err := o.checkPayment(s); err != nil {
		return false, err
	}
	if s.Amount.Cents() > o.BalanceDue().Cents() {
		return false, ErrPaymentExceedsDue
	}
	wasPaid := o.DepositPaid()
	o.payments = append(o.payments, Payment{
		id: s.ID, kind: s.Kind, amount: s.Amount, method: s.Method, by: s.By.UserID(), at: s.At,
	})
	o.addHistory(HistoryEntry{Kind: HistoryPayment, By: s.By.UserID(), At: s.At, Payment: s.ID, Amount: s.Amount, PaymentKind: s.Kind, PaymentMethod: s.Method})
	confirmed := !wasPaid && o.DepositPaid()
	if !confirmed {
		return false, nil
	}
	if o.depositFrozen == nil {
		due := o.DepositDue().Cents()
		o.depositFrozen = &due
	}
	// A deposit paid again after a void releases the order again, from the
	// day it was paid; an owner's release stands.
	if o.releasedBy.IsZero() {
		o.release(s.Today)
	}
	return true, nil
}

func (o *Order) checkPayment(s PaymentSpec) error {
	if s.ID.IsZero() || s.Today.IsZero() {
		return shared.ErrInvalid
	}
	var errs []error
	if !s.Kind.Valid() {
		errs = append(errs, fieldError("kind", msgPaymentKind))
	}
	if !s.Method.Valid() {
		errs = append(errs, fieldError("method", msgPaymentMethod))
	}
	if s.Amount.Cents() <= 0 || s.Amount.Currency() != o.currency() {
		errs = append(errs, fieldError("amountCents", msgAmount))
	}
	return shared.MergeValidation(errs...)
}

// VoidPayment marks a wrong payment void, on behalf of by, with a reason.
// It stays in the history and no longer counts toward the deposit or the
// balance. Voiding a deposit never undoes the release: purchases and dates
// may already depend on it. It returns ErrPaymentNotFound, a
// ValidationError for a payment already void or a reason that is empty or
// over 500 characters. Only the owner may void (order:override), which the
// app layer checks.
func (o *Order) VoidPayment(id shared.PaymentID, reason string, by shared.Principal, now time.Time) error {
	i := o.paymentIndex(id)
	if i < 0 {
		return ErrPaymentNotFound
	}
	if o.payments[i].Voided() {
		return fieldError("paymentId", msgPaymentVoided)
	}
	reason, err := requiredReason(reason)
	if err != nil {
		return err
	}
	p := &o.payments[i]
	p.voidedBy, p.voidedAt, p.voidReason = by.UserID(), now, reason
	o.addHistory(HistoryEntry{Kind: HistoryPaymentVoided, By: by.UserID(), At: now, Payment: id, Amount: p.amount, Reason: reason})
	return nil
}

func (o *Order) paymentIndex(id shared.PaymentID) int {
	for i, p := range o.payments {
		if p.id == id {
			return i
		}
	}
	return -1
}

const maxReasonLength = 500

// requiredReason trims a reason and refuses an empty one or one over 500
// characters.
func requiredReason(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "", fieldError("reason", msgReason)
	}
	return optionalReason(reason)
}

// optionalReason trims a reason and refuses one over 500 characters.
func optionalReason(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if utf8.RuneCountInString(reason) > maxReasonLength {
		return "", fieldError("reason", msgReasonTooLong)
	}
	return reason, nil
}
