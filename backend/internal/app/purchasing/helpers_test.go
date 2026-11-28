package purchasing_test

import (
	"context"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/app/purchasing/fake"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

// start is the fixed clock: 15:00 UTC, noon in the store's zone (UTC-3).
var start = time.Date(2026, 12, 1, 15, 0, 0, 0, time.UTC)

// storeZone is the store's time zone, UTC-3 the year round.
var storeZone = time.FixedZone("BRT", -3*3600)

const supplierPhone = "5511987654321"

// txProbe is a TxManager that says whether a call runs inside one.
type txProbe struct {
	inner *sharedfake.TxManager
	depth atomic.Int32
}

func (p *txProbe) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	p.depth.Add(1)
	defer p.depth.Add(-1)
	return p.inner.WithinTx(ctx, fn)
}

func (p *txProbe) Inside() bool { return p.depth.Load() > 0 }

// customer is the client of a fake order. None of these fields may reach a
// supplier text.
type customer struct{ Name, Phone, Address, Email string }

var maria = customer{Name: "Maria Aparecida Souza", Phone: "5592981234567", Address: "Rua das Flores, 123", Email: "maria@example.com"}

func (c customer) fields() []string { return []string{c.Name, c.Phone, c.Address, c.Email, "Maria"} }

// world is a purchasing service over fakes, with a store, a supplier and
// one principal per role.
type world struct {
	t         *testing.T
	clock     *sharedfake.Clock
	ids       *sharedfake.IDs
	tenant    shared.TenantID
	today     shared.CivilDate
	items     *fake.OrderItems
	repo      *fake.Repository
	suppliers *fake.Suppliers
	photos    *fake.ReceiptPhotos
	tx        *txProbe
	idem      *sharedfake.Idempotency
	svc       *apppurchasing.Service
	supplier  apppurchasing.SupplierContact
	customers map[shared.OrderID]customer
	keys      int
	catalog   map[string]shared.ItemID // catalog item by name

	owner, admin, staff, viewer shared.Principal
}

// bought is one bought item of a test order.
type bought struct {
	name    string
	qty     int
	choices []purchasing.Choice
	cost    int64 // cents; 0 for none
}

func newWorld(t *testing.T) *world {
	t.Helper()
	clock := sharedfake.NewClock(start)
	ids := sharedfake.NewIDs(clock)
	tenant := shared.TenantID(ids.NewID())
	today := shared.DateOf(start, storeZone)
	w := &world{
		t: t, clock: clock, ids: ids, tenant: tenant, today: today,
		items: fake.NewOrderItems(today), repo: fake.NewRepository(), suppliers: fake.NewSuppliers(),
		photos: fake.NewReceiptPhotos(clock), tx: &txProbe{inner: sharedfake.NewTxManager()},
		customers: map[shared.OrderID]customer{}, catalog: map[string]shared.ItemID{}, idem: sharedfake.NewIdempotency(clock),
		owner: testkit.Staff(t, tenant, shared.RoleOwner), admin: testkit.Staff(t, tenant, shared.RoleAdmin),
		staff: testkit.Staff(t, tenant, shared.RoleStaff), viewer: testkit.Staff(t, tenant, shared.RoleViewer),
	}
	w.repo.LinkCosts(w.items)
	w.repo.NameUser(w.owner.UserID(), "Dona Rita")
	w.repo.NameUser(w.admin.UserID(), "Seu Jorge")
	w.supplier = apppurchasing.SupplierContact{ID: shared.SupplierID(ids.NewID()), Name: "Couros Paulista", Phone: supplierPhone}
	w.suppliers.Add(w.supplier)
	svc, err := apppurchasing.NewService(w.deps())
	if err != nil {
		t.Fatal(err)
	}
	w.svc = svc
	return w
}

func (w *world) deps() apppurchasing.Deps {
	return apppurchasing.Deps{
		Repo: w.repo, Orders: w.items, Suppliers: w.suppliers,
		Store: fake.NewStore("Alta RPM", "BRL", storeZone, w.clock), Links: fake.SupplierLinks{}, Photos: w.photos,
		Idempotency: w.idem, Tx: w.tx, Clock: w.clock, IDs: w.ids,
	}
}

// ctx returns a context for who, with the scope a request would carry.
func (w *world) ctx(who shared.Principal) context.Context {
	return authz.WithPrincipal(context.Background(), who)
}

// key returns a new Idempotency-Key.
func (w *world) key() appshared.IdempotencyKey {
	w.keys++
	return appshared.IdempotencyKey{Key: "k" + strconv.Itoa(w.keys), RequestHash: [32]byte{byte(w.keys)}}
}

// order adds a released order of the supplier with one item per bought.
func (w *world) order(number int, items ...bought) (shared.OrderID, []shared.OrderItemID) {
	w.t.Helper()
	return w.orderOf(w.supplier, number, items...)
}

