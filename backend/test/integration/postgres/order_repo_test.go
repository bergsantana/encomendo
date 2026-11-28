//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"encoding/base64"
	hexenc "encoding/hex"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres"
	"github.com/bergsantana/encomendo/backend/internal/adapter/system"
	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
	"github.com/bergsantana/encomendo/backend/internal/platform/config"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

// orderEnv is one store with a staff user and a customer, and the order
// repository on the encomendo_app pool.
type orderEnv struct {
	t        *testing.T
	seed     *pgtest.Seeder
	tx       appshared.TxManager
	repo     *postgres.Orders
	wf       workflow.Workflow
	tenant   shared.TenantID
	user     shared.UserID
	customer shared.CustomerID
}

func newOrderEnv(t *testing.T) *orderEnv {
	t.Helper()
	s := env.Seeder(t)
	tx := postgres.NewTxManager(openApp(t))
	user := s.User()
	tenant := s.Tenant(user)
	return &orderEnv{
		t: t, seed: s, tx: tx, repo: postgres.NewOrders(system.NewIDs()), wf: defaultWorkflow(t, tx),
		tenant: tenant, user: user, customer: seedCustomer(t, s, tenant, "Cliente de teste"),
	}
}

func (e *orderEnv) staff() appshared.Scope {
	return appshared.Scope{TenantID: e.tenant, UserID: e.user}
}

func (e *orderEnv) storePage() appshared.Scope { return appshared.Scope{TenantID: e.tenant} }

func (e *orderEnv) run(scope appshared.Scope, fn func(ctx context.Context) error) error {
	return e.tx.WithinTx(appshared.WithScope(context.Background(), scope), fn)
}

func (e *orderEnv) must(scope appshared.Scope, fn func(ctx context.Context) error) {
	e.t.Helper()
	if err := e.run(scope, fn); err != nil {
		e.t.Fatal(err)
	}
}

// build returns a new order numbered n, of the given customer.
func (e *orderEnv) build(n order.Number, customer shared.CustomerID) *order.Order {
	e.t.Helper()
	spec := testkit.NewOrder().WithItem(testkit.ItemOpts{LeadDays: 5, Cost: ptr(int64(6000))}).Spec(e.t)
	spec.Tenant, spec.Customer, spec.Workflow, spec.Number = e.tenant, customer, e.wf, n
	spec.Tracking = shared.SecretHash(pgtest.RandomHash())
	p, err := shared.NewBackofficePrincipal(e.user, e.tenant, shared.RoleOwner)
	if err != nil {
		e.t.Fatal(err)
	}
	spec.By = p
	o, err := order.New(spec)
	if err != nil {
		e.t.Fatal(err)
	}
	return o
}

func ptr[T any](v T) *T { return &v }

// insert stores a new order with the store's next number and returns it.
func (e *orderEnv) insert() *order.Order {
	e.t.Helper()
	var o *order.Order
	e.must(e.staff(), func(ctx context.Context) error {
		n, err := e.repo.NextNumber(ctx, e.tenant)
		if err != nil {
			return err
		}
		o = e.build(n, e.customer)
		return e.repo.Insert(ctx, o)
	})
	return o
}

func (e *orderEnv) count(query string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.seed.DB().QueryRowContext(e.t.Context(), query, args...).Scan(&n); err != nil {
		e.t.Fatalf("count: %v", err)
	}
	return n
}

func TestOrdersRepository_FailedWriteLeavesNoOutboxRow(t *testing.T) {
	e := newOrderEnv(t)
	events := postgres.NewOutbox()
	publish := func(ctx context.Context, o *order.Order) error {
		return events.Publish(ctx, order.Created{Order: o.ID(), Number: o.Number(), Workflow: e.wf.ID(), Version: e.wf.Version()})
	}
	const outbox = `SELECT count(*) FROM outbox_events WHERE tenant_id = $1`

	t.Run("an order that fails to insert takes its event with it", func(t *testing.T) {
		err := e.run(e.staff(), func(ctx context.Context) error {
			o := e.build(1, shared.CustomerID(pgtest.NewID())) // a customer the store does not have
			if err := publish(ctx, o); err != nil {
				return err
			}
			return e.repo.Insert(ctx, o)
		})
		if !errors.Is(err, shared.ErrInvalid) {
			t.Fatalf("insert err = %v, want shared.ErrInvalid (a foreign key)", err)
		}
		if n := e.count(outbox, e.tenant.String()); n != 0 {
			t.Errorf("outbox has %d events after a failed write, want 0", n)
		}
		if n := e.count(`SELECT count(*) FROM orders WHERE tenant_id = $1`, e.tenant.String()); n != 0 {
			t.Errorf("%d orders stored, want 0", n)
		}
	})
	t.Run("an order and its event commit together", func(t *testing.T) {
		e.must(e.staff(), func(ctx context.Context) error {
			n, err := e.repo.NextNumber(ctx, e.tenant)
			if err != nil {
				return err
			}
			o := e.build(n, e.customer)
			if err := e.repo.Insert(ctx, o); err != nil {
				return err
			}
			return publish(ctx, o)
		})
		if n := e.count(outbox, e.tenant.String()); n != 1 {
			t.Errorf("outbox has %d events, want 1", n)
		}
	})
}

