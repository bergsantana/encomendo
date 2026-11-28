package customer_test

import (
	"errors"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/customer"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func TestAddAddress_AddsOneAndBumpsTheVersion(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(t, "Ana", phone, address("Casa"))

	sent := address("Trabalho")
	sent.ID = shared.AddressID(h.ids.NewID())
	added, err := h.svc.AddAddress(h.as(t, h.store1, shared.RoleStaff), v.ID, sent)
	if err != nil {
		t.Fatal(err)
	}
	if added.ID.IsZero() || added.ID == sent.ID || added.Label != "Trabalho" {
		t.Errorf("added = %+v, want a new id of its own", added)
	}
	got, err := h.svc.Get(h.owner(t), v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Addresses) != 2 || got.Addresses[0].ID != v.Addresses[0].ID || got.Addresses[1].ID != added.ID {
		t.Errorf("addresses = %+v, want the old one untouched and the new one last", got.Addresses)
	}
	if got.Version != 2 {
		t.Errorf("version = %d, want 2", got.Version)
	}
}

func TestAddAddress_Permissions(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(t, "Ana", phone)
	for role, allowed := range map[shared.Role]bool{
		shared.RoleOwner: true, shared.RoleAdmin: true, shared.RoleStaff: true, shared.RoleViewer: false,
	} {
		_, err := h.svc.AddAddress(h.as(t, h.store1, role), v.ID, address("Casa"))
		if denied := errors.Is(err, shared.ErrForbidden); denied == allowed || (allowed && err != nil) {
			t.Errorf("%s: err = %v, allowed = %v", role, err, allowed)
		}
	}
}

func TestAddAddress_OtherStoresCustomerIsNotFound(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(t, "Ana", phone)
	_, err := h.svc.AddAddress(h.as(t, h.store2, shared.RoleOwner), v.ID, address("Casa"))
	if !errors.Is(err, customer.ErrNotFound) {
		t.Errorf("err = %v, want not found", err)
	}
}

func TestAddAddress_Refuses(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	bad := address("Casa")
	bad.CEP = "1"
	v := h.create(t, "Ana", phone)
	_, err := h.svc.AddAddress(h.owner(t), v.ID, bad)
	if f := fieldsOf(t, err); len(f) != 1 || f[0] != "cep" {
		t.Errorf("fields = %v, want cep", f)
	}

	full := make([]customer.AddressSpec, 10)
	for i := range full {
		full[i] = address("Endereço")
	}
	many := h.create(t, "Bia", phone2, full...)
	_, err = h.svc.AddAddress(h.owner(t), many.ID, address("Mais um"))
	if f := fieldsOf(t, err); len(f) != 1 || f[0] != "addresses" {
		t.Errorf("fields = %v, want addresses for the limit of 10", f)
	}
	if got, _ := h.svc.Get(h.owner(t), many.ID); len(got.Addresses) != 10 || got.Version != 1 {
		t.Errorf("stored = %d addresses at version %d, want it unchanged", len(got.Addresses), got.Version)
	}
}

func TestAddAddress_RollsBackWhenTheSaveFails(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(t, "Ana", phone)
	h.store.Fail("Update", errBoom)
	_, err := h.svc.AddAddress(h.owner(t), v.ID, address("Casa"))
	h.store.Fail("Update", nil)
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want the store's", err)
	}
	if got, _ := h.svc.Get(h.owner(t), v.ID); len(got.Addresses) != 0 || got.Version != 1 {
		t.Errorf("stored = %+v, want it unchanged", got)
	}
}
