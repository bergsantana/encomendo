package order_test

import (
	"slices"
	"testing"
	"time"

	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

// needs returns the kinds of today's "Precisa de você", and the entry of
// the given kind, if any.
func (h *harness) needs(t *testing.T) apporder.TodayView {
	t.Helper()
	v, err := h.svc.Today(h.viewer(t))
	if err != nil {
		t.Fatalf("Today: %v", err)
	}
	return v
}

func kinds(v apporder.TodayView) []apporder.NeedKind {
	var out []apporder.NeedKind
	for _, n := range v.NeedsYou {
		out = append(out, n.Kind)
	}
	return out
}

func TestToday_AwaitingDepositNeedsYouPastThreeDays(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(h.staff(t), t, func(in *apporder.CreateInput) { in.SendToCustomer = true }).Order // 12-01 12:00
	for _, tc := range []struct {
		after time.Duration
		days  int // 0: no need
	}{{0, 0}, {3 * day, 0}, {day, 4}, {day, 5}} {
		h.clock.Advance(tc.after)
		got := h.needs(t)
		if tc.days == 0 {
			if len(got.NeedsYou) != 0 {
				t.Fatalf("%v: needs = %v, want none", h.clock.Now(), kinds(got))
			}
			continue
		}
		n := got.NeedsYou[0]
		if len(got.NeedsYou) != 1 || n.Kind != apporder.NeedAwaitingDeposit || n.Days != tc.days || n.Order.ID != v.ID || n.Item != "" || n.Amount != nil {
			t.Fatalf("%v: needs = %+v, want awaiting_deposit for %d days", h.clock.Now(), got.NeedsYou, tc.days)
		}
	}
}

// Days in a status are calendar days of the store: a move at 02:00 UTC on
// the 1st is still the 30th in São Paulo, so four days have passed on the
// 4th, where counting in UTC would give three.
func TestToday_DaysInAStatusCountInTheStoresZone(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.clock.Set(time.Date(2026, 12, 1, 2, 0, 0, 0, time.UTC))
	h.create(h.staff(t), t, func(in *apporder.CreateInput) { in.SendToCustomer = true })
	h.clock.Set(time.Date(2026, 12, 4, 15, 0, 0, 0, time.UTC))
	got := h.needs(t)
	if len(got.NeedsYou) != 1 || got.NeedsYou[0].Days != 4 {
		t.Errorf("needs = %+v, want awaiting_deposit for 4 days", got.NeedsYou)
	}
}

func TestToday_ADraftNeverWaitsForTheDeposit(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.create(h.staff(t), t, nil)
	h.clock.Advance(30 * day)
	if got := h.needs(t); len(got.NeedsYou) != 0 {
		t.Errorf("needs = %v", kinds(got))
	}
}

func TestToday_ReadyNeedsYouPastTwoDays(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.ready(h.staff(t), t)
	for _, tc := range []struct {
		after time.Duration
		days  int
	}{{0, 0}, {2 * day, 0}, {day, 3}, {2 * day, 5}} {
		h.clock.Advance(tc.after)
		got := h.needs(t)
		if tc.days == 0 {
			if len(got.NeedsYou) != 0 {
				t.Fatalf("%v: needs = %v, want none", h.clock.Now(), kinds(got))
			}
			continue
		}
		n := got.NeedsYou[0]
		if len(got.NeedsYou) != 1 || n.Kind != apporder.NeedReadyNotHandedOver || n.Days != tc.days || n.Order.ID != v.ID {
			t.Fatalf("%v: needs = %+v, want ready_not_handed_over for %d days", h.clock.Now(), got.NeedsYou, tc.days)
		}
	}
}

func TestToday_LateIsThePastDayNotTheExpectedDay(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.released(h.staff(t), t, nil) // item expected 12-16, delivery 12-21
	h.clock.Advance(15 * day)           // 12-16
	if got := h.needs(t); len(got.NeedsYou) != 0 {
		t.Fatalf("on the expected day: needs = %v", kinds(got))
	}
	h.clock.Advance(day) // 12-17
	got := h.needs(t)
	if len(got.NeedsYou) != 1 {
		t.Fatalf("needs = %+v", got.NeedsYou)
	}
	n := got.NeedsYou[0]
	if n.Kind != apporder.NeedLate || n.Days != 1 || n.Item != "Banco de couro" || n.Order.ID != v.ID || len(n.Order.Alerts) != 1 {
		t.Errorf("need = %+v", n)
	}
	if got.Counts.WithAlert != 1 {
		t.Errorf("counts = %+v", got.Counts)
	}
}

func TestToday_AtRiskIsPastTheArrivalLimit(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.released(h.staff(t), t, nil) // item expected 12-16
	// Promise 12-19: the item must arrive by 12-16 (3 assembly days).
	if _, err := h.svc.ChangeDeliveryDate(h.owner(t), apporder.DateInput{Order: v.ID, Date: testkit.Date(t, "2026-12-19"), Version: v.Version}); err != nil {
		t.Fatal(err)
	}
	if got := h.needs(t); len(got.NeedsYou) != 0 {
		t.Fatalf("on the limit: needs = %v", kinds(got))
	}
	v, _ = h.svc.Get(h.owner(t), v.ID)
	// Promise 12-18: limit 12-15, the item is expected a day after it.
	if _, err := h.svc.ChangeDeliveryDate(h.owner(t), apporder.DateInput{Order: v.ID, Date: testkit.Date(t, "2026-12-18"), Version: v.Version}); err != nil {
		t.Fatal(err)
	}
	got := h.needs(t)
	if len(got.NeedsYou) != 1 || got.NeedsYou[0].Kind != apporder.NeedAtRisk || got.NeedsYou[0].Days != 1 || got.NeedsYou[0].Item != "Banco de couro" {
		t.Fatalf("needs = %+v", got.NeedsYou)
	}
	// Past the expected date too: late wins, once.
	h.clock.Advance(16 * day)
	got = h.needs(t)
	if len(got.NeedsYou) != 1 || got.NeedsYou[0].Kind != apporder.NeedLate {
		t.Errorf("late and at risk: needs = %+v, want one late", got.NeedsYou)
	}
}

func TestToday_RefundDueShowsTheAmount(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(h.staff(t), t, func(in *apporder.CreateInput) {
		in.Lines = []apporder.LineRequest{{Item: h.banco, Quantity: 2}}
	}).Order
	v = h.pay(h.staff(t), t, v, order.PaymentDeposit, 100_000)
	v = h.pay(h.staff(t), t, v, order.PaymentBalance, 100_000)
	if _, err := h.svc.Change(h.admin(t), apporder.ChangeInput{
		Order: v.ID, Lines: []apporder.LineRequest{h.bancoLine()}, Terms: apporder.TermsInput{Handover: order.HandoverPickup},
		CustomerApproved: true, Version: v.Version,
	}); err != nil {
		t.Fatal(err)
	}
	got := h.needs(t)
	if len(got.NeedsYou) != 1 {
		t.Fatalf("needs = %+v", got.NeedsYou)
	}
	n := got.NeedsYou[0]
	if n.Kind != apporder.NeedRefundDue || n.Amount == nil || n.Amount.Cents() != 100_000 || n.Days != 0 {
		t.Errorf("need = %+v", n)
	}
}

// The five kinds come most urgent first, whatever the order they were found.
func TestToday_CountsAndNeedsInOrder(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	late := h.released(h.staff(t), t, nil) // 12-01: item expected 12-16
	h.clock.Advance(10 * day)              // 12-11
	atRisk := h.released(h.staff(t), t, nil)
	if _, err := h.svc.ChangeDeliveryDate(h.owner(t), apporder.DateInput{Order: atRisk.ID, Date: testkit.Date(t, "2026-12-20"), Version: atRisk.Version}); err != nil {
		t.Fatal(err)
	}
	waiting := h.create(h.staff(t), t, func(in *apporder.CreateInput) { in.SendToCustomer = true }).Order
	ready := h.ready(h.staff(t), t)
	refund := h.create(h.staff(t), t, func(in *apporder.CreateInput) {
		in.Lines = []apporder.LineRequest{{Item: h.banco, Quantity: 2}}
	}).Order
	refund = h.pay(h.staff(t), t, refund, order.PaymentDeposit, 100_000)
	refund = h.pay(h.staff(t), t, refund, order.PaymentBalance, 100_000)
	if _, err := h.svc.Change(h.admin(t), apporder.ChangeInput{
		Order: refund.ID, Lines: []apporder.LineRequest{h.bancoLine()}, Terms: apporder.TermsInput{Handover: order.HandoverPickup},
		CustomerApproved: true, Version: refund.Version,
	}); err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(6 * day) // 12-17

	got := h.needs(t)
	want := []apporder.NeedKind{apporder.NeedLate, apporder.NeedAtRisk, apporder.NeedAwaitingDeposit, apporder.NeedReadyNotHandedOver, apporder.NeedRefundDue}
	if !slices.Equal(kinds(got), want) {
		t.Fatalf("needs = %v, want %v", kinds(got), want)
	}
	for i, id := range []string{late.ID.String(), atRisk.ID.String(), waiting.ID.String(), ready.ID.String(), refund.ID.String()} {
		if got.NeedsYou[i].Order.ID.String() != id {
			t.Errorf("need %d is order %s, want %s", i, got.NeedsYou[i].Order.Number, id)
		}
	}
	// Active 5; one waiting for the deposit; late and at risk have alerts;
	// delivery within 7 days: the late order (12-21) and the at risk one (12-20).
	if c := got.Counts; c != (apporder.TodayCounts{Active: 5, AwaitingDeposit: 1, WithAlert: 2, DueIn7Days: 2}) {
		t.Errorf("counts = %+v", c)
	}
	if got.Today != testkit.Date(t, "2026-12-17") {
		t.Errorf("today = %s", got.Today)
	}
	var upcoming []string
	for _, s := range got.Upcoming {
		upcoming = append(upcoming, s.Number)
	}
	if !equal(upcoming, []string{"P-0002", "P-0001", "P-0003", "P-0004", "P-0005"}) {
		t.Errorf("upcoming = %v, want by delivery date then number", upcoming)
	}
}

func TestToday_MostLateFirstWithinAKind(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	older := h.released(h.staff(t), t, nil) // expected 12-16
	h.clock.Advance(3 * day)
	newer := h.released(h.staff(t), t, nil) // expected 12-19
	h.clock.Advance(17 * day)               // 12-21
	got := h.needs(t)
	if len(got.NeedsYou) != 2 || got.NeedsYou[0].Order.ID != older.ID || got.NeedsYou[0].Days != 5 || got.NeedsYou[1].Order.ID != newer.ID || got.NeedsYou[1].Days != 2 {
		t.Errorf("needs = %+v", got.NeedsYou)
	}
}

func TestToday_DueInSevenDaysIncludesTodayAndTheSeventhDay(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for _, date := range []string{"2026-12-01", "2026-12-08", "2026-12-09"} {
		v := h.released(h.staff(t), t, nil)
		if _, err := h.svc.ChangeDeliveryDate(h.owner(t), apporder.DateInput{Order: v.ID, Date: testkit.Date(t, date), Version: v.Version}); err != nil {
			t.Fatal(err)
		}
	}
	if got := h.needs(t).Counts.DueIn7Days; got != 2 {
		t.Errorf("dueIn7Days = %d, want today and today + 7 only", got)
	}
	h.clock.Advance(day) // the first promise is now in the past
	if got := h.needs(t).Counts.DueIn7Days; got != 2 {
		t.Errorf("a day later: dueIn7Days = %d, want the +7 and +8 orders", got)
	}
}

func TestToday_UpcomingStopsAtTwenty(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for range 22 {
		h.create(h.staff(t), t, nil)
		h.clock.Advance(day)
	}
	got := h.needs(t)
	if got.Counts.Active != 22 || len(got.Upcoming) != 20 || got.Upcoming[0].Number != "P-0001" || got.Upcoming[19].Number != "P-0020" {
		t.Errorf("active %d, upcoming %d (%s..)", got.Counts.Active, len(got.Upcoming), got.Upcoming[0].Number)
	}
}

func TestToday_ClosedOrdersLeave(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(h.staff(t), t, nil).Order
	if _, err := h.svc.Cancel(h.admin(t), apporder.CancelInput{Order: v.ID, Reason: "x", Version: v.Version}); err != nil {
		t.Fatal(err)
	}
	if got := h.needs(t); got.Counts.Active != 0 || len(got.Upcoming) != 0 {
		t.Errorf("today = %+v", got)
	}
}

// A store's day starts at its own midnight: at 03:30 UTC it is already the
// 2nd in São Paulo (UTC-3) and still the 1st in Manaus (UTC-4).
func TestToday_IsTheStoresDay(t *testing.T) {
	t.Parallel()
	instant := time.Date(2026, 12, 17, 3, 30, 0, 0, time.UTC)
	for zone, want := range map[string]string{"America/Sao_Paulo": "2026-12-17", "America/Manaus": "2026-12-16"} {
		t.Run(zone, func(t *testing.T) {
			t.Parallel()
			h := newHarnessIn(t, zone)
			h.clock.Set(time.Date(2026, 12, 1, 15, 0, 0, 0, time.UTC))
			h.released(h.staff(t), t, nil) // item expected 12-16
			h.clock.Set(instant)
			got := h.needs(t)
			if got.Today != testkit.Date(t, want) {
				t.Errorf("today = %s, want %s", got.Today, want)
			}
			// On the 16th the item is on its expected day, not late.
			late := len(got.NeedsYou) == 1 && got.NeedsYou[0].Kind == apporder.NeedLate
			if late != (zone == "America/Sao_Paulo") {
				t.Errorf("late = %v with needs %v", late, kinds(got))
			}
		})
	}
}
