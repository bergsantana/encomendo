package order

import (
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// Cancel cancels the order, on behalf of by, with a reason. Payments stay
// as recorded; a refund happens outside Encomendo (ADR 0030). Milestones 5
// and 6 add the purchase lines and stock reservations a cancel releases. It
// returns ErrOrderClosed after handover or a cancel, and a ValidationError
// for a reason that is empty or over 500 characters.
func (o *Order) Cancel(reason string, by shared.Principal, now time.Time) error {
	if o.Closed() {
		return ErrOrderClosed
	}
	reason, err := requiredReason(reason)
	if err != nil {
		return err
	}
	from := o.status
	o.cancelledFrom = from
	o.status, o.cancelledAt, o.cancelReason = workflow.Cancelled, now, reason
	o.addHistory(HistoryEntry{Kind: HistoryCancelled, By: by.UserID(), At: now, From: from, To: o.status, Reason: reason})
	return nil
}

// CancelledAt returns when the order was cancelled, and false when it was
// not.
func (o *Order) CancelledAt() (time.Time, bool) { return o.cancelledAt, !o.cancelledAt.IsZero() }
