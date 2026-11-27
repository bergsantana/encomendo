//go:build integration

package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres"
	appcustomer "github.com/bergsantana/encomendo/backend/internal/app/customer"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/customer"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/test/contract/customerports"
	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

// seedOrder inserts, as the superuser, an order of the customer created at
// at, on the default workflow, cancelled when cancelled is set. Order
// numbers count up per store.
func seedOrder(t testing.TB, db *sql.DB, tenant shared.TenantID, id shared.CustomerID, at time.Time, cancelled bool) {
	t.Helper()
	_, err := db.ExecContext(t.Context(), `INSERT INTO orders (tenant_id, id, number, customer_id, workflow_id,
			workflow_version, status_key, delivery_date, assembly_days, safety_margin_days, deposit_percent, currency,
			subtotal_cents, handover, tracking_token_hash, created_at, cancelled_at, cancel_reason)
		SELECT $1::uuid, gen_random_uuid(), (SELECT coalesce(max(number), 0) + 1 FROM orders WHERE tenant_id = $1::uuid),
			$2::uuid, w.id, w.version,
			(SELECT key FROM workflow_statuses s WHERE s.workflow_id = w.id AND s.workflow_version = w.version
				AND s.position = 1),
			current_date + 20, 3, 2, 50, 'BRL', 100000, 'pickup', $3::bytea, $4::timestamptz,
			CASE WHEN $5::boolean THEN $4::timestamptz END, CASE WHEN $5::boolean THEN 'Desistiu' END
		FROM status_workflows w WHERE w.is_default`,
		tenant.String(), id.String(), pgtest.RandomHash(), at, cancelled)
	if err != nil {
		t.Fatalf("seed order: %v", err)
	}
}

// The customer port contract, the same suite the fake passes in
// test/contract. Tenants are seeded as the superuser, so the suite runs as
// encomendo_app against real rows and real RLS.
func TestCustomerRepository_Contract(t *testing.T) {
	tx := postgres.NewTxManager(openApp(t))
	s := env.Seeder(t)
	customerports.RunRepositoryContract(t, func(*testing.T) customerports.Harness {
		return customerports.Harness{
			Customers: postgres.NewCustomers(),
			Tx:        tx,
			NewID:     pgtest.NewID,
			NewTenant: func(*testing.T) shared.TenantID { return s.Tenant(s.User()) },
			AddOrder: func(t *testing.T, tenant shared.TenantID, id shared.CustomerID, at time.Time) {
				seedOrder(t, s.DB(), tenant, id, at, false)
			},
		}
	})
}

type customerEnv struct {
	t      *testing.T
	seed   *pgtest.Seeder
	tx     appshared.TxManager
	tenant shared.TenantID
	scope  appshared.Scope
	repo   *postgres.Customers
	n      int
}

func newCustomerEnv(t *testing.T) *customerEnv {
	t.Helper()
	s := env.Seeder(t)
	tenant := s.Tenant(s.User())
	return &customerEnv{
		t: t, seed: s, tx: postgres.NewTxManager(openApp(t)), tenant: tenant,
		scope: appshared.Scope{TenantID: tenant, UserID: shared.UserID(pgtest.NewID())}, repo: postgres.NewCustomers(),
	}
}

func (e *customerEnv) run(fn func(ctx context.Context) error) error {
	return e.tx.WithinTx(appshared.WithScope(context.Background(), e.scope), fn)
}

func (e *customerEnv) must(fn func(ctx context.Context) error) {
	e.t.Helper()
	if err := e.run(fn); err != nil {
		e.t.Fatal(err)
	}
}

// add stores a customer with a number nobody else in the test has.
func (e *customerEnv) add(name string, addresses ...customer.AddressSpec) *customer.Customer {
	e.t.Helper()
	e.n++
	c, err := customer.New(e.tenant, shared.CustomerID(pgtest.NewID()), customer.Spec{
		Name: name, Phone: fmt.Sprintf("559299%07d", e.n), Addresses: addresses,
	})
	if err != nil {
		e.t.Fatal(err)
	}
	e.must(func(ctx context.Context) error { return e.repo.Insert(ctx, c) })
	return c
}

func addressSpec(label string) customer.AddressSpec {
	return customer.AddressSpec{
		ID: shared.AddressID(pgtest.NewID()), Label: label, CEP: "69000000", Street: "Rua das Flores",
		Number: "10", City: "Manaus", State: "AM",
	}
}

