//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/adapter/inproc"
	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres"
	appcustomer "github.com/bergsantana/encomendo/backend/internal/app/customer"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/customer"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

// pgIDs issues ids for the customer service from the integration helpers.
type pgIDs struct{}

func (pgIDs) NewID() shared.ID { return pgtest.NewID() }

// "Novo endereço" from an order goes through the order's Customers port, the
// customer use case and the real repository: staff add an address under RLS,
// the customer's other addresses and version bookkeeping stay as they were,
// and the address limit holds.
func TestOrderCustomers_AddAddressStoresItUnderRLS(t *testing.T) {
	e := newCustomerEnv(t)
	home := addressSpec("Casa")
	c := e.add("Ana", home)

	svc := appcustomer.NewService(e.repo, e.tx, pgIDs{})
	port := inproc.NewOrderCustomers(svc)
	p, err := shared.NewBackofficePrincipal(e.seed.User(), e.tenant, shared.RoleStaff)
	if err != nil {
		t.Fatal(err)
	}
	ctx := authz.WithPrincipal(context.Background(), p)

	added, err := port.AddAddress(ctx, c.ID(), order.DeliveryAddress{
		Label: "Obra", CEP: "69000000", Street: "Rua Nova", Number: "5", City: "Manaus", State: "AM",
	})
	if err != nil || added.ID.IsZero() {
		t.Fatalf("AddAddress: %+v %v", added, err)
	}
	got, err := svc.Get(ctx, c.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Addresses) != 2 || got.Addresses[0].ID != home.ID || got.Addresses[1].ID != added.ID || got.Version != 2 {
		t.Errorf("customer = %+v, want the old address first, the new one second, version 2", got)
	}

	// The owner of another store finds no such customer to add to.
	other := newCustomerEnv(t)
	otherP, err := shared.NewBackofficePrincipal(other.seed.User(), other.tenant, shared.RoleOwner)
	if err != nil {
		t.Fatal(err)
	}
	_, err = port.AddAddress(authz.WithPrincipal(context.Background(), otherP), c.ID(), order.DeliveryAddress{
		Label: "x", CEP: "69000000", Street: "R", City: "Manaus", State: "AM",
	})
	if !errors.Is(err, customer.ErrNotFound) {
		t.Errorf("another store: err = %v, want not found", err)
	}
}

func TestOrderCustomers_AddAddressStopsAtTen(t *testing.T) {
	e := newCustomerEnv(t)
	specs := make([]customer.AddressSpec, 10)
	for i := range specs {
		specs[i] = addressSpec("Endereço")
	}
	c := e.add("Bia", specs...)
	port := inproc.NewOrderCustomers(appcustomer.NewService(postgres.NewCustomers(), e.tx, pgIDs{}))
	p, err := shared.NewBackofficePrincipal(e.seed.User(), e.tenant, shared.RoleStaff)
	if err != nil {
		t.Fatal(err)
	}
	_, err = port.AddAddress(authz.WithPrincipal(context.Background(), p), c.ID(), order.DeliveryAddress{
		Label: "11", CEP: "69000000", Street: "R", City: "Manaus", State: "AM",
	})
	if !errors.Is(err, shared.ErrInvalid) {
		t.Errorf("eleventh address: err = %v, want a validation error", err)
	}
}
