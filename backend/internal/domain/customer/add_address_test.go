package customer

import (
	"errors"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func TestAddAddress_AppendsTrimmedAndKeepsTheRest(t *testing.T) {
	t.Parallel()

	c, err := New(tenantID, customerID, goodSpec())
	if err != nil {
		t.Fatal(err)
	}
	s := goodAddress(workID)
	s.Label = "  Trabalho  "
	got, err := c.AddAddress(s)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != workID || got.Label != "Trabalho" {
		t.Errorf("returned %+v, want the trimmed address with its id", got)
	}
	addrs := c.Addresses()
	if len(addrs) != 2 || addrs[0].ID != homeID || addrs[1].ID != workID {
		t.Errorf("addresses = %+v, want the old one first and the new one last", addrs)
	}
}

func TestAddAddress_NamesBareFields(t *testing.T) {
	t.Parallel()

	c, err := New(tenantID, customerID, goodSpec())
	if err != nil {
		t.Fatal(err)
	}
	bad := goodAddress(workID)
	bad.CEP, bad.Street, bad.State = "123", "", "XX"
	_, err = c.AddAddress(bad)
	f := fieldsOf(t, err)
	if f["cep"] != msgCEPInvalid || f["street"] != msgStreetRequired || f["state"] != msgStateInvalid {
		t.Errorf("fields = %v, want cep, street and state under their bare names", f)
	}
	if len(c.Addresses()) != 1 {
		t.Errorf("a refused address was kept: %d addresses", len(c.Addresses()))
	}
}

func TestAddAddress_LimitOfTen(t *testing.T) {
	t.Parallel()

	s := goodSpec()
	s.Addresses = nil
	for i := range 10 {
		s.Addresses = append(s.Addresses, goodAddress(shared.AddressID(shared.NewV7(int64(100+i), [10]byte{byte(i + 1)}))))
	}
	c, err := New(tenantID, customerID, s)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.AddAddress(goodAddress(workID))
	assertField(t, err, "addresses", msgTooManyAddrs)
	if len(c.Addresses()) != 10 {
		t.Errorf("addresses = %d, want 10", len(c.Addresses()))
	}
}

func TestAddAddress_IDBugs(t *testing.T) {
	t.Parallel()

	c, err := New(tenantID, customerID, goodSpec())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.AddAddress(goodAddress(shared.AddressID{})); !errors.Is(err, ErrInvalidCustomer) {
		t.Errorf("zero id: err = %v, want ErrInvalidCustomer", err)
	}
	if _, err := c.AddAddress(goodAddress(homeID)); !errors.Is(err, ErrInvalidCustomer) {
		t.Errorf("repeated id: err = %v, want ErrInvalidCustomer", err)
	}
	if errors.Is(ErrInvalidCustomer, shared.ErrInvalid) {
		t.Fatal("ErrInvalidCustomer must stay a 500")
	}
}

func TestCheckNewAddress(t *testing.T) {
	t.Parallel()

	a, err := CheckNewAddress(9, goodAddress(workID))
	if err != nil || a.ID != workID {
		t.Fatalf("CheckNewAddress = %+v, %v", a, err)
	}
	bad := goodAddress(workID)
	bad.City = ""
	_, err = CheckNewAddress(0, bad)
	assertField(t, err, "city", msgCityRequired)
	_, err = CheckNewAddress(10, goodAddress(workID))
	assertField(t, err, "addresses", msgTooManyAddrs)
}
