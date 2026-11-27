package order_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

// guardState is one way the order can stand when a move is tried.
type guardState struct {
	name                          string
	depositOK, itemsOK, balanceOK bool
	edit                          func(t *testing.T, r *order.Record)
}

// guardStates covers each guard held and broken. The order is one R$
// 1.000,00 item with a 50% deposit.
var guardStates = []guardState{
	{"all held", true, true, true, func(t *testing.T, r *order.Record) {
		r.Payments = []order.PaymentRecord{payment(t, order.PaymentDeposit, 50_000), payment(t, order.PaymentBalance, 50_000)}
		r.ReleaseDate = date(t, "2026-12-01")
		setItems(r, order.ItemReceived)
	}},
	{"no deposit", false, true, false, func(t *testing.T, r *order.Record) {
		setItems(r, order.ItemReceived)
	}},
	{"released without deposit", true, true, false, func(t *testing.T, r *order.Record) {
		r.ReleaseDate, r.ReleasedBy = date(t, "2026-12-01"), shared.UserID(testkit.NextID())
		setItems(r, order.ItemReceived)
	}},
	{"deposit only", true, true, false, func(t *testing.T, r *order.Record) {
		r.Payments = []order.PaymentRecord{payment(t, order.PaymentDeposit, 50_000)}
		r.ReleaseDate = date(t, "2026-12-01")
		setItems(r, order.ItemReceived)
	}},
	{"items missing", true, false, true, func(t *testing.T, r *order.Record) {
		r.Payments = []order.PaymentRecord{payment(t, order.PaymentDeposit, 50_000), payment(t, order.PaymentBalance, 50_000)}
		r.ReleaseDate = date(t, "2026-12-01")
		setItems(r, order.ItemToBuy)
	}},
}

// expectMove returns the error Advance must give for from → to in a
// workflow with statuses, from the written rule (docs/architecture.md
// section 4.2), not from the code under test: nil, ErrInvalidTransition, or
// the first guard that does not hold.
func expectMove(statuses []workflow.Status, from, to int, s guardState) (order.Guard, error) {
	f, d := statuses[from], statuses[to]
	if from == to || f.Phase == workflow.PhaseHandedOver || (to < from && d.Phase != f.Phase) {
		return "", order.ErrInvalidTransition
	}
	var entered []workflow.Phase
	if to > from {
		for _, st := range statuses[from+1 : to+1] {
			if st.Phase != f.Phase && !slices.Contains(entered, st.Phase) {
				entered = append(entered, st.Phase)
			}
		}
	}
	hasAssembling := slices.ContainsFunc(statuses, func(s workflow.Status) bool { return s.Phase == workflow.PhaseAssembling })
	leaves := f.Phase.Rank() <= workflow.PhaseAwaitingDeposit.Rank() &&
		slices.ContainsFunc(entered, func(p workflow.Phase) bool { return p.Rank() > workflow.PhaseAwaitingDeposit.Rank() })
	assembly := slices.Contains(entered, workflow.PhaseAssembling) || (!hasAssembling && slices.Contains(entered, workflow.PhaseReady))
	switch {
	case leaves && !s.depositOK:
		return order.GuardDepositUnpaid, order.ErrGuardFailed
	case assembly && !s.itemsOK:
		return order.GuardItemsNotReady, order.ErrGuardFailed
	case slices.Contains(entered, workflow.PhaseHandedOver) && !s.balanceOK:
		return order.GuardBalanceUnpaid, order.ErrGuardFailed
	}
	return "", nil
}

func checkEveryMove(t *testing.T, preset string) {
	t.Helper()
	base := testkit.NewOrder().WithPreset(preset).Build(t)
	statuses := base.Workflow().Statuses()
	pickup := order.HandoverPickup
	for fi, from := range statuses {
		for ti, to := range statuses {
			for _, s := range guardStates {
				name := string(from.Key) + "→" + string(to.Key) + "/" + s.name
				o := restoreAt(t, base, from.Key, func(r *order.Record) { s.edit(t, r) })
				err := o.Advance(to.Key, &pickup, owner(t, o), date(t, "2026-12-02"), day1)
				wantGuard, wantErr := expectMove(statuses, fi, ti, s)
				switch {
				case wantErr == nil && err != nil:
					t.Errorf("%s: err = %v, want the move allowed", name, err)
				case wantErr != nil && !errors.Is(err, wantErr):
					t.Errorf("%s: err = %v, want %v", name, err, wantErr)
				case wantGuard != "" && guardOf(err) != wantGuard:
					t.Errorf("%s: guard = %q, want %q", name, guardOf(err), wantGuard)
				case wantErr == nil && o.Status().Key != to.Key:
					t.Errorf("%s: status = %s, want %s", name, o.Status().Key, to.Key)
				case wantErr != nil && o.Status().Key != from.Key:
					t.Errorf("%s: a refused move changed the status to %s", name, o.Status().Key)
				}
			}
		}
		o := restoreAt(t, base, from.Key, nil)
		err := o.Advance(workflow.Cancelled.Key, nil, owner(t, o), date(t, "2026-12-02"), day1)
		if !errors.Is(err, order.ErrInvalidTransition) {
			t.Errorf("%s→cancelled through Advance: err = %v, want ErrInvalidTransition", from.Key, err)
		}
	}
}

