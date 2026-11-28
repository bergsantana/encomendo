package purchasing

import (
	"context"
	"fmt"

	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// noPromisedAfterDays is how many days a pedido de compra may go without a
// promised date before Hoje asks for one: sent more than 2 days ago
// (docs/plans/m5-purchasing-receiving.md; ADR 0031).
const noPromisedAfterDays = 3

// Today returns Hoje's purchasing entries (purchase:write; any other role
// gets an error matching shared.ErrForbidden, so Hoje asks only the roles
// that may act). Needs come in this order: lines to warn the supplier about,
// then pedidos de compra with no promised date sent more than 2 days ago
// (days counted in the store's time zone), then items to send, by supplier.
func (s *Service) Today(ctx context.Context) (TodayView, error) {
	by, err := authz.Require(ctx, identity.PermPurchaseWrite)
	if err != nil {
		return TodayView{}, err
	}
	var out TodayView
	err = s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		c, err := s.call(ctx, by)
		if err != nil {
			return err
		}
		open, err := s.d.Repo.OpenLines(ctx, by.TenantID())
		if err != nil {
			return fmt.Errorf("read open lines: %w", err)
		}
		notices, err := s.noticesAmong(ctx, open)
		if err != nil {
			return err
		}
		undated, err := s.undated(ctx, c, open, notices)
		if err != nil {
			return err
		}
		tb, err := s.d.Orders.ToBuy(ctx)
		if err != nil {
			return fmt.Errorf("read items to buy: %w", err)
		}
		out.ToSend = len(tb.Items)
		for _, n := range notices {
			out.Needs = append(out.Needs, NeedView{
				Kind: NeedNotifySupplier, SupplierName: n.line.SupplierName, SupplierOrder: n.line.SupplierOrder,
				Number: n.line.Number, Line: n.line.ID, ItemName: n.line.Item.Name,
			})
		}
		out.Needs = append(out.Needs, undated...)
		for _, g := range groupBySupplier(tb.Items) {
			out.Needs = append(out.Needs, NeedView{Kind: NeedToSend, Supplier: g.id, SupplierName: g.name, Count: len(g.items)})
		}
		return nil
	})
	return out, err
}

// undated returns a need for each pedido de compra that has an open line,
// no promised date, and was sent at least noPromisedAfterDays ago in the
// store's zone, oldest first. A line already flagged as a notice does not
// count: the supplier is being told to cancel it.
func (s *Service) undated(ctx context.Context, c call, open []StoredLine, notices []notice) ([]NeedView, error) {
	loc, err := s.d.Store.Location(ctx)
	if err != nil {
		return nil, fmt.Errorf("store time zone: %w", err)
	}
	flagged := make(map[shared.PurchaseLineID]bool, len(notices))
	for _, n := range notices {
		flagged[n.line.ID] = true
	}
	var out []NeedView
	seen := map[shared.SupplierOrderID]bool{}
	for _, l := range open {
		if seen[l.SupplierOrder] || flagged[l.ID] || !l.OrderPromised.IsZero() {
			continue
		}
		days := shared.DateOf(l.SentAt, loc).DaysUntil(c.today)
		if days < noPromisedAfterDays {
			continue
		}
		seen[l.SupplierOrder] = true
		out = append(out, NeedView{
			Kind: NeedNoPromisedDate, SupplierName: l.SupplierName, SupplierOrder: l.SupplierOrder,
			Number: l.Number, Days: days,
		})
	}
	return out, nil
}
