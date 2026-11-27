package order_test

import (
	"errors"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

var (
	day1 = time.Date(2026, 12, 1, 15, 0, 0, 0, time.UTC)
	ok   = order.CheckOK
	bad  = order.CheckDefective
)

func date(t *testing.T, s string) shared.CivilDate { t.Helper(); return testkit.Date(t, s) }
func brl(t *testing.T, c int64) shared.Money       { t.Helper(); return testkit.BRL(t, c) }

func owner(t *testing.T, o *order.Order) shared.Principal {
	t.Helper()
	return testkit.Staff(t, o.TenantID(), shared.RoleOwner)
}

// restoreAt returns o restored at status key, after change edits its
// record: the way tests put an order in any state without walking there.
func restoreAt(t *testing.T, o *order.Order, key workflow.StatusKey, change func(*order.Record)) *order.Order {
	t.Helper()
	r := o.Record()
	r.StatusKey = key
	if change != nil {
		change(&r)
	}
	out, err := order.Restore(r, o.Workflow())
	if err != nil {
		t.Fatalf("restore at %s: %v", key, err)
	}
	return out
}

// payment is a stored payment for a record.
func payment(t *testing.T, kind order.PaymentKind, cents int64) order.PaymentRecord {
	t.Helper()
	return order.PaymentRecord{
		ID: shared.PaymentID(testkit.NextID()), Kind: kind, Amount: brl(t, cents),
		Method: order.MethodPix, By: shared.UserID(testkit.NextID()), At: day1,
	}
}

// setItems sets every item's status (and an ok check where it needs one).
func setItems(r *order.Record, s order.ItemStatus) {
	for li := range r.Lines {
		for ii := range r.Lines[li].Items {
			it := &r.Lines[li].Items[ii]
			it.Status = s
			if s == order.ItemReceived || s == order.ItemReady || s == order.ItemInstalled {
				c := order.CheckOK
				it.Check = &c
			}
		}
	}
}

func guardOf(err error) order.Guard {
	var g *order.GuardError
	if errors.As(err, &g) {
		return g.Guard
	}
	return ""
}

func fieldsOf(err error) map[string]string {
	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		return nil
	}
	out := map[string]string{}
	for _, f := range ve.Fields {
		out[f.Field] = f.Message
	}
	return out
}