func TestAdvance_EncomendaMontagem_EveryMove_WithGuardHeldAndBroken(t *testing.T) {
	t.Parallel()
	checkEveryMove(t, testkit.PresetEncomendaMontagem)
}

func TestAdvance_OrcamentoPrimeiro_EveryMove_WithGuardHeldAndBroken(t *testing.T) {
	t.Parallel()
	checkEveryMove(t, testkit.PresetOrcamentoPrimeiro)
}

func TestAdvance_ProducaoPropria_EveryMove_WithGuardHeldAndBroken(t *testing.T) {
	t.Parallel()
	checkEveryMove(t, testkit.PresetProducaoPropria)
}

func TestAdvance_ProntaEntrega_EveryMove_WithGuardHeldAndBroken(t *testing.T) {
	t.Parallel()
	checkEveryMove(t, testkit.PresetProntaEntrega)
}

func TestAdvance_NothingToSource_PassesSourcingAtOnce(t *testing.T) {
	t.Parallel()

	// Every item already arrived and the deposit is paid: the order goes
	// from Aguardando sinal straight to Em montagem.
	o := testkit.NewOrder().Build(t)
	o = restoreAt(t, o, "awaiting_deposit", func(r *order.Record) {
		r.Payments = []order.PaymentRecord{payment(t, order.PaymentDeposit, 50_000)}
		r.ReleaseDate = date(t, "2026-12-01")
		setItems(r, order.ItemReceived)
	})
	if err := o.Advance("assembling", nil, owner(t, o), date(t, "2026-12-01"), day1); err != nil {
		t.Fatalf("err = %v, want the move allowed", err)
	}
}

func TestAdvance_ReadyWithoutAssembling_ChecksItems(t *testing.T) {
	t.Parallel()

	tests := map[string]workflow.StatusKey{
		testkit.PresetProntaEntrega:     "ready_for_pickup",
		testkit.PresetOrcamentoPrimeiro: "ready",
	}
	for preset, ready := range tests {
		t.Run(preset, func(t *testing.T) {
			t.Parallel()

			o := testkit.NewOrder().WithPreset(preset).Build(t)
			first := o.Workflow().Statuses()[2].Key // the first status past awaiting_deposit
			o = restoreAt(t, o, first, func(r *order.Record) {
				r.Payments = []order.PaymentRecord{payment(t, order.PaymentDeposit, 50_000)}
				r.ReleaseDate = date(t, "2026-12-01")
			})
			err := o.Advance(ready, nil, owner(t, o), date(t, "2026-12-02"), day1)
			if guardOf(err) != order.GuardItemsNotReady {
				t.Errorf("err = %v, want items_not_ready", err)
			}
			if got := o.AssemblyDaysCounted(); got != 0 {
				t.Errorf("assembly days counted = %d, want 0 without an assembling phase", got)
			}
		})
	}
}

func TestAdvance_HandedOverNeedsHowAndRecordsIt(t *testing.T) {
	t.Parallel()

	o := testkit.NewOrder().Build(t)
	o = restoreAt(t, o, "ready", func(r *order.Record) {
		r.Payments = []order.PaymentRecord{payment(t, order.PaymentDeposit, 50_000), payment(t, order.PaymentBalance, 50_000)}
		r.ReleaseDate = date(t, "2026-12-01")
		setItems(r, order.ItemInstalled)
	})
	err := o.Advance("delivered", nil, owner(t, o), date(t, "2026-12-21"), day1)
	if !errors.Is(err, order.ErrHandoverRequired) || !errors.Is(err, shared.ErrInvalid) || fieldsOf(err)["handover"] == "" {
		t.Fatalf("err = %v, want ErrHandoverRequired as a ValidationError on handover", err)
	}
	pickup := order.HandoverPickup
	if err := o.Advance("delivered", &pickup, owner(t, o), date(t, "2026-12-21"), day1); err != nil {
		t.Fatal(err)
	}
	how, at, done := o.HandedOver()
	if !done || how != order.HandoverPickup || !at.Equal(day1) || !o.Closed() {
		t.Errorf("handed over = %v %s at %s, closed %v", done, how, at, o.Closed())
	}
}

func TestAdvance_ZeroDepositReleasesOnLeaving(t *testing.T) {
	t.Parallel()

	o := testkit.NewOrder().WithTerms(func(tr *order.Terms) { tr.DepositPercent = 0 }).Build(t)
	leave := date(t, "2026-12-04")
	if err := o.Advance("purchasing", nil, owner(t, o), leave, day1); err != nil {
		t.Fatal(err)
	}
	if got, ok := o.ReleaseDate(); !ok || got != leave {
		t.Errorf("release = %s %v, want the day it left awaiting_deposit", got, ok)
	}
}

