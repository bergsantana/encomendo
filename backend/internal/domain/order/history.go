package order

import (
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// HistoryKind says what a history entry records. The values are part of the
// API contract.
type HistoryKind string

// The history kinds.
const (
	HistoryCreated        HistoryKind = "created"
	HistoryStatus         HistoryKind = "status"
	HistoryItem           HistoryKind = "item"
	HistoryBackupSupplier HistoryKind = "backup_supplier"
	HistoryPayment        HistoryKind = "payment"
	HistoryPaymentVoided  HistoryKind = "payment_voided"
	HistoryReleased       HistoryKind = "released"
	HistoryDate           HistoryKind = "date"
	HistoryChanged        HistoryKind = "changed"
	HistoryTrackingLink   HistoryKind = "tracking_link"
	HistoryCancelled      HistoryKind = "cancelled"
	HistoryPromised       HistoryKind = "promised_date" // a supplier's promised date (ADR 0031)
)

// HistoryEntry is one event of an order's history, for staff. Only the
// fields its kind uses are set. It never holds a tracking token, a
// customer's phone or a supplier (ADR 0030), so the table needs no client
// policy and leaks nothing if a staff screen shows it whole.
type HistoryEntry struct {
	Kind HistoryKind
	By   shared.UserID // zero for a store page order (M7)
	At   time.Time

	From, To workflow.Status // status moves and cancel
	Handover Handover        // a move into handed_over

	Item             shared.OrderItemID
	ItemFrom, ItemTo ItemStatus
	Check            Check

	Payment       shared.PaymentID
	PaymentKind   PaymentKind
	PaymentMethod PaymentMethod
	Amount        shared.Money // a payment, a void, or the new total of a change

	OldDate, NewDate shared.CivilDate
	Reason           string
	CustomerApproved bool

	// Ref is the pedido de compra ("C-0012") of an item move made by
	// purchasing or of a promised date; empty otherwise.
	Ref string
}
