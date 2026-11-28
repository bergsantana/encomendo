package customerports

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	appcustomer "github.com/bergsantana/encomendo/backend/internal/app/customer"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/customer"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Harness is the customer repository with what the suite needs to use it.
type Harness struct {
	Customers appcustomer.Repository
	Tx        appshared.TxManager
	NewID     func() shared.ID
	// NewTenant returns a store that customers may belong to. The Postgres
	// harness seeds a tenants row; the fake needs none.
	NewTenant func(t *testing.T) shared.TenantID
	// AddOrder gives the customer an order created at at. The orders table
	// belongs to another context, so each harness seeds it its own way.
	// Times have microsecond precision.
	AddOrder func(t *testing.T, tenant shared.TenantID, id shared.CustomerID, at time.Time)
}

// tenant is one store of the suite with the staff user that acts in it.
type tenant struct {
	id   shared.TenantID
	user shared.UserID
}

func (h Harness) newTenant(t *testing.T) tenant {
	t.Helper()
	return tenant{id: h.NewTenant(t), user: shared.UserID(h.NewID())}
}

func (tn tenant) scope() appshared.Scope {
	return appshared.Scope{TenantID: tn.id, UserID: tn.user}
}

func (h Harness) in(t *testing.T, s appshared.Scope, fn func(ctx context.Context) error) {
	t.Helper()
	if err := h.try(s, fn); err != nil {
		t.Fatal(err)
	}
}

func (h Harness) try(s appshared.Scope, fn func(ctx context.Context) error) error {
	return h.Tx.WithinTx(appshared.WithScope(context.Background(), s), fn)
}

var phones atomic.Int64

// newPhone returns a number no other call of the process returned.
func newPhone() string { return fmt.Sprintf("55929%08d", phones.Add(1)) }

// address returns a valid address spec with a new id.
func (h Harness) address(label string) customer.AddressSpec {
	return customer.AddressSpec{
		ID: shared.AddressID(h.NewID()), Label: label, CEP: "69000000", Street: "Rua das Flores",
		Number: "10", Complement: "Apto 2", District: "Centro", City: "Manaus", State: "AM",
	}
}

