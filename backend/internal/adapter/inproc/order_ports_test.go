package inproc_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/inproc"
	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	appcustomer "github.com/bergsantana/encomendo/backend/internal/app/customer"
	customerfake "github.com/bergsantana/encomendo/backend/internal/app/customer/fake"
	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	apptenancy "github.com/bergsantana/encomendo/backend/internal/app/tenancy"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

func money(t *testing.T, c int64) shared.Money {
	t.Helper()
	m, err := shared.NewMoney(c, "BRL")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestOrderCustomers_FindsByPhoneOrCreates(t *testing.T) {
	t.Parallel()
	ids := sharedfake.NewIDs(sharedfake.NewClock(time.Date(2026, 12, 1, 12, 0, 0, 0, time.UTC)))
	tenant := shared.TenantID(ids.NewID())
	ctx := staff(t, shared.UserID(ids.NewID()), tenant, shared.RoleStaff)
	port := inproc.NewOrderCustomers(appcustomer.NewService(customerfake.NewStore(), sharedfake.NewTxManager(), ids))

	made, err := port.FindOrCreate(ctx, "João Pereira", "5592981234567")
	if err != nil || made.ID.IsZero() || made.Name != "João Pereira" {
		t.Fatalf("create: %+v %v", made, err)
	}
	found, err := port.FindOrCreate(ctx, "Outro nome", "5592981234567")
	if err != nil || found.ID != made.ID || found.Name != "João Pereira" {
		t.Errorf("find by phone: %+v %v, want the same customer unchanged", found, err)
	}
	got, err := port.Get(ctx, made.ID)
	if err != nil || got.Phone != "5592981234567" {
		t.Errorf("get: %+v %v", got, err)
	}
}

func TestOrderCustomers_AddAddressNeedsOnlyOrderWriteAndGivesItsOwnID(t *testing.T) {
	t.Parallel()
	ids := sharedfake.NewIDs(sharedfake.NewClock(time.Date(2026, 12, 1, 12, 0, 0, 0, time.UTC)))
	tenant := shared.TenantID(ids.NewID())
	user := shared.UserID(ids.NewID())
	port := inproc.NewOrderCustomers(appcustomer.NewService(customerfake.NewStore(), sharedfake.NewTxManager(), ids))
	made, err := port.FindOrCreate(staff(t, user, tenant, shared.RoleStaff), "João Pereira", "5592981234567")
	if err != nil {
		t.Fatal(err)
	}
	spec := order.DeliveryAddress{Label: "Obra", CEP: "69000000", Street: "Rua Nova", Number: "5", City: "Manaus", State: "AM"}

	if _, err := port.AddAddress(staff(t, user, tenant, shared.RoleViewer), made.ID, spec); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("viewer: err = %v, want forbidden", err)
	}
	added, err := port.AddAddress(staff(t, user, tenant, shared.RoleStaff), made.ID, spec)
	if err != nil || added.ID.IsZero() || added.Street != "Rua Nova" {
		t.Fatalf("staff: %+v %v", added, err)
	}
	got, err := port.Get(staff(t, user, tenant, shared.RoleStaff), made.ID)
	if err != nil || len(got.Addresses) != 1 || got.Addresses[0].ID != added.ID {
		t.Errorf("customer = %+v %v, want the address added", got, err)
	}
	spec.CEP = "1"
	if _, err := port.AddAddress(staff(t, user, tenant, shared.RoleStaff), made.ID, spec); !errors.Is(err, shared.ErrInvalid) {
		t.Errorf("bad cep: err = %v, want a validation error", err)
	}
}

type stubLines struct{ lines []appcatalog.OrderLine }

func (s stubLines) OrderLines(context.Context, []appcatalog.OrderLineRequest) ([]appcatalog.OrderLine, error) {
	return s.lines, nil
}

func TestOrderCatalog_MapsSnapshotsAndStaffSourcingApart(t *testing.T) {
	t.Parallel()
	cost := money(t, 40_000)
	item, kit := shared.ItemID(shared.NewV7(1, [10]byte{1})), shared.KitID(shared.NewV7(1, [10]byte{2}))
	main, backup := shared.SupplierID(shared.NewV7(1, [10]byte{3})), shared.SupplierID(shared.NewV7(1, [10]byte{4}))
	days := 4
	port := inproc.NewOrderCatalog(stubLines{[]appcatalog.OrderLine{{
		Kit:   &appcatalog.OrderKit{ID: kit, Name: "Kit", Swaps: []appcatalog.OrderSwap{{RemovedName: "A", AddedName: "B"}}},
		Price: money(t, 250_000), AssemblyDays: &days,
		Items: []appcatalog.OrderItem{{
			Item: item, Name: "Banco", SourcingMode: catalog.ModeToOrder, UnitPrice: money(t, 120_000), Quantity: 1, ReadyDays: 15,
			Options:        []appcatalog.ChosenOption{{Name: "Cor", Label: "Vermelha", ExtraPrice: money(t, 5_000), ExtraDays: 2}},
			MainSupplier:   &appcatalog.SupplierFact{ID: main, Name: "Couros", LeadDays: 15, Cost: &cost},
			BackupSupplier: &appcatalog.SupplierFact{ID: backup, Name: "Rio", LeadDays: 6},
			Cost:           &cost,
		}},
	}}})
	specs, err := port.OrderLines(context.Background(), []apporder.LineRequest{{Kit: kit}})
	if err != nil {
		t.Fatal(err)
	}
	l := specs[0]
	if l.Kit == nil || l.Kit.Name != "Kit" || len(l.Kit.Swaps) != 1 || *l.AssemblyDays != 4 || l.Price.Cents() != 250_000 {
		t.Errorf("line = %+v", l)
	}
	it := l.Items[0]
	if it.Snapshot.Name != "Banco" || it.Snapshot.LeadDays != 15 || len(it.Snapshot.Options) != 1 || it.Snapshot.Options[0].ExtraDays != 2 {
		t.Errorf("snapshot = %+v", it.Snapshot)
	}
	if it.Sourcing.Supplier != main || it.Sourcing.SupplierName != "Couros" || it.Sourcing.Cost == nil || it.Sourcing.Backup == nil || it.Sourcing.Backup.LeadDays != 6 {
		t.Errorf("sourcing = %+v", it.Sourcing)
	}
	if !it.ID.IsZero() || !l.ID.IsZero() {
		t.Error("the port must leave ids to the order service")
	}
}

type storeDefaultsStub struct{ v apptenancy.StoreDefaultsView }

func (s storeDefaultsStub) StoreDefaults(context.Context) (apptenancy.StoreDefaultsView, error) {
	return s.v, nil
}

func TestOrderStore_FactsWithTheStoresZone(t *testing.T) {
	t.Parallel()
	port := inproc.NewOrderStore(storeDefaultsStub{apptenancy.StoreDefaultsView{
		Currency: "BRL", AssemblyDays: 3, SafetyMarginDays: 2, DepositPercent: 50, TimeZone: tenancy.ZoneManaus,
		Slug: "altarpm", StoreName: "Alta RPM", WhatsAppPhone: "5592991234567",
	}})
	f, err := port.Facts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if f.Name != "Alta RPM" || f.Slug != "altarpm" || f.DepositPercent != 50 || f.Location == nil || f.Location.String() != "America/Manaus" {
		t.Errorf("facts = %+v", f)
	}
}
