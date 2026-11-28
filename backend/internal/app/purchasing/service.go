package purchasing

import (
	"context"
	"fmt"
	"slices"
	"time"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// idempotencyTTL is how long a key is remembered (ADR 0030).
const idempotencyTTL = 24 * time.Hour

// Deps are the Service's ports. Every one is required. Purchasing
// publishes no events in milestone 5, so there is no publisher.
type Deps struct {
	Repo        Repository
	Orders      OrderItems
	Suppliers   Suppliers
	Store       Store
	Links       SupplierLinks
	Photos      ReceiptPhotos
	Idempotency appshared.Idempotency
	Tx          appshared.TxManager
	Clock       appshared.Clock
	IDs         appshared.IDGenerator
}

// Service implements UseCases.
type Service struct{ d Deps }

var _ UseCases = (*Service)(nil)

// NewService returns a Service over d, or an error naming the first
// dependency that is missing.
func NewService(d Deps) (*Service, error) {
	for _, dep := range []struct {
		name    string
		missing bool
	}{
		{"Repo", d.Repo == nil}, {"Orders", d.Orders == nil}, {"Suppliers", d.Suppliers == nil},
		{"Store", d.Store == nil}, {"Links", d.Links == nil}, {"Photos", d.Photos == nil},
		{"Idempotency", d.Idempotency == nil}, {"Tx", d.Tx == nil}, {"Clock", d.Clock == nil}, {"IDs", d.IDs == nil},
	} {
		if dep.missing {
			return nil, fmt.Errorf("purchasing service: %s is required", dep.name)
		}
	}
	return &Service{d: d}, nil
}

// call is what one use case call reads once, so every date it computes
// falls on the same store day: who is calling, the time and the store's day.
type call struct {
	by    shared.Principal
	now   time.Time
	today shared.CivilDate
}

func (s *Service) call(ctx context.Context, by shared.Principal) (call, error) {
	today, err := s.d.Store.Today(ctx)
	if err != nil {
		return call{}, fmt.Errorf("store day: %w", err)
	}
	return call{by: by, now: s.d.Clock.Now(), today: today}, nil
}

// owner reports whether by sees costs and prices paid.
func owner(by shared.Principal) bool { return identity.SeesCosts(by.Role()) }

// orderFacts are the orders purchasing read for one call, by id.
type orderFacts map[shared.OrderID]OrderFacts

// facts reads the orders of ids, each once. An order the store does not
// have is an error matching shared.ErrNotFound.
func (s *Service) facts(ctx context.Context, ids ...shared.OrderID) (orderFacts, error) {
	var uniq []shared.OrderID
	for _, id := range ids {
		if !slices.Contains(uniq, id) {
			uniq = append(uniq, id)
		}
	}
	out := make(orderFacts, len(uniq))
	if len(uniq) == 0 {
		return out, nil
	}
	got, err := s.d.Orders.Orders(ctx, uniq)
	if err != nil {
		return nil, fmt.Errorf("read orders: %w", err)
	}
	for _, f := range got {
		out[f.Order] = f
	}
	return out, nil
}

// has reports whether the order is there and still has item.
func (f orderFacts) has(order shared.OrderID, item shared.OrderItemID) bool {
	o, ok := f[order]
	if !ok || item.IsZero() {
		return false
	}
	_, found := o.Item(item)
	return found
}

// movable reports whether purchasing may move item of order: the order is
// open and still has the item. A line of a cancelled or handed over order,
// or of a removed item, is still changed on its own, but its item needs no
// move (ADR 0031: such an item "is still received").
func (f orderFacts) movable(order shared.OrderID, item shared.OrderItemID) bool {
	return f.has(order, item) && !f[order].Closed
}

// ordersOf returns the orders of lines, for a read of their facts.
func ordersOf(lines []purchasing.Line) []shared.OrderID {
	ids := make([]shared.OrderID, len(lines))
	for i, l := range lines {
		ids[i] = l.Item().Order
	}
	return ids
}

// loadSupplierOrder reads and restores a supplier order.
func (s *Service) loadSupplierOrder(ctx context.Context, tenant shared.TenantID, id shared.SupplierOrderID) (*purchasing.SupplierOrder, error) {
	rec, err := s.d.Repo.SupplierOrder(ctx, tenant, id)
	if err != nil {
		return nil, fmt.Errorf("load supplier order %s: %w", id, err)
	}
	return restore(rec)
}

func restore(rec purchasing.Record) (*purchasing.SupplierOrder, error) {
	so, err := purchasing.Restore(rec)
	if err != nil {
		return nil, fmt.Errorf("restore supplier order %s: %w", rec.ID, err)
	}
	return so, nil
}

func choiceViews(cs []purchasing.Choice) []ChoiceView {
	out := make([]ChoiceView, len(cs))
	for i, c := range cs {
		out[i] = ChoiceView{Name: c.Name, Label: c.Label}
	}
	return out
}