func (h Harness) build(t *testing.T, tn tenant, name, phone string, addresses ...customer.AddressSpec) *customer.Customer {
	t.Helper()
	c, err := customer.New(tn.id, shared.CustomerID(h.NewID()), customer.Spec{
		Name: name, Phone: phone, Email: "Pessoa@Example.com", Notes: "Prefere à tarde.", Addresses: addresses,
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// add stores a customer of tn with a new number.
func (h Harness) add(t *testing.T, tn tenant, name string, addresses ...customer.AddressSpec) *customer.Customer {
	t.Helper()
	c := h.build(t, tn, name, newPhone(), addresses...)
	h.in(t, tn.scope(), func(ctx context.Context) error { return h.Customers.Insert(ctx, c) })
	return c
}

// load reads a customer of tn back.
func (h Harness) load(t *testing.T, tn tenant, id shared.CustomerID) *customer.Customer {
	t.Helper()
	var got *customer.Customer
	h.in(t, tn.scope(), func(ctx context.Context) error {
		var err error
		got, err = h.Customers.ByID(ctx, tn.id, id)
		return err
	})
	return got
}

// replace changes c as the app layer does and stores it, in tn's scope.
func (h Harness) replace(tn tenant, c *customer.Customer, s customer.Spec) error {
	if err := c.Replace(s, c.Version()); err != nil {
		return err
	}
	return h.try(tn.scope(), func(ctx context.Context) error { return h.Customers.Update(ctx, c) })
}

func page(t *testing.T, cursor string, limit int) shared.PageRequest {
	t.Helper()
	p, err := shared.NewPageRequest(cursor, limit)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func names(p shared.Page[appcustomer.Summary]) []string {
	out := make([]string, 0, len(p.Items))
	for _, s := range p.Items {
		out = append(out, s.Name)
	}
	return out
}

func labels(c *customer.Customer) []string {
	var out []string
	for _, a := range c.Addresses() {
		out = append(out, a.Label)
	}
	return out
}

func specOf(c *customer.Customer, addresses ...customer.AddressSpec) customer.Spec {
	return customer.Spec{Name: c.Name(), Phone: c.Phone(), Email: c.Email().String(), Notes: c.Notes(), Addresses: addresses}
}

// RunRepositoryContract checks the customer repository: reading back what
// was stored, optimistic updates, the unique number, addresses replaced as a
// set, ordering and paging, the filter, the order count, and what each scope
// sees and may write (tenant_isolation of 0012_customers). Names in the
// suite are ASCII letters, so the order does not depend on a database
// collation.
func RunRepositoryContract(t *testing.T, newHarness func(t *testing.T) Harness) {
	t.Run("a customer reads back as inserted", func(t *testing.T) { insertAndGet(t, newHarness(t)) })
	t.Run("an unknown customer is not found", func(t *testing.T) { getUnknown(t, newHarness(t)) })
	t.Run("a customer is found by number", func(t *testing.T) { byPhone(t, newHarness(t)) })
	t.Run("updates are optimistic", func(t *testing.T) { updateOptimistic(t, newHarness(t)) })
	t.Run("an unknown customer cannot be updated", func(t *testing.T) { updateUnknown(t, newHarness(t)) })
	t.Run("a duplicate id is a conflict", func(t *testing.T) { insertDuplicate(t, newHarness(t)) })
	t.Run("a number is unique per store", func(t *testing.T) { phoneTaken(t, newHarness(t)) })
	t.Run("addresses are replaced as a set", func(t *testing.T) { addressesAsSet(t, newHarness(t)) })
	t.Run("an address of another customer is refused", func(t *testing.T) { addressOfAnother(t, newHarness(t)) })
	t.Run("a failed transaction undoes an insert", func(t *testing.T) { rollbackInsert(t, newHarness(t)) })
	t.Run("a failed transaction undoes an update", func(t *testing.T) { rollbackUpdate(t, newHarness(t)) })
	t.Run("the list orders by name, then id, and pages", func(t *testing.T) { listPages(t, newHarness(t)) })
	t.Run("the last page ends at an exact boundary", func(t *testing.T) { listExactBoundary(t, newHarness(t)) })
	t.Run("the list filters by name prefix and number", func(t *testing.T) { listFilter(t, newHarness(t)) })
	t.Run("the filter is literal text", func(t *testing.T) { listFilterLiteral(t, newHarness(t)) })
	t.Run("a cursor survives a rename and a filter", func(t *testing.T) { listCursorSurvives(t, newHarness(t)) })
	t.Run("a cursor the repository did not issue is invalid", func(t *testing.T) { listBadCursor(t, newHarness(t)) })
	t.Run("the list counts orders", func(t *testing.T) { listOrders(t, newHarness(t)) })
	t.Run("another store's customers are invisible", func(t *testing.T) { otherTenant(t, newHarness(t)) })
	t.Run("a search never finds another store's customer", func(t *testing.T) { searchStaysInStore(t, newHarness(t)) })
	t.Run("rows cannot be written into another store", func(t *testing.T) { writesStayInScope(t, newHarness(t)) })
	t.Run("a scope with no user still sees the store", func(t *testing.T) { noUserScope(t, newHarness(t)) })
	t.Run("needs a transaction", func(t *testing.T) { needsTx(t, newHarness(t)) })
}

func insertAndGet(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	home, work := h.address("Casa"), h.address("Trabalho")
	c := h.add(t, tn, "Joao Pereira", home, work)
	got := h.load(t, tn, c.ID())
	if got.ID() != c.ID() || got.TenantID() != tn.id || got.Name() != "Joao Pereira" || got.Phone() != c.Phone() ||
		got.Email().String() != "Pessoa@Example.com" || got.Notes() != "Prefere à tarde." || got.Version() != 1 {
		t.Errorf("read back %+v, want what was inserted at version 1", got)
	}
	if !got.ClientAccountID().IsZero() {
		t.Errorf("client account = %v, want none: a repository never links one", got.ClientAccountID())
	}
	if addrs := got.Addresses(); len(addrs) != 2 || addrs[0] != (customer.Address(home)) || addrs[1] != (customer.Address(work)) {
		t.Errorf("addresses = %+v, want Casa then Trabalho, as given", addrs)
	}
	bare, err := customer.New(tn.id, shared.CustomerID(h.NewID()), customer.Spec{Name: "Sem", Phone: newPhone()})
	if err != nil {
		t.Fatal(err)
	}
	h.in(t, tn.scope(), func(ctx context.Context) error { return h.Customers.Insert(ctx, bare) })
	if got := h.load(t, tn, bare.ID()); !got.Email().IsZero() || len(got.Addresses()) != 0 || got.Notes() != "" {
		t.Errorf("a bare customer read back as %+v, want no email, notes or addresses", got)
	}
}

func getUnknown(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	h.in(t, tn.scope(), func(ctx context.Context) error {
		_, err := h.Customers.ByID(ctx, tn.id, shared.CustomerID(h.NewID()))
		if !errors.Is(err, customer.ErrNotFound) || !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("an unknown id: %v, want customer.ErrNotFound", err)
		}
		return nil
	})
}

func byPhone(t *testing.T, h Harness) {
	a, b := h.newTenant(t), h.newTenant(t)
	c := h.add(t, a, "Ana", h.address("Casa"))
	h.in(t, a.scope(), func(ctx context.Context) error {
		got, err := h.Customers.ByPhone(ctx, a.id, c.Phone())
		if err != nil || got.ID() != c.ID() || len(got.Addresses()) != 1 {
			t.Errorf("ByPhone = %+v, %v; want the customer with its address", got, err)
		}
		if _, err := h.Customers.ByPhone(ctx, a.id, newPhone()); !errors.Is(err, customer.ErrNotFound) {
			t.Errorf("an unknown number: %v, want ErrNotFound", err)
		}
		if _, err := h.Customers.ByPhone(ctx, b.id, c.Phone()); !errors.Is(err, customer.ErrNotFound) {
			t.Errorf("the number in another store's scope: %v, want ErrNotFound", err)
		}
		return nil
	})
	h.in(t, b.scope(), func(ctx context.Context) error {
		if _, err := h.Customers.ByPhone(ctx, b.id, c.Phone()); !errors.Is(err, customer.ErrNotFound) {
			t.Errorf("the number as store B: %v, want ErrNotFound", err)
		}
		return nil
	})
}

func updateOptimistic(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	c := h.add(t, tn, "Ana")
	cur, stale := h.load(t, tn, c.ID()), h.load(t, tn, c.ID())
	newPhoneNumber := newPhone()
	err := h.replace(tn, cur, customer.Spec{Name: "Ana Maria", Phone: newPhoneNumber, Email: "", Notes: "", Addresses: nil})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	got := h.load(t, tn, c.ID())
	if got.Name() != "Ana Maria" || got.Phone() != newPhoneNumber || !got.Email().IsZero() || got.Notes() != "" || got.Version() != 2 {
		t.Errorf("after the update: %+v; want the new values, emptied fields cleared, at version 2", got)
	}
	// stale was read at version 1: the domain accepts it, the repository must not.
	if err := h.replace(tn, stale, specOf(stale)); !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("a stale update: %v, want ErrVersionConflict", err)
	}
	if got := h.load(t, tn, c.ID()); got.Name() != "Ana Maria" || got.Version() != 2 {
		t.Errorf("after the stale update: %+v; want it unchanged", got)
	}
}

func updateUnknown(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	ghost := h.build(t, tn, "Fantasma", newPhone())
	err := h.try(tn.scope(), func(ctx context.Context) error { return h.Customers.Update(ctx, ghost) })
	if !errors.Is(err, customer.ErrNotFound) {
		t.Errorf("updating a customer that was never inserted: %v, want ErrNotFound", err)
	}
}

func insertDuplicate(t *testing.T, h Harness) {
	a, b := h.newTenant(t), h.newTenant(t)
	c := h.add(t, a, "Alfa")
	again, err := customer.New(a.id, c.ID(), customer.Spec{Name: "Outra", Phone: newPhone()})
	if err != nil {
		t.Fatal(err)
	}
	err = h.try(a.scope(), func(ctx context.Context) error { return h.Customers.Insert(ctx, again) })
	if !errors.Is(err, shared.ErrConflict) || errors.Is(err, appcustomer.ErrPhoneTaken) {
		t.Errorf("the same id again: %v, want a conflict that is not ErrPhoneTaken", err)
	}
	// The key sees every store's rows: another store reusing the id is the
	// same conflict, never a leak of whose it is.
	foreign, err := customer.New(b.id, c.ID(), customer.Spec{Name: "Alheia", Phone: newPhone()})
	if err != nil {
		t.Fatal(err)
	}
	err = h.try(b.scope(), func(ctx context.Context) error { return h.Customers.Insert(ctx, foreign) })
	if !errors.Is(err, shared.ErrConflict) {
		t.Errorf("another store with the same id: %v, want a conflict", err)
	}
	if got := h.load(t, a, c.ID()); got.Name() != "Alfa" {
		t.Errorf("the original after the refusals: %+v; want it untouched", got)
	}
}

func phoneTaken(t *testing.T, h Harness) {
	a, b := h.newTenant(t), h.newTenant(t)
	first := h.add(t, a, "Ana")
	second := h.add(t, a, "Bia")

	dup := h.build(t, a, "Outra Ana", first.Phone())
	err := h.try(a.scope(), func(ctx context.Context) error { return h.Customers.Insert(ctx, dup) })
	if !errors.Is(err, appcustomer.ErrPhoneTaken) || !errors.Is(err, shared.ErrConflict) {
		t.Errorf("insert with a number in use: %v, want ErrPhoneTaken", err)
	}
	cur := h.load(t, a, second.ID())
	err = h.replace(a, cur, customer.Spec{Name: "Bia", Phone: first.Phone()})
	if !errors.Is(err, appcustomer.ErrPhoneTaken) {
		t.Errorf("update to a number in use: %v, want ErrPhoneTaken", err)
	}
	if got := h.load(t, a, second.ID()); got.Phone() != second.Phone() || got.Version() != 1 {
		t.Errorf("after the refusal: %+v; want it unchanged", got)
	}
	same := h.load(t, a, first.ID()) // keeping its own number is no conflict
	if err := h.replace(a, same, specOf(same)); err != nil {
		t.Errorf("update keeping its own number: %v", err)
	}
	other := h.build(t, b, "Ana", first.Phone()) // the same number in another store is another person
	if err := h.try(b.scope(), func(ctx context.Context) error { return h.Customers.Insert(ctx, other) }); err != nil {
		t.Errorf("the same number in another store: %v, want it allowed", err)
	}
}

func addressesAsSet(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	home, work, farm := h.address("Casa"), h.address("Trabalho"), h.address("Sitio")
	c := h.add(t, tn, "Ana", home, work, farm)

	edited := work
	edited.Label, edited.City = "Escritorio", "Belem"
	added := h.address("Praia")
	cur := h.load(t, tn, c.ID())
	// Home is dropped, work is edited and moves first, the farm stays, and a new one joins.
	if err := h.replace(tn, cur, specOf(cur, edited, customer.AddressSpec(mustAddress(t, cur, farm.ID)), added)); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got := h.load(t, tn, c.ID())
	if want := []string{"Escritorio", "Sitio", "Praia"}; !slices.Equal(labels(got), want) {
		t.Fatalf("addresses = %v, want %v, in the order given", labels(got), want)
	}
	if a, ok := got.Address(work.ID); !ok || a.City != "Belem" {
		t.Errorf("the edited address = %+v, %v; want it kept by id with its new city", a, ok)
	}
	if _, ok := got.Address(home.ID); ok {
		t.Error("the address left out of the update is still there")
	}

	cleared := h.load(t, tn, c.ID())
	if err := h.replace(tn, cleared, specOf(cleared)); err != nil {
		t.Fatalf("Update with no addresses: %v", err)
	}
	if got := h.load(t, tn, c.ID()); len(got.Addresses()) != 0 || got.Version() != 3 {
		t.Errorf("after clearing: %+v; want no addresses at version 3", got)
	}
	// A removed address's id can be used again, by the same customer.
	back := h.load(t, tn, c.ID())
	if err := h.replace(tn, back, specOf(back, home)); err != nil {
		t.Errorf("reusing a removed address id: %v", err)
	}
}

func mustAddress(t *testing.T, c *customer.Customer, id shared.AddressID) customer.Address {
	t.Helper()
	a, ok := c.Address(id)
	if !ok {
		t.Fatalf("customer has no address %s", id)
	}
	return a
}

func addressOfAnother(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	theirs := h.address("Deles")
	owner := h.add(t, tn, "Ana", theirs)
	cur := h.add(t, tn, "Bia", h.address("Casa"))

	intruder := h.build(t, tn, "Carla", newPhone(), theirs)
	err := h.try(tn.scope(), func(ctx context.Context) error { return h.Customers.Insert(ctx, intruder) })
	if !errors.Is(err, shared.ErrConflict) {
		t.Errorf("insert with an address id in use: %v, want a conflict", err)
	}
	loaded := h.load(t, tn, cur.ID())
	err = h.replace(tn, loaded, specOf(loaded, theirs))
	if !errors.Is(err, shared.ErrInvalid) {
		t.Errorf("update with another customer's address id: %v, want ErrInvalid", err)
	}
	if got := h.load(t, tn, owner.ID()); len(got.Addresses()) != 1 || got.Addresses()[0] != customer.Address(theirs) {
		t.Errorf("the owner's address after the attempts: %+v; want it untouched", got.Addresses())
	}
	if got := h.load(t, tn, cur.ID()); labels(got)[0] != "Casa" || got.Version() != 1 {
		t.Errorf("the updated customer after the refusal: %+v; want it unchanged", got)
	}
}

var errRollback = errors.New("customerports: the transaction fails on purpose")

func rollbackInsert(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	c := h.build(t, tn, "Ana", newPhone(), h.address("Casa"))
	err := h.try(tn.scope(), func(ctx context.Context) error {
		if err := h.Customers.Insert(ctx, c); err != nil {
			return err
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("the transaction = %v, want the error it returned", err)
	}
	h.in(t, tn.scope(), func(ctx context.Context) error {
		if _, err := h.Customers.ByID(ctx, tn.id, c.ID()); !errors.Is(err, customer.ErrNotFound) {
			t.Errorf("after the rollback: %v, want ErrNotFound", err)
		}
		return nil
	})
}

func rollbackUpdate(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	home := h.address("Casa")
	c := h.add(t, tn, "Ana", home)
	err := h.try(tn.scope(), func(ctx context.Context) error {
		cur, err := h.Customers.ByID(ctx, tn.id, c.ID())
		if err != nil {
			return err
		}
		if err := cur.Replace(customer.Spec{Name: "Renomeada", Phone: newPhone(), Addresses: []customer.AddressSpec{h.address("Nova")}}, 1); err != nil {
			return err
		}
		if err := h.Customers.Update(ctx, cur); err != nil {
			return err
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("the transaction = %v, want the error it returned", err)
	}
	got := h.load(t, tn, c.ID())
	if got.Name() != "Ana" || got.Phone() != c.Phone() || got.Version() != 1 || !slices.Equal(labels(got), []string{"Casa"}) {
		t.Errorf("after the rollback: %+v; want the old values, address and version", got)
	}
}

func listPages(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	for _, name := range []string{"delta", "Alfa", "Charlie", "bravo", "Echo"} {
		h.add(t, tn, name)
	}
	twinA := h.add(t, tn, "Foxtrot")
	twinB := h.add(t, tn, "foxtrot")
	h.in(t, tn.scope(), func(ctx context.Context) error {
		all := h.walk(ctx, t, tn, 3)
		var got []string
		var twins []shared.CustomerID
		for _, s := range all {
			got = append(got, s.Name)
			if s.Name == "Foxtrot" || s.Name == "foxtrot" {
				twins = append(twins, s.ID)
			}
		}
		want := []string{"Alfa", "bravo", "Charlie", "delta", "Echo"}
		if !slices.Equal(got[:5], want) || len(got) != 7 {
			t.Errorf("names across pages = %v, want %v then the two Foxtrots (case ignored)", got, want)
		}
		wantTwins := []shared.CustomerID{twinA.ID(), twinB.ID()}
		slices.SortFunc(wantTwins, func(a, b shared.CustomerID) int { return slices.Compare(a[:], b[:]) })
		if !slices.Equal(twins, wantTwins) {
			t.Errorf("equal names = %v, want them ordered by id %v", twins, wantTwins)
		}
		return nil
	})
}

// walk lists every customer of tn page by page, limit at a time, following
// the cursors, and fails when a page is too long or the cursors never end.
func (h Harness) walk(ctx context.Context, t *testing.T, tn tenant, limit int) []appcustomer.Summary {
	t.Helper()
	var all []appcustomer.Summary
	cursor := ""
	for range 20 {
		p, err := h.Customers.List(ctx, tn.id, appcustomer.ListFilter{}, page(t, cursor, limit))
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(p.Items) > limit {
			t.Errorf("a page of %d, want at most %d", len(p.Items), limit)
		}
		all = append(all, p.Items...)
		if !p.HasMore() {
			return all
		}
		cursor = p.NextCursor
	}
	t.Fatal("the cursors never ended")
	return nil
}

func listExactBoundary(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	for _, name := range []string{"Alfa", "Bravo", "Charlie", "Delta"} {
		h.add(t, tn, name)
	}
	h.in(t, tn.scope(), func(ctx context.Context) error {
		first, err := h.Customers.List(ctx, tn.id, appcustomer.ListFilter{}, page(t, "", 2))
		if err != nil || !first.HasMore() {
			t.Fatalf("first page = %v, %v; want it to have more", names(first), err)
		}
		last, err := h.Customers.List(ctx, tn.id, appcustomer.ListFilter{}, page(t, first.NextCursor, 2))
		if err != nil || !slices.Equal(names(last), []string{"Charlie", "Delta"}) || last.NextCursor != "" {
			t.Errorf("second page = %v, cursor %q, %v; want Charlie and Delta and no cursor", names(last), last.NextCursor, err)
		}
		whole, err := h.Customers.List(ctx, tn.id, appcustomer.ListFilter{}, page(t, "", 4))
		if err != nil || len(whole.Items) != 4 || whole.NextCursor != "" {
			t.Errorf("a page of exactly all rows = %v, cursor %q, %v; want no cursor", names(whole), whole.NextCursor, err)
		}
		return nil
	})
}

func listFilter(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	ana := h.build(t, tn, "Ana", "5592991112222")
	for _, c := range []*customer.Customer{
		ana, h.build(t, tn, "anabela", "5511993334444"), h.build(t, tn, "Joana", "5592995556666"),
	} {
		h.in(t, tn.scope(), func(ctx context.Context) error { return h.Customers.Insert(ctx, c) })
	}
	cases := []struct {
		name, query string
		want        []string
	}{
		{"all", "", []string{"Ana", "anabela", "Joana"}},
		{"a prefix, ignoring case", "ANA", []string{"Ana", "anabela"}},
		{"a prefix is not a substring", "joa", []string{"Joana"}},
		{"the middle of a name does not match", "bela", []string{}},
		{"digits of the number", "99111", []string{"Ana"}},
		{"digits typed as a number", "(92) 99111-22", []string{"Ana"}},
		{"digits of several numbers", "5592", []string{"Ana", "Joana"}},
		{"a name with a digit is not a number", "ana 9", []string{}},
		{"no match", "zzz", []string{}},
	}
	h.in(t, tn.scope(), func(ctx context.Context) error {
		for _, c := range cases {
			p, err := h.Customers.List(ctx, tn.id, appcustomer.ListFilter{Query: c.query}, page(t, "", 10))
			if err != nil || !slices.Equal(names(p), c.want) {
				t.Errorf("%s (%q) = %v, %v; want %v", c.name, c.query, names(p), err, c.want)
			}
		}
		return nil
	})
}

func listFilterLiteral(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	for _, name := range []string{"Alfa", "AxB", "A_B", "100% Couro"} {
		h.add(t, tn, name)
	}
	cases := []struct {
		query string
		want  []string
	}{
		{"%", []string{}},
		{"100%", []string{"100% Couro"}},
		{"_", []string{}},
		{"A_", []string{"A_B"}},
		{"a%", []string{}},
		{"A_B", []string{"A_B"}},
	}
	h.in(t, tn.scope(), func(ctx context.Context) error {
		for _, c := range cases {
			p, err := h.Customers.List(ctx, tn.id, appcustomer.ListFilter{Query: c.query}, page(t, "", 10))
			if err != nil || !slices.Equal(names(p), c.want) {
				t.Errorf("filter %q = %v, %v; want %v", c.query, names(p), err, c.want)
			}
		}
		return nil
	})
}

func listCursorSurvives(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	alfa := h.add(t, tn, "Alfa")
	h.add(t, tn, "Bravo")
	h.add(t, tn, "Charlie")
	var cursor string
	h.in(t, tn.scope(), func(ctx context.Context) error {
		first, err := h.Customers.List(ctx, tn.id, appcustomer.ListFilter{}, page(t, "", 1))
		if err != nil || !slices.Equal(names(first), []string{"Alfa"}) || !first.HasMore() {
			t.Fatalf("first page = %v, %v", names(first), err)
		}
		cursor = first.NextCursor
		return nil
	})
	cur := h.load(t, tn, alfa.ID()) // the row the cursor names is renamed after the page was read
	if err := h.replace(tn, cur, customer.Spec{Name: "Alfb", Phone: cur.Phone()}); err != nil {
		t.Fatal(err)
	}
	h.in(t, tn.scope(), func(ctx context.Context) error {
		p, err := h.Customers.List(ctx, tn.id, appcustomer.ListFilter{}, page(t, cursor, 10))
		if err != nil || !slices.Equal(names(p), []string{"Bravo", "Charlie"}) {
			t.Errorf("after a rename = %v, %v; want the rows after the renamed row", names(p), err)
		}
		f := appcustomer.ListFilter{Query: "charlie"}
		p, err = h.Customers.List(ctx, tn.id, f, page(t, cursor, 10))
		if err != nil || !slices.Equal(names(p), []string{"Charlie"}) {
			t.Errorf("with a cursor and a filter = %v, %v; want Charlie", names(p), err)
		}
		return nil
	})
}

func listBadCursor(t *testing.T, h Harness) {
	a, b := h.newTenant(t), h.newTenant(t)
	for _, name := range []string{"Alfa", "Bravo", "Charlie"} {
		h.add(t, a, name)
	}
	h.add(t, b, "Delta")
	h.add(t, b, "Eco")
	real := h.firstCursor(t, a)
	first := "A" // a real cursor changed in its first character names another id
	if real[:1] == first {
		first = "B"
	}
	cursors := map[string]string{
		"gibberish":              "not-a-cursor",
		"a truncated cursor":     real[:len(real)-4],
		"a tampered cursor":      first + real[1:],
		"another store's cursor": h.firstCursor(t, b),
	}
	h.in(t, a.scope(), func(ctx context.Context) error {
		for name, c := range cursors {
			if _, err := h.Customers.List(ctx, a.id, appcustomer.ListFilter{}, page(t, c, 10)); !errors.Is(err, shared.ErrInvalid) {
				t.Errorf("%s: %v, want ErrInvalid", name, err)
			}
		}
		return nil
	})
}

// firstCursor returns the cursor after the first customer of tn.
func (h Harness) firstCursor(t *testing.T, tn tenant) string {
	t.Helper()
	var cursor string
	h.in(t, tn.scope(), func(ctx context.Context) error {
		p, err := h.Customers.List(ctx, tn.id, appcustomer.ListFilter{}, page(t, "", 1))
		if err != nil || !p.HasMore() {
			t.Fatalf("first page = %v, %v; want a cursor", names(p), err)
		}
		cursor = p.NextCursor
		return nil
	})
	return cursor
}

func listOrders(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	busy := h.add(t, tn, "Ana")
	h.add(t, tn, "Bia")
	other := h.add(t, tn, "Carla")
	oldest := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	newest := time.Date(2026, 11, 20, 15, 30, 0, 123456000, time.UTC)
	h.AddOrder(t, tn.id, busy.ID(), oldest)
	h.AddOrder(t, tn.id, busy.ID(), newest)
	h.AddOrder(t, tn.id, busy.ID(), oldest.Add(24*time.Hour))
	h.AddOrder(t, tn.id, other.ID(), oldest)
	h.in(t, tn.scope(), func(ctx context.Context) error {
		p, err := h.Customers.List(ctx, tn.id, appcustomer.ListFilter{}, page(t, "", 10))
		if err != nil || len(p.Items) != 3 {
			t.Fatalf("List = %v, %v", names(p), err)
		}
		ana, bia, carla := p.Items[0], p.Items[1], p.Items[2]
		if ana.Orders != 3 || ana.LastOrderAt == nil || !ana.LastOrderAt.Equal(newest) {
			t.Errorf("Ana = %+v, want 3 orders, the newest at %v", ana, newest)
		}
		if bia.Orders != 0 || bia.LastOrderAt != nil {
			t.Errorf("Bia = %+v, want no orders and no date", bia)
		}
		if carla.Orders != 1 || carla.LastOrderAt == nil || !carla.LastOrderAt.Equal(oldest) {
			t.Errorf("Carla = %+v, want 1 order", carla)
		}
		if ana.Phone != busy.Phone() || ana.ID != busy.ID() {
			t.Errorf("Ana = %+v, want her id and number", ana)
		}
		return nil
	})
}

func otherTenant(t *testing.T, h Harness) {
	a, b := h.newTenant(t), h.newTenant(t)
	mine := h.add(t, a, "Alfa")
	theirs := h.add(t, b, "Bravo", h.address("Casa"))
	h.AddOrder(t, b.id, theirs.ID(), time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC))
	h.in(t, a.scope(), func(ctx context.Context) error {
		if _, err := h.Customers.ByID(ctx, a.id, theirs.ID()); !errors.Is(err, customer.ErrNotFound) {
			t.Errorf("reading B's customer as A: %v, want ErrNotFound", err)
		}
		if _, err := h.Customers.ByID(ctx, b.id, theirs.ID()); !errors.Is(err, customer.ErrNotFound) {
			t.Errorf("reading B's customer as A, naming B: %v, want ErrNotFound", err)
		}
		p, err := h.Customers.List(ctx, a.id, appcustomer.ListFilter{}, page(t, "", 10))
		if err != nil || !slices.Equal(names(p), []string{"Alfa"}) || p.Items[0].ID != mine.ID() {
			t.Errorf("A's list = %v, %v; want only Alfa", names(p), err)
		}
		p, err = h.Customers.List(ctx, b.id, appcustomer.ListFilter{}, page(t, "", 10))
		if err != nil || len(p.Items) != 0 {
			t.Errorf("B's list in A's scope = %v, %v; want none", names(p), err)
		}
		other, err := customer.Restore(customer.Record{Tenant: b.id, ID: theirs.ID(), Name: "Invadido", Phone: theirs.Phone(), Version: 1})
		if err != nil {
			return err
		}
		if err := h.Customers.Update(ctx, other); !errors.Is(err, customer.ErrNotFound) {
			t.Errorf("A updating B's customer: %v, want ErrNotFound", err)
		}
		return nil
	})
	got := h.load(t, b, theirs.ID())
	if got.Name() != "Bravo" || got.Version() != 1 || len(got.Addresses()) != 1 {
		t.Errorf("B's customer after A's attempts: %+v; want it untouched", got)
	}
}

// searchStaysInStore: a name and a number that exist only in store A find
// nothing when store B searches for them, whether B's scope names B's own
// store or A's. The control search in A proves the customer is findable, so
// the empty answers are not an empty fixture.
func searchStaysInStore(t *testing.T, h Harness) {
	a, b := h.newTenant(t), h.newTenant(t)
	h.add(t, b, "Bruno")
	h.in(t, a.scope(), func(ctx context.Context) error {
		return h.Customers.Insert(ctx, h.build(t, a, "Zelda Unica", "5592987654321"))
	})
	queries := []struct{ name, query string }{
		{"a name", "Zelda"},
		{"the name in capitals", "ZELDA UNICA"},
		{"the digits of the number", "987654321"},
		{"the number typed as people do", "(92) 98765-4321"},
	}
	h.in(t, a.scope(), func(ctx context.Context) error {
		for _, q := range queries {
			p, err := h.Customers.List(ctx, a.id, appcustomer.ListFilter{Query: q.query}, page(t, "", 10))
			if err != nil || !slices.Equal(names(p), []string{"Zelda Unica"}) {
				t.Errorf("A searching %s = %v, %v; want its own customer", q.name, names(p), err)
			}
		}
		return nil
	})
	h.in(t, b.scope(), func(ctx context.Context) error {
		for _, q := range queries {
			for _, store := range []tenant{a, b} {
				p, err := h.Customers.List(ctx, store.id, appcustomer.ListFilter{Query: q.query}, page(t, "", 10))
				if err != nil || len(p.Items) != 0 {
					t.Errorf("B searching %s (as %v) = %v, %v; want nothing", q.name, store.id, names(p), err)
				}
			}
		}
		return nil
	})
}

func writesStayInScope(t *testing.T, h Harness) {
	a, b := h.newTenant(t), h.newTenant(t)
	intruder := h.build(t, b, "Intruso", newPhone(), h.address("Casa"))
	if err := h.try(a.scope(), func(ctx context.Context) error { return h.Customers.Insert(ctx, intruder) }); err == nil {
		t.Error("store A inserted a customer of store B")
	}
	h.in(t, b.scope(), func(ctx context.Context) error {
		p, err := h.Customers.List(ctx, b.id, appcustomer.ListFilter{}, page(t, "", 10))
		if err != nil || len(p.Items) != 0 {
			t.Errorf("B's list = %v, %v; want the refused insert absent", names(p), err)
		}
		return nil
	})
}

// noUserScope: customers are tenant data, not staff-only data. A scope with
// a tenant and no user (a store page request, milestone 7) still reads and
// writes them, which the order created on the store page needs.
func noUserScope(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	c := h.add(t, tn, "Alfa", h.address("Casa"))
	noUser := appshared.Scope{TenantID: tn.id}
	h.in(t, noUser, func(ctx context.Context) error {
		got, err := h.Customers.ByID(ctx, tn.id, c.ID())
		if err != nil || len(got.Addresses()) != 1 {
			t.Errorf("ByID with no user: %+v, %v; want the customer and its address", got, err)
		}
		p, err := h.Customers.List(ctx, tn.id, appcustomer.ListFilter{}, page(t, "", 10))
		if err != nil || len(p.Items) != 1 {
			t.Errorf("List with no user = %v, %v; want the customer", names(p), err)
		}
		return nil
	})
}

func needsTx(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	_, err := h.Customers.ByID(context.Background(), tn.id, shared.CustomerID(h.NewID()))
	if !errors.Is(err, appshared.ErrNoTransaction) {
		t.Errorf("outside a transaction: %v, want ErrNoTransaction", err)
	}
	_, err = h.Customers.List(context.Background(), tn.id, appcustomer.ListFilter{}, page(t, "", 1))
	if !errors.Is(err, appshared.ErrNoTransaction) {
		t.Errorf("a list outside a transaction: %v, want ErrNoTransaction", err)
	}
}
