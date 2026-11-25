//go:build integration

package rls_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

// world is two stores, A and B, each with an owner, an invitation, audit
// rows and an outbox event; a third store C; a user who belongs to A and B;
// a user with no store; and a client account.
type world struct {
	a, b, c                     shared.TenantID
	ownerA, ownerB, both, spare shared.UserID
	account                     shared.ClientAccountID
}

func seed(t *testing.T) world {
	t.Helper()
	s := env.Seeder(t)
	w := world{ownerA: s.User(), ownerB: s.User(), both: s.User(), spare: s.User()}
	w.a, w.b, w.c = s.Tenant(w.ownerA), s.Tenant(w.ownerB), s.Tenant(s.User())
	s.Member(w.a, w.both, shared.RoleStaff)
	s.Member(w.b, w.both, shared.RoleViewer)
	for _, tenant := range []shared.TenantID{w.a, w.b, w.c} {
		s.Invitation(tenant, w.ownerA)
		s.Event(tenant)
	}
	w.account = s.ClientAccount()
	return w
}

// tenantTable describes one tenant-scoped table for the five cases.
type tenantTable struct {
	name string
	// column holds the row's tenant: tenant_id, or id on tenants itself.
	column string
	// grants is what encomendo_app may do: s(elect) i(nsert) u(pdate) d(elete).
	grants string
	// insert writes one row that belongs to tenant.
	insert func(w world, tenant shared.TenantID) (string, []any)
	// set is the SET clause of the update case.
	set string
}

func (tt tenantTable) can(op string) bool { return strings.Contains(tt.grants, op) }

// tenantTables lists every tenant-scoped table. TestEveryTenantTableIsListed
// fails when a migration adds one that is missing here.
var tenantTables = []tenantTable{
	{
		name: "tenants", column: "id", grants: "siu", set: "name = 'Outra loja'",
		insert: func(_ world, tenant shared.TenantID) (string, []any) {
			return `INSERT INTO tenants (id, slug, name) VALUES ($1, $2, 'Nova loja')`,
				[]any{tenant.String(), "nova-" + tenant.String()[24:]}
		},
	},
	{
		name: "memberships", column: "tenant_id", grants: "siud", set: "role = 'admin'",
		insert: func(w world, tenant shared.TenantID) (string, []any) {
			return `INSERT INTO memberships (tenant_id, user_id, role) VALUES ($1, $2, 'staff')`,
				[]any{tenant.String(), w.spare.String()}
		},
	},
	{
		name: "tenant_settings", column: "tenant_id", grants: "siu", set: "deposit_percent = 10",
		insert: func(_ world, tenant shared.TenantID) (string, []any) {
			return `INSERT INTO tenant_settings (tenant_id, status_workflow_id, status_workflow_version)
				SELECT $1, id, version FROM status_workflows WHERE is_default`, []any{tenant.String()}
		},
	},
	{
		name: "invitations", column: "tenant_id", grants: "siu", set: "role = 'viewer'",
		insert: func(w world, tenant shared.TenantID) (string, []any) {
			return `INSERT INTO invitations (tenant_id, id, email, role, token_hash, invited_by, expires_at)
				VALUES ($1, $2, 'novo@teste.encomendo.com.br', 'staff', $3, $4, $5)`,
				[]any{tenant.String(), pgtest.NewID().String(), pgtest.RandomHash(), w.ownerA.String(), time.Now().Add(time.Hour)}
		},
	},
	{
		name: "audit_log", column: "tenant_id", grants: "si", set: "action = 'delete'",
		insert: func(_ world, tenant shared.TenantID) (string, []any) {
			return `INSERT INTO audit_log (tenant_id, table_name, action) VALUES ($1, 'memberships', 'insert')`,
				[]any{tenant.String()}
		},
	},
	{
		name: "outbox_events", column: "tenant_id", grants: "i", set: "name = 'other.name'",
		insert: func(_ world, tenant shared.TenantID) (string, []any) {
			return `INSERT INTO outbox_events (tenant_id, name, payload) VALUES ($1, 'test.inserted', '{}')`,
				[]any{tenant.String()}
		},
	},
}

// TestTenantTables runs the five RLS cases of backend/CLAUDE.md on every
// tenant table, plus a positive insert so a policy that refuses everything
// cannot pass: as store A, rows of B are invisible and untouchable, and with
// no tenant set nothing is visible.
func TestTenantTables(t *testing.T) {
	t.Parallel()
	w := seed(t)
	app := env.Open(t, pgtest.App)
	for _, tt := range tenantTables {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Run("select sees only its own tenant", func(t *testing.T) { caseSelect(t, app, w, tt) })
			t.Run("insert for another tenant is refused", func(t *testing.T) { caseInsertOther(t, app, w, tt) })
			t.Run("insert for its own tenant works", func(t *testing.T) { caseInsertOwn(t, app, w, tt) })
			t.Run("update of another tenant changes nothing", func(t *testing.T) {
				caseWrite(t, app, w, tt, "u", "UPDATE "+tt.name+" SET "+tt.set+" WHERE "+tt.column+" = $1")
			})
			t.Run("delete of another tenant changes nothing", func(t *testing.T) {
				caseWrite(t, app, w, tt, "d", "DELETE FROM "+tt.name+" WHERE "+tt.column+" = $1")
			})
			t.Run("no tenant set sees and writes nothing", func(t *testing.T) { caseNoTenant(t, app, w, tt) })
			t.Run("own row cannot move to another tenant", func(t *testing.T) { caseMove(t, app, w, tt) })
		})
	}
}

