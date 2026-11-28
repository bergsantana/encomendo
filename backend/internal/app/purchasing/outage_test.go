package purchasing_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

var errBoom = errors.New("boom")

// brokenStore is a Store whose named facts fail.
type brokenStore struct {
	apppurchasing.Store
	today, name, currency, location bool
}

func (s brokenStore) Today(ctx context.Context) (shared.CivilDate, error) {
	if s.today {
		return shared.CivilDate{}, errBoom
	}
	return s.Store.Today(ctx)
}

func (s brokenStore) Name(ctx context.Context) (string, error) {
	if s.name {
		return "", errBoom
	}
	return s.Store.Name(ctx)
}

func (s brokenStore) Currency(ctx context.Context) (string, error) {
	if s.currency {
		return "", errBoom
	}
	return s.Store.Currency(ctx)
}

func (s brokenStore) Location(ctx context.Context) (*time.Location, error) {
	if s.location {
		return nil, errBoom
	}
	return s.Store.Location(ctx)
}

// serviceWith is the world's service over a store that breaks as asked.
func (w *world) serviceWith(broken brokenStore) apppurchasing.UseCases {
	d := w.deps()
	broken.Store = d.Store
	d.Store = broken
	svc, err := apppurchasing.NewService(d)
	if err != nil {
		w.t.Fatal(err)
	}
	return svc
}

// A failing port reaches the caller, wrapped, from every use case that
// reads it, and never as a refusal or a conflict.
func TestService_PortFailuresSurface(t *testing.T) {
	all := []string{"ToBuy", "Preview", "Send", "SupplierOrders", "SupplierOrder", "RecordPromisedDate", "MarkInTransit", "Undo",
		"RecordPaidPrices", "ToReceive", "RequestPhotoUpload", "Receive", "Notices", "ResolveNotice", "OrderLines", "Today"}
	without := func(names ...string) []string {
		return slices.DeleteFunc(slices.Clone(all), func(n string) bool { return slices.Contains(names, n) })
	}
	type armed = func(w *world) apppurchasing.UseCases
	for _, fault := range []struct {
		name     string
		arm      armed
		mustFail []string // the others may not reach the port
	}{
		{"repository", func(w *world) apppurchasing.UseCases { w.repo.Fail(errBoom); return w.svc }, without("RequestPhotoUpload")},
		{"orders", func(w *world) apppurchasing.UseCases { w.items.Fail(errBoom); return w.svc }, without("RequestPhotoUpload", "SupplierOrders")},
		{"store day", func(w *world) apppurchasing.UseCases { return w.serviceWith(brokenStore{today: true}) },
			[]string{"Send", "RecordPromisedDate", "MarkInTransit", "Undo", "RecordPaidPrices", "Receive", "Today"}},
		{"store name", func(w *world) apppurchasing.UseCases { return w.serviceWith(brokenStore{name: true}) },
			[]string{"Preview", "Send", "Notices", "OrderLines"}},
		{"store currency", func(w *world) apppurchasing.UseCases { return w.serviceWith(brokenStore{currency: true}) },
			[]string{"RecordPaidPrices"}},
		{"store zone", func(w *world) apppurchasing.UseCases { return w.serviceWith(brokenStore{location: true}) },
			[]string{"Today"}},
	} {
		t.Run(fault.name, func(t *testing.T) {
			for _, name := range all {
				// Every use case on a world of its own: a call that works
				// changes it for the next.
				w := newWorld(t)
				s := sendTwo(w, w.owner)
				w.items.Cancel(s.order2) // a notice, so the reads go all the way down
				_, spare := w.order(241, bought{name: "Capa", qty: 1})
				svc := fault.arm(w)
				for _, c := range useCasesOf(w, svc, s.view, spare[0], s.order1) {
					if c.name != name {
						continue
					}
					err := c.call(w.ctx(w.owner))
					if slices.Contains(fault.mustFail, name) && !errors.Is(err, errBoom) {
						t.Errorf("%s: err = %v, want the port's failure", name, err)
					}
					if errors.Is(err, errBoom) && (errors.Is(err, shared.ErrInvalid) || errors.Is(err, shared.ErrConflict)) {
						t.Errorf("%s: a port failure became a refusal: %v", name, err)
					}
				}
			}
		})
	}
}

// A supplier whose phone is not a WhatsApp number cannot be linked: the
// link builder's refusal reaches the caller, never a link of nonsense.
func TestService_LinkFailuresSurface(t *testing.T) {
	w := newWorld(t)
	badPhone := apppurchasing.SupplierContact{ID: shared.SupplierID(w.ids.NewID()), Name: "Sem zap", Phone: "123"}
	w.suppliers.Add(badPhone)
	orderID, bad := w.orderOf(badPhone, 232, bought{name: "Pedal", qty: 1})
	ctx := w.ctx(w.owner)

	_, err := w.svc.Send(ctx, apppurchasing.SendInput{PickInput: apppurchasing.PickInput{Supplier: badPhone.ID, OrderItems: bad}, Idempotency: w.key()})
	if !errors.Is(err, shared.ErrInvalid) {
		t.Fatalf("send: %v", err)
	}
	if _, err := w.svc.OrderLines(ctx, orderID); !errors.Is(err, shared.ErrInvalid) {
		t.Fatalf("order lines: %v", err)
	}
	// Without the chat link, the same order reads fine.
	if _, err := w.svc.OrderLines(w.ctx(w.staff), orderID); err != nil {
		t.Fatalf("order lines without chat: %v", err)
	}
	// A supplier the directory does not know.
	missing := apppurchasing.SupplierContact{ID: shared.SupplierID(w.ids.NewID()), Name: "Fantasma", Phone: supplierPhone}
	_, ghost := w.orderOf(missing, 233, bought{name: "Capa", qty: 1})
	if _, err := w.svc.ToBuy(ctx); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("to buy with an unknown supplier: %v", err)
	}
	if _, err := w.svc.Preview(ctx, apppurchasing.PickInput{Supplier: missing.ID, OrderItems: ghost}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("preview: %v", err)
	}
}