func TestRelease_CountsDatesFromTodayAndOnlyOnce(t *testing.T) {
	t.Parallel()

	o := testkit.NewOrder().WithItem(testkit.ItemOpts{LeadDays: 15}).Build(t)
	by := owner(t, o)
	today := date(t, "2026-12-10")
	if err := o.Release(by, today, day1); err != nil {
		t.Fatal(err)
	}
	if who, ok := o.Released(); !ok || who != by.UserID() {
		t.Errorf("released by %v %v, want the owner", who, ok)
	}
	// 15 lead + 3 assembly + 2 margin from the release day.
	if got := o.Promise(); got != today.AddDays(20) {
		t.Errorf("promise = %s, want release + 20", got)
	}
	if err := o.Release(by, today, day1); !errors.Is(err, order.ErrAlreadyReleased) {
		t.Errorf("second release: err = %v, want ErrAlreadyReleased", err)
	}
	if err := o.Advance("purchasing", nil, by, today, day1); err != nil {
		t.Errorf("leaving awaiting_deposit after a release: err = %v", err)
	}
	if err := o.Release(by, today, day1); !errors.Is(err, order.ErrInvalidTransition) {
		t.Errorf("release past awaiting_deposit: err = %v, want ErrInvalidTransition", err)
	}
}

func TestRelease_RefusedWhenTheDepositIsPaid(t *testing.T) {
	t.Parallel()

	o := testkit.NewOrder().Build(t)
	o = restoreAt(t, o, "awaiting_deposit", func(r *order.Record) {
		r.Payments = []order.PaymentRecord{payment(t, order.PaymentDeposit, 50_000)}
	})
	if err := o.Release(owner(t, o), date(t, "2026-12-02"), day1); !errors.Is(err, order.ErrAlreadyReleased) {
		t.Errorf("err = %v, want ErrAlreadyReleased", err)
	}
}

func TestUseBackupSupplier_RedatesAndSwapsCost(t *testing.T) {
	t.Parallel()

	main, backupCost := int64(40_000), brl(t, 45_000)
	backup := &order.BackupSource{Supplier: shared.SupplierID(testkit.NextID()), Name: "Couros Rio", LeadDays: 6, Cost: &backupCost}
	o := testkit.NewOrder().WithItem(testkit.ItemOpts{Name: "Banco", LeadDays: 15, Cost: &main, Backup: backup}).Build(t)
	promise := o.Promise()
	id := o.Items()[0].ID()
	today := date(t, "2026-12-02")
	if err := o.UseBackupSupplier(id, today, day1, owner(t, o)); err != nil {
		t.Fatal(err)
	}
	it := o.Items()[0]
	src := it.Sourcing()
	if src.SupplierName != "Couros Rio" || src.Cost == nil || src.Cost.Cents() != 45_000 || src.Backup != nil {
		t.Errorf("sourcing = %+v, want the backup with its cost and no backup left", src)
	}
	if it.Snapshot().LeadDays != 6 || it.Expected() != today.AddDays(6) {
		t.Errorf("lead %d expected %s, want 6 days from today", it.Snapshot().LeadDays, it.Expected())
	}
	if o.Promise() != promise {
		t.Errorf("promise moved to %s", o.Promise())
	}
	if err := o.UseBackupSupplier(id, today, day1, owner(t, o)); !errors.Is(err, order.ErrNoBackupSupplier) {
		t.Errorf("second switch: err = %v, want ErrNoBackupSupplier", err)
	}
}

func TestUseBackupSupplier_Refusals(t *testing.T) {
	t.Parallel()

	backup := &order.BackupSource{Supplier: shared.SupplierID(testkit.NextID()), Name: "Outro", LeadDays: 3}
	made := testkit.NewOrder().WithItem(testkit.ItemOpts{Mode: catalog.ModeInHouse, LeadDays: 4, Backup: backup}).Build(t)
	if err := made.UseBackupSupplier(made.Items()[0].ID(), date(t, "2026-12-02"), day1, owner(t, made)); !errors.Is(err, order.ErrNoBackupSupplier) {
		t.Errorf("in-house: err = %v, want ErrNoBackupSupplier", err)
	}
	sent := testkit.NewOrder().WithItem(testkit.ItemOpts{LeadDays: 4, Backup: backup}).Build(t)
	sent = restoreAt(t, sent, "purchasing", func(r *order.Record) { r.Lines[0].Items[0].Status = order.ItemSentToSupplier })
	if err := sent.UseBackupSupplier(sent.Items()[0].ID(), date(t, "2026-12-02"), day1, owner(t, sent)); !errors.Is(err, order.ErrNoBackupSupplier) {
		t.Errorf("already sent: err = %v, want ErrNoBackupSupplier", err)
	}
	if err := sent.UseBackupSupplier(shared.OrderItemID(testkit.NextID()), date(t, "2026-12-02"), day1, owner(t, sent)); !errors.Is(err, order.ErrItemNotFound) {
		t.Errorf("unknown item: err = %v, want ErrItemNotFound", err)
	}
}
