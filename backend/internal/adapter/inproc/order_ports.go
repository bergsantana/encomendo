package inproc

import (
	"context"
	"errors"
	"fmt"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	appcustomer "github.com/bergsantana/encomendo/backend/internal/app/customer"
	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/customer"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// The order context's ports onto the catalog, customer and tenancy
// contexts. Each adapter maps types and nothing more; the use cases on the
// other side check their own permissions.

var _ apporder.Workflows = (*OrderWorkflows)(nil)

// CatalogOrderLines is what OrderCatalog needs of app/catalog:
// *appcatalog.Service.OrderLines.
type CatalogOrderLines interface {
	OrderLines(ctx context.Context, reqs []appcatalog.OrderLineRequest) ([]appcatalog.OrderLine, error)
}

// OrderCatalog implements apporder.Catalog over app/catalog.
type OrderCatalog struct{ catalog CatalogOrderLines }

var _ apporder.Catalog = (*OrderCatalog)(nil)

// NewOrderCatalog returns the order's catalog port.
func NewOrderCatalog(c CatalogOrderLines) *OrderCatalog { return &OrderCatalog{catalog: c} }

// OrderLines resolves the lines through the catalog and maps them to the
// order's line specs, ids left zero.
func (c *OrderCatalog) OrderLines(ctx context.Context, reqs []apporder.LineRequest) ([]order.LineSpec, error) {
	in := make([]appcatalog.OrderLineRequest, len(reqs))
	for i, r := range reqs {
		in[i] = appcatalog.OrderLineRequest{Kit: r.Kit, Item: r.Item, Quantity: r.Quantity, Swaps: r.Swaps}
		for _, o := range r.Options {
			in[i].Options = append(in[i].Options, appcatalog.OptionChoice{Item: o.Item, Option: o.Option, Choice: o.Choice})
		}
	}
	lines, err := c.catalog.OrderLines(ctx, in)
	if err != nil {
		return nil, err
	}
	out := make([]order.LineSpec, len(lines))
	for i, l := range lines {
		out[i] = lineSpec(l)
	}
	return out, nil
}

func lineSpec(l appcatalog.OrderLine) order.LineSpec {
	spec := order.LineSpec{Price: l.Price, AssemblyDays: l.AssemblyDays, SafetyMarginDays: l.SafetyMarginDays}
	if k := l.Kit; k != nil {
		snap := order.KitSnapshot{Kit: k.ID, Name: k.Name}
		for _, sw := range k.Swaps {
			snap.Swaps = append(snap.Swaps, order.SwapSnapshot{Removed: sw.Removed, Added: sw.Added, RemovedName: sw.RemovedName, AddedName: sw.AddedName})
		}
		spec.Kit = &snap
	}
	for _, it := range l.Items {
		spec.Items = append(spec.Items, order.ItemSpec{Snapshot: itemSnapshot(it), Sourcing: sourcing(it)})
	}
	return spec
}

func itemSnapshot(it appcatalog.OrderItem) order.ItemSnapshot {
	snap := order.ItemSnapshot{
		CatalogItem: it.Item, Name: it.Name, SourcingMode: it.SourcingMode, UnitPrice: it.UnitPrice,
		Quantity: it.Quantity, LeadDays: it.ReadyDays,
	}
	for _, o := range it.Options {
		snap.Options = append(snap.Options, order.ChosenOption{
			Option: o.Option, Choice: o.Choice, Name: o.Name, Label: o.Label, ExtraPrice: o.ExtraPrice, ExtraDays: o.ExtraDays,
		})
	}
	return snap
}

// sourcing is the staff-only half: the main supplier, the cost, the backup.
func sourcing(it appcatalog.OrderItem) order.Sourcing {
	src := order.Sourcing{Cost: it.Cost}
	if m := it.MainSupplier; m != nil {
		src.Supplier, src.SupplierName = m.ID, m.Name
	}
	if b := it.BackupSupplier; b != nil {
		src.Backup = &order.BackupSource{Supplier: b.ID, Name: b.Name, LeadDays: b.LeadDays, Cost: b.Cost}
	}
	return src
}

// OrderCustomers implements apporder.Customers over app/customer.
type OrderCustomers struct{ customers appcustomer.UseCases }

var _ apporder.Customers = (*OrderCustomers)(nil)

// NewOrderCustomers returns the order's customer port.
func NewOrderCustomers(c appcustomer.UseCases) *OrderCustomers { return &OrderCustomers{customers: c} }

// Get returns the customer's facts (order:read on the customer side).
func (c *OrderCustomers) Get(ctx context.Context, id shared.CustomerID) (apporder.CustomerFacts, error) {
	v, err := c.customers.Get(ctx, id)
	if err != nil {
		return apporder.CustomerFacts{}, err
	}
	return customerFacts(v), nil
}

// FindOrCreate returns the store's customer with phone, unchanged, or
// creates one with name and phone (order:write on the customer side). When
// a concurrent order created the customer first, it finds that one.
//
// Milestone 7: on the public store page a phone proves nothing, so the
// store page must answer no facts (name, addresses) of a customer matched
// only by phone, and use no stored address (docs/plans/m4-code-design.md
// section 8).
func (c *OrderCustomers) FindOrCreate(ctx context.Context, name, phone string) (apporder.CustomerFacts, error) {
	v, err := c.customers.ByPhone(ctx, phone)
	if err == nil {
		return customerFacts(v), nil
	}
	if !errors.Is(err, customer.ErrNotFound) {
		return apporder.CustomerFacts{}, fmt.Errorf("find customer by phone: %w", err)
	}
	v, err = c.customers.Create(ctx, appcustomer.CreateInput{Spec: customer.Spec{Name: name, Phone: phone}})
	if errors.Is(err, appcustomer.ErrPhoneTaken) {
		v, err = c.customers.ByPhone(ctx, phone)
	}
	if err != nil {
		return apporder.CustomerFacts{}, err
	}
	return customerFacts(v), nil
}

// AddAddress adds a to the customer through the customer use case (order:write
// there, so staff may add an address from an order and never edit one). The
// returned address carries the id the customer gave it.
func (c *OrderCustomers) AddAddress(ctx context.Context, id shared.CustomerID, a order.DeliveryAddress) (order.DeliveryAddress, error) {
	added, err := c.customers.AddAddress(ctx, id, customer.AddressSpec(a))
	if err != nil {
		return order.DeliveryAddress{}, err
	}
	return order.DeliveryAddress(added), nil
}

func customerFacts(v appcustomer.View) apporder.CustomerFacts {
	f := apporder.CustomerFacts{ID: v.ID, Name: v.Name, Phone: v.Phone}
	for _, a := range v.Addresses {
		f.Addresses = append(f.Addresses, order.DeliveryAddress{
			ID: a.ID, Label: a.Label, CEP: a.CEP, Street: a.Street, Number: a.Number,
			Complement: a.Complement, District: a.District, City: a.City, State: a.State,
		})
	}
	return f
}

// OrderStore implements apporder.Store over tenancy's StoreDefaults.
type OrderStore struct{ tenancy TenancyDefaults }

var _ apporder.Store = (*OrderStore)(nil)

// NewOrderStore returns the order's store port.
func NewOrderStore(t TenancyDefaults) *OrderStore { return &OrderStore{tenancy: t} }

// Facts returns the store's identity, defaults and time zone.
func (s *OrderStore) Facts(ctx context.Context) (apporder.StoreFacts, error) {
	v, err := s.tenancy.StoreDefaults(ctx)
	if err != nil {
		return apporder.StoreFacts{}, err
	}
	loc, err := v.TimeZone.Location()
	if err != nil {
		return apporder.StoreFacts{}, fmt.Errorf("store time zone %q: %w", v.TimeZone, err)
	}
	return apporder.StoreFacts{
		Name: v.StoreName, Slug: v.Slug, WhatsApp: v.WhatsAppPhone, Currency: v.Currency,
		DepositPercent: v.DepositPercent, AssemblyDays: v.AssemblyDays, SafetyMarginDays: v.SafetyMarginDays,
		Location: loc, PurchaseBeforeDeposit: v.PurchaseBeforeDeposit,
	}, nil
}
