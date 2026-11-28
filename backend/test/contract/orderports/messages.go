package orderports

import (
	"context"
	"errors"
	"testing"
	"time"

	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

var messageCases = []contractCase{
	{"a message is recorded for an order of the store", messageRecorded},
	{"every method refuses to run outside a transaction", noMethodRunsOutsideATransaction},
	{"staff names are read by user id", staffNamesRead},
}

// staffNamesRead: the users' names come back by id, and an unknown user is
// absent ("Registrado por" on payments).
func staffNamesRead(t *testing.T, h Harness) {
	s := h.newStore(t)
	ana := h.NewUser(t, "Ana Souza")
	gone := shared.UserID(h.NewID())
	var names map[shared.UserID]string
	h.in(t, s, func(ctx context.Context) error {
		var err error
		names, err = h.Orders.StaffNames(ctx, []shared.UserID{ana, gone})
		return err
	})
	if len(names) != 1 || names[ana] != "Ana Souza" {
		t.Errorf("names = %v, want only Ana Souza", names)
	}
}

func messageRecorded(t *testing.T, h Harness) {
	s := h.newStore(t)
	o := h.insert(t, s)
	h.in(t, s, func(ctx context.Context) error {
		return h.Orders.AddMessage(ctx, apporder.Message{
			ID: h.NewID(), Order: o.ID(), Kind: apporder.MessageCreated, ToPhone: "5592999990000",
			Text: "Olá! Seu pedido foi criado.", URL: "https://wa.me/5592999990000?text=Ol%C3%A1", By: s.user, At: epoch,
		})
	})
}

func noMethodRunsOutsideATransaction(t *testing.T, h Harness) {
	ctx := context.Background()
	tenant, id, s := shared.TenantID(h.NewID()), shared.OrderID(h.NewID()), h.newStore(t)
	from, to := epoch, epoch.Add(time.Hour)
	filter := apporder.ListFilter{Scope: apporder.ScopeAll}
	calls := map[string]func() error{
		"NextNumber": func() error { _, err := h.Orders.NextNumber(ctx, tenant); return err },
		"Insert":     func() error { return h.Orders.Insert(ctx, h.newOrder(t, s, 1)) },
		"Update":     func() error { return h.Orders.Update(ctx, h.newOrder(t, s, 1)) },
		"ByID":       func() error { _, err := h.Orders.ByID(ctx, tenant, id); return err },
		"AddMessage": func() error { return h.Orders.AddMessage(ctx, apporder.Message{}) },
		"List":       func() error { _, err := h.Orders.List(ctx, tenant, filter, page(t, "", 10)); return err },
		"Active":     func() error { _, err := h.Orders.Active(ctx, tenant, 10); return err },
		"History":    func() error { _, err := h.Orders.History(ctx, tenant, id, page(t, "", 10)); return err },
		"Payments":   func() error { _, err := h.Orders.Payments(ctx, tenant, from, to); return err },
		"HandedOver": func() error { _, err := h.Orders.HandedOver(ctx, tenant, from, to); return err },
	}
	for name, call := range calls {
		if err := call(); !errors.Is(err, appshared.ErrNoTransaction) {
			t.Errorf("%s outside a transaction err = %v, want appshared.ErrNoTransaction", name, err)
		}
	}
}