func TestCustomersRepository_PhoneConflictNamesTheUniqueKey(t *testing.T) {
	e := newCustomerEnv(t)
	first := e.add("Ana")
	dup, err := customer.New(e.tenant, shared.CustomerID(pgtest.NewID()), customer.Spec{Name: "Outra", Phone: first.Phone()})
	if err != nil {
		t.Fatal(err)
	}
	err = e.run(func(ctx context.Context) error { return e.repo.Insert(ctx, dup) })
	var pgErr *pgconn.PgError
	if !errors.Is(err, appcustomer.ErrPhoneTaken) || !errors.As(err, &pgErr) || pgErr.ConstraintName != "customers_tenant_id_whatsapp_phone_key" {
		t.Errorf("duplicate number: %v, want ErrPhoneTaken wrapping the unique key violation", err)
	}
	// A taken id is a plain conflict: its key is the primary key, not the number's.
	sameID, err := customer.New(e.tenant, first.ID(), customer.Spec{Name: "Outra", Phone: "5592988880000"})
	if err != nil {
		t.Fatal(err)
	}
	err = e.run(func(ctx context.Context) error { return e.repo.Insert(ctx, sameID) })
	if !errors.Is(err, shared.ErrConflict) || errors.Is(err, appcustomer.ErrPhoneTaken) {
		t.Errorf("duplicate id: %v, want a conflict that is not ErrPhoneTaken", err)
	}
}

