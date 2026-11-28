package purchasingports

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/inproc"
	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	orderfake "github.com/bergsantana/encomendo/backend/internal/app/order/fake"
	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/app/purchasing/fake"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

var start = time.Date(2026, 12, 1, 15, 0, 0, 0, time.UTC)

func TestFakeOrderItems_Contract(t *testing.T) {
	RunOrderItemsContract(t, func(t *testing.T) OrderItemsHarness {
		today := testkit.DateOf(start)
		items := fake.NewOrderItems(today)
		ids := sharedfake.NewIDs(sharedfake.NewClock(start))
		n := 0
		add := func(t *testing.T, purchasable bool) (shared.OrderID, shared.OrderItemID) {
			n++
			o, it := shared.OrderID(ids.NewID()), shared.OrderItemID(ids.NewID())
			items.AddOrder(apppurchasing.OrderFacts{Order: o, Number: order.Number(n).String(), ArrivalLimit: today.AddDays(17)}, purchasable,
				fake.Item{CatalogItem: shared.ItemID(ids.NewID()), Facts: apppurchasing.OrderItemFacts{
					Item: it, Bought: true, Status: order.ItemToBuy, Name: "Banco de couro", Quantity: 1,
					Supplier: shared.SupplierID(ids.NewID()), SupplierName: "Couros Paulista", Expected: today.AddDays(15),
				}})
			return o, it
		}
		return OrderItemsHarness{
			Items: items, Ctx: context.Background(), Today: today,
			Released: func(t *testing.T) (shared.OrderID, shared.OrderItemID) { return add(t, true) },
			Draft:    func(t *testing.T) (shared.OrderID, shared.OrderItemID) { return add(t, false) },
			Cancel:   func(_ *testing.T, id shared.OrderID) { items.Cancel(id) },
		}
	})
}

// The inproc adapter over a real app/order with its fakes: the order
// context's rules and permissions behind the port (ADR 0031).
func TestInprocOrderItems_Contract(t *testing.T) {
	RunOrderItemsContract(t, func(t *testing.T) OrderItemsHarness {
		clock := sharedfake.NewClock(start)
		ids := sharedfake.NewIDs(clock)
		tenant := shared.TenantID(ids.NewID())
		repo, cat, customers := orderfake.NewOrders(), orderfake.NewCatalog(), orderfake.NewCustomers(ids.NewID)
		workflows := orderfake.NewWorkflows(testkit.Preset(t, testkit.PresetEncomendaMontagem))
		repo.Link(workflows, customers)
		svc, err := apporder.NewService(apporder.Deps{
			Repo: repo, Catalog: cat, Customers: customers, Workflows: workflows,
			Store: orderfake.NewStore(apporder.StoreFacts{Name: "Alta RPM", Slug: "altarpm", Currency: "BRL",
				DepositPercent: 50, AssemblyDays: 3, SafetyMarginDays: 2, Location: time.UTC}),
			Tracking: orderfake.Tracking{}, WhatsApp: orderfake.WhatsApp{},
			Idempotency: sharedfake.NewIdempotency(clock), Secrets: sharedfake.NewSecrets(),
			Tx: sharedfake.NewTxManager(), Events: sharedfake.NewEvents(), Clock: clock, IDs: ids,
		})
		if err != nil {
			t.Fatal(err)
		}
		banco := shared.ItemID(ids.NewID())
		cost := testkit.BRL(t, 40_000)
		cat.AddItem(orderfake.CatalogItem{ID: banco, Name: "Banco de couro", Mode: catalog.ModeToOrder, Price: testkit.BRL(t, 100_000),
			LeadDays: 15, Sourcing: order.Sourcing{Supplier: shared.SupplierID(ids.NewID()), SupplierName: "Couros Paulista", Cost: &cost}})
		customer := apporder.CustomerFacts{ID: shared.CustomerID(ids.NewID()), Name: "João Pereira", Phone: "5592981234567"}
		customers.Add(customer)
		as := func(t *testing.T, role shared.Role) context.Context {
			return authz.WithPrincipal(context.Background(), testkit.Staff(t, tenant, role))
		}
		ctx := as(t, shared.RoleOwner)
		key := 0
		create := func(t *testing.T, pay bool) (shared.OrderID, shared.OrderItemID) {
			key++
			c, err := svc.Create(ctx, apporder.CreateInput{Customer: customer.ID, Lines: []apporder.LineRequest{{Item: banco, Quantity: 1}},
				Terms: apporder.TermsInput{Handover: order.HandoverPickup}, SendToCustomer: true,
				Idempotency: idem(key)})
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			if pay {
				key++
				if _, err := svc.RecordPayment(ctx, apporder.PaymentInput{Order: c.Order.ID, Kind: order.PaymentDeposit,
					AmountCents: c.Order.DepositDue.Cents(), Method: order.MethodPix, Version: c.Order.Version, Idempotency: idem(key)}); err != nil {
					t.Fatalf("deposit: %v", err)
				}
			}
			return c.Order.ID, c.Order.Items[0].ID
		}
		return OrderItemsHarness{
			Items: inproc.NewPurchasingOrders(svc), Ctx: ctx, Today: testkit.DateOf(start), As: as,
			Released: func(t *testing.T) (shared.OrderID, shared.OrderItemID) { return create(t, true) },
			Draft:    func(t *testing.T) (shared.OrderID, shared.OrderItemID) { return create(t, false) },
			Cancel: func(t *testing.T, id shared.OrderID) {
				v, err := svc.Get(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := svc.Cancel(ctx, apporder.CancelInput{Order: id, Reason: "Desistiu", Version: v.Version}); err != nil {
					t.Fatal(err)
				}
			},
		}
	})
}

// idem is the n-th Idempotency-Key of a harness.
func idem(n int) appshared.IdempotencyKey {
	return appshared.IdempotencyKey{Key: "k" + strconv.Itoa(n), RequestHash: [32]byte{byte(n), byte(n >> 8)}}
}