func storeA(w world) appshared.Scope { return appshared.Scope{TenantID: w.a, UserID: w.ownerA} }

func caseSelect(t *testing.T, app *sql.DB, w world, tt tenantTable) {
	err := inScope(t, app, storeA(w), func(ctx context.Context, tx *sql.Tx) error {
		total, err := count(ctx, tx, "SELECT count(*) FROM "+tt.name)
		if err != nil {
			return err
		}
		own, err := count(ctx, tx, "SELECT count(*) FROM "+tt.name+" WHERE "+tt.column+" = $1", w.a.String())
		if err != nil {
			return err
		}
		if own == 0 || total != own {
			t.Errorf("%s as store A: %d rows, %d of them A's; want only A's, at least one", tt.name, total, own)
		}
		return nil
	})
	expectRefusedUnless(t, tt.can("s"), err)
}

func caseInsertOther(t *testing.T, app *sql.DB, w world, tt tenantTable) {
	target := w.b
	if tt.name == "tenants" {
		target = shared.TenantID(pgtest.NewID()) // B exists; any id but A's is another tenant
	}
	err := inScope(t, app, storeA(w), func(ctx context.Context, tx *sql.Tx) error {
		query, args := tt.insert(w, target)
		_, err := tx.ExecContext(ctx, query, args...)
		return err
	})
	if !refused(err) {
		t.Errorf("insert into %s for another tenant: err = %v, want 42501", tt.name, err)
	}
}

func caseInsertOwn(t *testing.T, app *sql.DB, w world, tt tenantTable) {
	if !tt.can("i") {
		t.Skip("encomendo_app cannot insert here")
	}
	// A fresh store, as CreateTenant scopes its transaction to the new tenant.
	fresh := shared.TenantID(pgtest.NewID())
	err := inScope(t, app, appshared.Scope{TenantID: fresh, UserID: w.ownerA}, func(ctx context.Context, tx *sql.Tx) error {
		if tt.name != "tenants" {
			create, args := tenantTables[0].insert(w, fresh)
			if _, err := tx.ExecContext(ctx, create, args...); err != nil {
				return err
			}
		}
		query, args := tt.insert(w, fresh)
		_, err := tx.ExecContext(ctx, query, args...)
		return err
	})
	if err != nil {
		t.Errorf("insert into %s for its own tenant: %v", tt.name, err)
	}
}

func caseWrite(t *testing.T, app *sql.DB, w world, tt tenantTable, op, query string) {
	err := inScope(t, app, storeA(w), func(ctx context.Context, tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, query, w.b.String())
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 0 { // pgx always reports the count
			t.Errorf("%q as store A changed %d of B's rows", query, n)
		}
		return nil
	})
	expectRefusedUnless(t, tt.can(op), err)
}

func caseNoTenant(t *testing.T, app *sql.DB, w world, tt tenantTable) {
	if tt.can("s") {
		err := inScope(t, app, appshared.Scope{}, func(ctx context.Context, tx *sql.Tx) error {
			n, err := count(ctx, tx, "SELECT count(*) FROM "+tt.name)
			if err == nil && n != 0 {
				t.Errorf("%s with no tenant set: %d rows, want 0", tt.name, n)
			}
			return err
		})
		if err != nil {
			t.Errorf("select %s with no tenant set: %v", tt.name, err)
		}
	}
	err := inScope(t, app, appshared.Scope{}, func(ctx context.Context, tx *sql.Tx) error {
		query, args := tt.insert(w, w.a)
		if tt.name == "tenants" {
			query, args = tt.insert(w, shared.TenantID(pgtest.NewID()))
		}
		_, err := tx.ExecContext(ctx, query, args...)
		return err
	})
	if !refused(err) {
		t.Errorf("insert into %s with no tenant set: err = %v, want 42501", tt.name, err)
	}
}

// caseMove rewrites the tenant column of store A's own rows to B. The column
// grants refuse it before any policy runs; that the policy would refuse it
// too is TestPolicies_OnlyTheListedExtrasWidenReads (WITH CHECK = USING).
func caseMove(t *testing.T, app *sql.DB, w world, tt tenantTable) {
	err := inScope(t, app, storeA(w), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE "+tt.name+" SET "+tt.column+" = $1 WHERE "+tt.column+" = $2",
			w.b.String(), w.a.String())
		return err
	})
	if !refused(err) {
		t.Errorf("moving %s rows from A to B: err = %v, want 42501", tt.name, err)
	}
}

// expectRefusedUnless checks err for a case the app is granted (no error)
// or not granted (refused with 42501).
func expectRefusedUnless(t *testing.T, granted bool, err error) {
	t.Helper()
	switch {
	case granted && err != nil:
		t.Errorf("granted statement failed: %v", err)
	case !granted && !refused(err):
		t.Errorf("statement without a grant: err = %v, want 42501", err)
	}
}