func TestOrdersRepository_ScopesWithoutStaffReadNoSourcing(t *testing.T) {
	e := newOrderEnv(t)
	o := e.insert()
	account, other := e.seed.ClientAccount(), e.seed.ClientAccount()
	if _, err := e.seed.DB().ExecContext(t.Context(), `UPDATE orders SET client_account_id = $1 WHERE id = $2`,
		account.String(), o.ID().String()); err != nil {
		t.Fatal(err)
	}
	read := func(scope appshared.Scope) (order.Record, error) {
		var r order.Record
		err := e.run(scope, func(ctx context.Context) error {
			var err error
			r, err = e.repo.ByID(ctx, e.tenant, o.ID())
			return err
		})
		return r, err
	}
	staff, err := read(e.staff())
	if err != nil {
		t.Fatal(err)
	}
	if staff.Lines[0].Items[0].Sourcing.SupplierName == "" || staff.Lines[0].Items[0].Sourcing.Cost == nil {
		t.Fatalf("staff read no sourcing: %+v", staff.Lines[0].Items[0].Sourcing)
	}
	withoutSourcing := staff
	withoutSourcing.Lines = append([]order.LineRecord(nil), staff.Lines...)
	for i := range withoutSourcing.Lines {
		withoutSourcing.Lines[i].Items = append([]order.ItemRecord(nil), staff.Lines[i].Items...)
		for j := range withoutSourcing.Lines[i].Items {
			withoutSourcing.Lines[i].Items[j].Sourcing = order.Sourcing{}
		}
	}
	for name, scope := range map[string]appshared.Scope{
		"a client account": {ClientAccountID: account},
		"a store page":     e.storePage(),
	} {
		got, err := read(scope)
		if err != nil {
			t.Errorf("%s: ByID err = %v, want the order with no sourcing", name, err)
			continue
		}
		if !reflect.DeepEqual(got, withoutSourcing) {
			t.Errorf("%s read\n got: %+v\nwant: %+v", name, got, withoutSourcing)
		}
	}
	if _, err := read(appshared.Scope{ClientAccountID: other}); !errors.Is(err, apporder.ErrNotFound) {
		t.Errorf("another client account: ByID err = %v, want ErrNotFound", err)
	}

	t.Run("those scopes write nothing through the repository", func(t *testing.T) {
		loaded := e.reload(o.ID())
		loaded.RotateTracking(shared.SecretHash(pgtest.RandomHash()), mustPrincipal(t, e), epochNow())
		for name, scope := range map[string]appshared.Scope{"a store page": e.storePage()} {
			if err := e.run(scope, func(ctx context.Context) error { return e.repo.Update(ctx, loaded) }); err == nil {
				t.Errorf("%s: Update succeeded", name)
			}
			err := e.run(scope, func(ctx context.Context) error {
				return e.repo.AddMessage(ctx, apporder.Message{
					ID: pgtest.NewID(), Order: o.ID(), Kind: apporder.MessageCreated, ToPhone: "5592999990000",
					Text: "Oi", URL: "https://wa.me/5592999990000", At: epochNow(),
				})
			})
			if err == nil {
				t.Errorf("%s: AddMessage succeeded", name)
			}
		}
		if n := e.count(`SELECT count(*) FROM order_messages WHERE tenant_id = $1`, e.tenant.String()); n != 0 {
			t.Errorf("%d messages stored by scopes without staff", n)
		}
		if v := e.count(`SELECT version FROM orders WHERE id = $1`, o.ID().String()); v != 1 {
			t.Errorf("version = %d after refused updates, want 1", v)
		}
	})
}