func (w *world) orderOf(sup apppurchasing.SupplierContact, number int, items ...bought) (shared.OrderID, []shared.OrderItemID) {
	w.t.Helper()
	id := shared.OrderID(w.ids.NewID())
	facts := apppurchasing.OrderFacts{Order: id, Number: order.Number(number).String(), ArrivalLimit: w.today.AddDays(17)}
	var fi []fake.Item
	var ids []shared.OrderItemID
	for _, b := range items {
		qty := max(b.qty, 1)
		item := shared.OrderItemID(w.ids.NewID())
		ids = append(ids, item)
		var cost *shared.Money
		if b.cost > 0 {
			c := testkit.BRL(w.t, b.cost)
			cost = &c
		}
		if _, ok := w.catalog[b.name]; !ok {
			w.catalog[b.name] = shared.ItemID(w.ids.NewID())
		}
		fi = append(fi, fake.Item{
			CatalogItem: w.catalog[b.name], Cost: cost,
			Facts: apppurchasing.OrderItemFacts{
				Item: item, Bought: true, Status: order.ItemToBuy, Name: b.name, Quantity: qty, Choices: b.choices,
				Expected: w.today.AddDays(15), Supplier: sup.ID, SupplierName: sup.Name,
			},
		})
	}
	w.items.AddOrder(facts, true, fi...)
	w.customers[id] = maria
	return id, ids
}

// send sends the items to the supplier as who and returns the supplier order.
func (w *world) send(who shared.Principal, ids ...shared.OrderItemID) apppurchasing.SentView {
	w.t.Helper()
	v, err := w.svc.Send(w.ctx(who), apppurchasing.SendInput{
		PickInput:   apppurchasing.PickInput{Supplier: w.supplier.ID, OrderItems: ids},
		Idempotency: w.key(),
	})
	if err != nil {
		w.t.Fatalf("send: %v", err)
	}
	return v
}

// itemStatus returns the status of an order item as the order context has it.
func (w *world) itemStatus(orderID shared.OrderID, item shared.OrderItemID) order.ItemStatus {
	w.t.Helper()
	got, err := w.items.Orders(context.Background(), []shared.OrderID{orderID})
	if err != nil {
		w.t.Fatal(err)
	}
	f, ok := got[0].Item(item)
	if !ok {
		w.t.Fatalf("item %s not in order", item)
	}
	return f.Status
}

// facts returns the order as the order context has it.
func (w *world) facts(orderID shared.OrderID) apppurchasing.OrderFacts {
	w.t.Helper()
	got, err := w.items.Orders(context.Background(), []shared.OrderID{orderID})
	if err != nil {
		w.t.Fatal(err)
	}
	return got[0]
}

var preto = purchasing.Choice{Option: shared.OptionID(testkit.NextID()), ChoiceID: shared.ChoiceID(testkit.NextID()), Name: "Cor", Label: "Preto"}

// storedMessage reads the message a supplier order was sent with.
func storedMessage(t *testing.T, w *world, id shared.SupplierOrderID) apppurchasing.SentMessage {
	t.Helper()
	var msg apppurchasing.SentMessage
	err := w.tx.WithinTx(w.ctx(w.owner), func(ctx context.Context) error {
		var err error
		msg, err = w.repo.Message(ctx, w.tenant, id)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return msg
}

// peek returns the number the next supplier order would get.
func (w *world) peek(who shared.Principal) (purchasing.Number, error) {
	var n purchasing.Number
	err := w.tx.WithinTx(w.ctx(who), func(ctx context.Context) error {
		var err error
		n, err = w.repo.PeekNumber(ctx, w.tenant)
		return err
	})
	return n, err
}

// remember stores that key made resource, as an earlier call would have.
func remember(t *testing.T, w *world, k appshared.IdempotencyKey, resource string) {
	t.Helper()
	err := w.tx.WithinTx(w.ctx(w.owner), func(ctx context.Context) error {
		return w.idem.Remember(ctx, k, resource, time.Hour)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// itemOrder returns the order that holds an item.
func (w *world) itemOrder(item shared.OrderItemID) shared.OrderID {
	w.t.Helper()
	tb, err := w.items.ToBuy(context.Background())
	if err != nil {
		w.t.Fatal(err)
	}
	for _, it := range tb.Items {
		if it.Item.OrderItem == item {
			return it.Item.Order
		}
	}
	w.t.Fatalf("item %s is not to buy", item)
	return shared.OrderID{}
}

// sendThenRewind makes the repository hold an open line for an item that
// the order context still shows to buy.
func (w *world) sendThenRewind(orderID shared.OrderID, item shared.OrderItemID) {
	w.t.Helper()
	w.send(w.owner, item)
	if err := w.items.Move(context.Background(), []apppurchasing.ItemMove{{Order: orderID, Item: item, To: order.ItemToBuy}}); err != nil {
		w.t.Fatal(err)
	}
}

// fakeItem is an item of a hand-made order, in any status.
func fakeItem(w *world, name string, boughtItem bool, status order.ItemStatus) fake.Item {
	return fake.Item{
		CatalogItem: shared.ItemID(w.ids.NewID()),
		Facts: apppurchasing.OrderItemFacts{
			Item: shared.OrderItemID(w.ids.NewID()), Bought: boughtItem, Status: status, Name: name, Quantity: 1,
			Supplier: w.supplier.ID, SupplierName: w.supplier.Name,
		},
	}
}