func TestCustomersRepository_AddressesAreUpsertedNotRecreated(t *testing.T) {
	e := newCustomerEnv(t)
	home, work := addressSpec("Casa"), addressSpec("Trabalho")
	c := e.add("Ana", home, work)
	type row struct {
		position            int
		createdAt, updateAt time.Time
	}
	read := func() map[string]row {
		rows, err := e.seed.DB().QueryContext(t.Context(),
			`SELECT id, position, created_at, updated_at FROM customer_addresses WHERE customer_id = $1`, c.ID().String())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = rows.Close() }()
		out := map[string]row{}
		for rows.Next() {
			var id string
			var r row
			if err := rows.Scan(&id, &r.position, &r.createdAt, &r.updateAt); err != nil {
				t.Fatal(err)
			}
			out[id] = r
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return out
	}
	before := read()

	var cur *customer.Customer
	e.must(func(ctx context.Context) error {
		var err error
		cur, err = e.repo.ByID(ctx, e.tenant, c.ID())
		return err
	})
	work.Street = "Rua Nova"
	fresh := addressSpec("Praia")
	err := cur.Replace(customer.Spec{Name: "Ana", Phone: c.Phone(), Addresses: []customer.AddressSpec{work, fresh}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	e.must(func(ctx context.Context) error { return e.repo.Update(ctx, cur) })

	after := read()
	if len(after) != 2 || after[home.ID.String()].position != 0 {
		t.Fatalf("rows after = %v, want work and the new address only", after)
	}
	kept := after[work.ID.String()]
	if kept.position != 1 || !kept.createdAt.Equal(before[work.ID.String()].createdAt) || !kept.updateAt.After(before[work.ID.String()].updateAt) {
		t.Errorf("work = %+v, before %+v; want the same row, now first, with a later updated_at", kept, before[work.ID.String()])
	}
	if after[fresh.ID.String()].position != 2 {
		t.Errorf("new address = %+v, want position 2", after[fresh.ID.String()])
	}
}

func TestCustomersRepository_NeverWritesTheClientAccountLink(t *testing.T) {
	e := newCustomerEnv(t)
	c := e.add("Ana")
	account := e.seed.ClientAccount()
	if _, err := e.seed.DB().ExecContext(t.Context(),
		`UPDATE customers SET client_account_id = $1 WHERE id = $2`, account.String(), c.ID().String()); err != nil {
		t.Fatal(err)
	}
	var got *customer.Customer
	e.must(func(ctx context.Context) error {
		var err error
		got, err = e.repo.ByID(ctx, e.tenant, c.ID())
		return err
	})
	if got.ClientAccountID() != account {
		t.Fatalf("client account = %v, want the linked %v", got.ClientAccountID(), account)
	}
	if err := got.Replace(customer.Spec{Name: "Ana Maria", Phone: c.Phone()}, 1); err != nil {
		t.Fatal(err)
	}
	e.must(func(ctx context.Context) error { return e.repo.Update(ctx, got) })
	e.must(func(ctx context.Context) error {
		var err error
		got, err = e.repo.ByID(ctx, e.tenant, c.ID())
		return err
	})
	if got.Name() != "Ana Maria" || got.ClientAccountID() != account {
		t.Errorf("after the update: %v, account %v; want the new name and the link untouched", got.Name(), got.ClientAccountID())
	}
}

func TestCustomersRepository_ListCountsEveryOrder(t *testing.T) {
	e := newCustomerEnv(t)
	other := newCustomerEnv(t)
	ana, bia := e.add("Ana"), e.add("Bia")
	stranger := other.add("Ana")
	at := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	seedOrder(t, e.seed.DB(), e.tenant, ana.ID(), at, false)
	seedOrder(t, e.seed.DB(), e.tenant, ana.ID(), at.Add(48*time.Hour), true) // cancelled, still counted
	seedOrder(t, e.seed.DB(), e.tenant, ana.ID(), at.Add(24*time.Hour), false)
	seedOrder(t, other.seed.DB(), other.tenant, stranger.ID(), at.Add(500*time.Hour), false)

	e.must(func(ctx context.Context) error {
		p, err := e.repo.List(ctx, e.tenant, appcustomer.ListFilter{}, mustPage(t, "", 10))
		if err != nil || len(p.Items) != 2 {
			t.Fatalf("List = %+v, %v", p, err)
		}
		a, b := p.Items[0], p.Items[1]
		if a.ID != ana.ID() || a.Orders != 3 || a.LastOrderAt == nil || !a.LastOrderAt.Equal(at.Add(48*time.Hour)) {
			t.Errorf("Ana = %+v, want 3 orders (the cancelled one too), the newest 48h in", a)
		}
		if b.ID != bia.ID() || b.Orders != 0 || b.LastOrderAt != nil {
			t.Errorf("Bia = %+v, want no orders and no date", b)
		}
		return nil
	})
}

func TestCustomersRepository_KeysetPagesByLowerName(t *testing.T) {
	e := newCustomerEnv(t)
	var want []string
	for i := range 23 {
		name := fmt.Sprintf("%c%c", 'a'+i%26, 'a'+i/26)
		if i%2 == 0 {
			name = fmt.Sprintf("%c%c", 'A'+i%26, 'a'+i/26) // capitals sort with the lower case: by lower(name)
		}
		e.add(name)
		want = append(want, name)
	}
	slices.SortFunc(want, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	var got []string
	cursor := ""
	pages := 0
	e.must(func(ctx context.Context) error {
		for {
			p, err := e.repo.List(ctx, e.tenant, appcustomer.ListFilter{}, mustPage(t, cursor, 7))
			if err != nil {
				return err
			}
			pages++
			for _, s := range p.Items {
				got = append(got, s.Name)
			}
			if !p.HasMore() {
				return nil
			}
			cursor = p.NextCursor
		}
	})
	if pages != 4 || !slices.Equal(got, want) {
		t.Errorf("%d pages, names %v; want 4 pages of 7, 7, 7 and 2 in %v", pages, got, want)
	}
}

func TestCustomersRepository_RLSHidesAnotherStoresCustomer(t *testing.T) {
	a, b := newCustomerEnv(t), newCustomerEnv(t)
	theirs := b.add("Bravo", addressSpec("Casa"))
	a.must(func(ctx context.Context) error {
		if _, err := a.repo.ByID(ctx, a.tenant, theirs.ID()); !errors.Is(err, customer.ErrNotFound) {
			t.Errorf("ByID of B's customer in A's scope: %v, want ErrNotFound", err)
		}
		if _, err := a.repo.ByID(ctx, b.tenant, theirs.ID()); !errors.Is(err, customer.ErrNotFound) {
			t.Errorf("ByID naming B's tenant in A's scope: %v, want ErrNotFound (RLS)", err)
		}
		if _, err := a.repo.ByPhone(ctx, b.tenant, theirs.Phone()); !errors.Is(err, customer.ErrNotFound) {
			t.Errorf("ByPhone naming B's tenant in A's scope: %v, want ErrNotFound (RLS)", err)
		}
		return nil
	})
}