func epochNow() time.Time { return time.Date(2026, 12, 2, 10, 0, 0, 0, time.UTC) }

func mustPrincipal(t *testing.T, e *orderEnv) shared.Principal {
	t.Helper()
	p, err := shared.NewBackofficePrincipal(e.user, e.tenant, shared.RoleOwner)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// reload reads an order as the app layer does.
func (e *orderEnv) reload(id shared.OrderID) *order.Order {
	e.t.Helper()
	var o *order.Order
	e.must(e.staff(), func(ctx context.Context) error {
		r, err := e.repo.ByID(ctx, e.tenant, id)
		if err != nil {
			return err
		}
		o, err = order.Restore(r, e.wf)
		return err
	})
	return o
}

func TestOrdersRepository_NeverWritesTheClientAccount(t *testing.T) {
	e := newOrderEnv(t)
	o := e.insert()
	clientOf := func() *string {
		var s *string
		if err := e.seed.DB().QueryRowContext(t.Context(), `SELECT client_account_id::text FROM orders WHERE id = $1`, o.ID().String()).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	if got := clientOf(); got != nil {
		t.Fatalf("a new order is linked to account %s, want none", *got)
	}
	account := e.seed.ClientAccount()
	if _, err := e.seed.DB().ExecContext(t.Context(), `UPDATE orders SET client_account_id = $1 WHERE id = $2`, account.String(), o.ID().String()); err != nil {
		t.Fatal(err)
	}
	loaded := e.reload(o.ID())
	loaded.RotateTracking(shared.SecretHash(pgtest.RandomHash()), mustPrincipal(t, e), epochNow())
	e.must(e.staff(), func(ctx context.Context) error { return e.repo.Update(ctx, loaded) })
	if got := clientOf(); got == nil || *got != account.String() {
		t.Errorf("client account after an update = %v, want it untouched (%s)", got, account)
	}
}

func TestOrdersRepository_ConstraintMapping(t *testing.T) {
	e := newOrderEnv(t)
	o := e.insert()

	t.Run("a number the store already uses is a conflict", func(t *testing.T) {
		err := e.run(e.staff(), func(ctx context.Context) error { return e.repo.Insert(ctx, e.build(o.Number(), e.customer)) })
		if !errors.Is(err, shared.ErrConflict) {
			t.Errorf("insert err = %v, want shared.ErrConflict", err)
		}
	})
	t.Run("a customer of another store is an invalid reference", func(t *testing.T) {
		other := newOrderEnv(t)
		err := e.run(e.staff(), func(ctx context.Context) error { return e.repo.Insert(ctx, e.build(99, other.customer)) })
		if !errors.Is(err, shared.ErrInvalid) {
			t.Errorf("insert err = %v, want shared.ErrInvalid", err)
		}
	})
	t.Run("a message for an order the store lacks is an invalid reference", func(t *testing.T) {
		err := e.run(e.staff(), func(ctx context.Context) error {
			return e.repo.AddMessage(ctx, apporder.Message{
				ID: pgtest.NewID(), Order: shared.OrderID(pgtest.NewID()), Kind: apporder.MessageCreated, ToPhone: "5592999990000",
				Text: "Oi", URL: "https://wa.me/5592999990000", By: e.user, At: epochNow(),
			})
		})
		if !errors.Is(err, shared.ErrInvalid) {
			t.Errorf("AddMessage err = %v, want shared.ErrInvalid", err)
		}
	})
	t.Run("a message is stored for an order of the store", func(t *testing.T) {
		e.must(e.staff(), func(ctx context.Context) error {
			return e.repo.AddMessage(ctx, apporder.Message{
				ID: pgtest.NewID(), Order: o.ID(), Kind: apporder.MessageTrackingLink, ToPhone: "5592999990000",
				Text: "Acompanhe: [link de acompanhamento]", URL: "https://wa.me/5592999990000?text=Acompanhe", By: e.user, At: epochNow(),
			})
		})
		if n := e.count(`SELECT count(*) FROM order_messages WHERE order_id = $1 AND by_user = $2`, o.ID().String(), e.user.String()); n != 1 {
			t.Errorf("%d messages stored, want 1", n)
		}
	})
	t.Run("a cursor the repository did not issue is refused", func(t *testing.T) {
		bad, err := shared.NewPageRequest("not-a-cursor", 10)
		if err != nil {
			t.Fatal(err)
		}
		err = e.run(e.staff(), func(ctx context.Context) error {
			_, err := e.repo.List(ctx, e.tenant, apporder.ListFilter{Scope: apporder.ScopeAll}, bad)
			return err
		})
		if !errors.Is(err, shared.ErrInvalid) {
			t.Errorf("List err = %v, want shared.ErrInvalid", err)
		}
		err = e.run(e.staff(), func(ctx context.Context) error {
			_, err := e.repo.History(ctx, e.tenant, o.ID(), bad)
			return err
		})
		if !errors.Is(err, shared.ErrInvalid) {
			t.Errorf("History err = %v, want shared.ErrInvalid", err)
		}
	})
}

// syncBuffer is a log sink the pool's goroutines may write to.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestOrdersRepository_TrackingHashIsStoredWhole_AndNeverLogged(t *testing.T) {
	var logs syncBuffer
	db, err := postgres.Open(t.Context(), config.Database{
		URL: env.DSN(pgtest.App), MaxOpenConns: 4, MaxIdleConns: 2, ConnMaxLifetime: time.Minute,
		StatementTimeout: 5 * time.Second, SlowQueryThreshold: time.Second,
	}, slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	e := newOrderEnv(t)
	e.tx = postgres.NewTxManager(db)
	o := e.insert()
	loaded := e.reload(o.ID())
	fresh := shared.SecretHash(pgtest.RandomHash())
	loaded.RotateTracking(fresh, mustPrincipal(t, e), epochNow())
	e.must(e.staff(), func(ctx context.Context) error { return e.repo.Update(ctx, loaded) })

	var stored []byte
	if err := e.seed.DB().QueryRowContext(t.Context(), `SELECT tracking_token_hash FROM orders WHERE id = $1`, o.ID().String()).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if len(stored) != 32 || !bytes.Equal(stored, fresh[:]) {
		t.Errorf("stored hash = %x (%d bytes), want the new 32-byte hash %x", stored, len(stored), fresh)
	}
	out := logs.String()
	if !strings.Contains(out, "orders") {
		t.Fatalf("the log has no query, the test would prove nothing: %q", out)
	}
	for _, h := range [][32]byte{o.TrackingHash(), fresh} {
		for _, form := range []string{hexenc.EncodeToString(h[:]), base64.StdEncoding.EncodeToString(h[:]), base64.RawURLEncoding.EncodeToString(h[:])} {
			if strings.Contains(out, form) {
				t.Errorf("a tracking hash reached the log as %q", form)
			}
		}
	}
}

func TestOrdersRepository_HandedOverStopsAtTheCap(t *testing.T) {
	e := newOrderEnv(t)
	last := e.wf.Statuses()[len(e.wf.Statuses())-1].Key
	_, err := e.seed.DB().ExecContext(t.Context(), `INSERT INTO orders (tenant_id, id, number, customer_id, workflow_id,
			workflow_version, status_key, delivery_date, assembly_days, safety_margin_days, deposit_percent, currency,
			subtotal_cents, handover, tracking_token_hash, handed_over_as, handed_over_at)
		SELECT $1::uuid, gen_random_uuid(), i, $2::uuid, $3::uuid, $4, $5, current_date, 3, 2, 50, 'BRL', 100000,
			'pickup', decode(md5(random()::text || i::text) || md5(random()::text), 'hex'), 'pickup',
			timestamptz '2026-12-01 00:00:00+00' + make_interval(secs => i)
		FROM generate_series(1, $6::int) AS i`,
		e.tenant.String(), e.customer.String(), e.wf.ID().String(), e.wf.Version(), string(last), apporder.HandedOverCap+5)
	if err != nil {
		t.Fatal(err)
	}
	var got []order.Record
	e.must(e.staff(), func(ctx context.Context) error {
		var err error
		got, err = e.repo.HandedOver(ctx, e.tenant, time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 2, 0, 0, 0, 0, time.UTC))
		return err
	})
	if len(got) != apporder.HandedOverCap {
		t.Fatalf("handed over = %d orders, want the cap, %d", len(got), apporder.HandedOverCap)
	}
	if got[0].Number != 1 || got[len(got)-1].Number != order.Number(apporder.HandedOverCap) {
		t.Errorf("the cap kept orders %d to %d, want the oldest 1 to %d", got[0].Number, got[len(got)-1].Number, apporder.HandedOverCap)
	}
}
