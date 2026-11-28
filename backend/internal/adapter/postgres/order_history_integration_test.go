//go:build integration

package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/adapter/system"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

// This test sits inside the package because it calls appendHistory, which
// Insert and Update reach only from a staff scope: the sourcing they write
// is staff-only, so a store page scope cannot yet place an order through the
// repository (milestone 7 decides how, ADR 0027). What must already hold is
// the history policy: any scope of the store writes an entry, none but staff
// reads one, so the insert may not ask for a row back (RETURNING would need
// the read policy).
func TestOrders_AppendHistory_StorePageScopeWritesButReadsNothing(t *testing.T) {
	s := env.Seeder(t)
	user := s.User()
	tenant := s.Tenant(user)
	tx := NewTxManager(openApp(t, 4))
	repo := NewOrders(system.NewIDs())

	customer := pgtest.NewID()
	_, err := s.DB().ExecContext(t.Context(),
		`INSERT INTO customers (tenant_id, id, name, whatsapp_phone) VALUES ($1, $2, 'Cliente', '5592977770000')`,
		tenant.String(), customer.String())
	if err != nil {
		t.Fatal(err)
	}
	wf := defaultWorkflowFor(t, tx)
	spec := testkit.NewOrder().Spec(t)
	spec.Tenant, spec.Customer, spec.Workflow = tenant, shared.CustomerID(customer), wf
	spec.Tracking = shared.SecretHash(pgtest.RandomHash())
	o, err := order.New(spec)
	if err != nil {
		t.Fatal(err)
	}
	staff := appshared.Scope{TenantID: tenant, UserID: user}
	run := func(scope appshared.Scope, fn func(ctx context.Context) error) error {
		return tx.WithinTx(appshared.WithScope(context.Background(), scope), fn)
	}
	if err := run(staff, func(ctx context.Context) error { return repo.Insert(ctx, o) }); err != nil {
		t.Fatalf("insert: %v", err)
	}
	count := func() int {
		var n int
		row := s.DB().QueryRowContext(t.Context(), `SELECT count(*) FROM order_status_history WHERE order_id = $1`, o.ID().String())
		if err := row.Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(); n != 1 {
		t.Fatalf("history rows after insert = %d, want 1 (created)", n)
	}

	storePage := appshared.Scope{TenantID: tenant}
	err = run(storePage, func(ctx context.Context) error {
		db, err := conn(ctx)
		if err != nil {
			return err
		}
		return repo.appendHistory(db, o) // writes the pending "created" entry again, as a second order would
	})
	if err != nil {
		t.Fatalf("a store page scope could not write a history entry: %v", err)
	}
	if n := count(); n != 2 {
		t.Errorf("history rows = %d, want 2", n)
	}
	err = run(storePage, func(ctx context.Context) error {
		p, err := repo.History(ctx, tenant, o.ID(), mustPage(t))
		if err != nil {
			return err
		}
		if len(p.Items) != 0 {
			t.Errorf("a store page scope read %d history entries, want 0", len(p.Items))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("history read in a store page scope: %v", err)
	}
	err = run(staff, func(ctx context.Context) error {
		p, err := repo.History(ctx, tenant, o.ID(), mustPage(t))
		if err != nil {
			return err
		}
		if len(p.Items) != 2 || !strings.EqualFold(string(p.Items[0].Entry.Kind), string(order.HistoryCreated)) {
			t.Errorf("staff read %d entries, want 2 created entries", len(p.Items))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func mustPage(t *testing.T) shared.PageRequest {
	t.Helper()
	p, err := shared.NewPageRequest("", 10)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func defaultWorkflowFor(t *testing.T, tx *TxManager) (wf workflow.Workflow) {
	t.Helper()
	err := tx.WithinTx(context.Background(), func(ctx context.Context) error {
		var err error
		wf, err = NewWorkflows().Default(ctx)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return wf
}
