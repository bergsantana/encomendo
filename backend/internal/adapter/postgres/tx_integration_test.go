//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/platform/config"
	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

// These tests sit inside the package because they read the transaction from
// the context the way repositories do. The port's shared contract runs from
// test/integration/postgres.

var env *pgtest.Env

func TestMain(m *testing.M) {
	os.Exit(pgtest.Run(m, &env))
}

func openApp(t *testing.T, maxConns int) *DB {
	t.Helper()
	db, err := Open(t.Context(), config.Database{
		URL:                env.DSN(pgtest.App),
		MaxOpenConns:       maxConns,
		MaxIdleConns:       maxConns,
		ConnMaxLifetime:    time.Minute,
		StatementTimeout:   5 * time.Second,
		SlowQueryThreshold: time.Second,
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

type settings struct{ tenant, user, client string }

func readSettings(ctx context.Context, t *testing.T) settings {
	t.Helper()
	db, err := conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var s settings
	row := db.Raw(`SELECT current_setting('app.tenant_id', true), current_setting('app.user_id', true),
		current_setting('app.client_account_id', true)`).Row()
	if err := row.Scan(&s.tenant, &s.user, &s.client); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestTxManager_SetsEveryScopeSettingOnEveryTransaction(t *testing.T) {
	t.Parallel()
	tenant, user := shared.TenantID(pgtest.NewID()), shared.UserID(pgtest.NewID())
	account := shared.ClientAccountID(pgtest.NewID())
	cases := map[string]struct {
		scope appshared.Scope
		want  settings
	}{
		"staff in a store": {appshared.Scope{TenantID: tenant, UserID: user}, settings{tenant.String(), user.String(), ""}},
		"user only":        {appshared.Scope{UserID: user}, settings{"", user.String(), ""}},
		"client account":   {appshared.Scope{ClientAccountID: account}, settings{"", "", account.String()}},
		"zero scope":       {appshared.Scope{}, settings{"", "", ""}},
	}
	// One connection, so each case also proves the previous one's settings
	// did not leak into it.
	tx := NewTxManager(openApp(t, 1))
	for name, c := range cases {
		err := tx.WithinTx(appshared.WithScope(t.Context(), c.scope), func(ctx context.Context) error {
			if got := readSettings(ctx, t); got != c.want {
				t.Errorf("%s: settings = %+v, want %+v", name, got, c.want)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestTxManager_SettingsEndWithTheTransaction(t *testing.T) {
	t.Parallel()
	db := openApp(t, 1)
	scope := appshared.Scope{TenantID: shared.TenantID(pgtest.NewID())}
	if err := NewTxManager(db).WithinTx(appshared.WithScope(t.Context(), scope), func(context.Context) error {
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var after string
	if err := db.Gorm().WithContext(t.Context()).Raw(`SELECT current_setting('app.tenant_id', true)`).Row().Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != "" {
		t.Errorf("app.tenant_id after commit = %q, want '' (set_config must be transaction-local)", after)
	}
}

// insertTenant writes a store row in the transaction of ctx, whose scope
// must be that store, as CreateTenant does.
func insertTenant(ctx context.Context, id shared.TenantID) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	return db.Exec(`INSERT INTO tenants (id, slug, name) VALUES (?, ?, 'Loja')`, id.String(), "tx-"+id.String()[24:]).Error
}

func renameTenant(ctx context.Context, id shared.TenantID, name string) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	return db.Exec(`UPDATE tenants SET name = ? WHERE id = ?`, name, id.String()).Error
}

func tenantName(t *testing.T, id shared.TenantID) string {
	t.Helper()
	var name string
	if err := env.Seeder(t).DB().QueryRowContext(t.Context(),
		`SELECT name FROM tenants WHERE id = $1`, id.String()).Scan(&name); err != nil {
		t.Fatal(err)
	}
	return name
}

func tenantExists(t *testing.T, id shared.TenantID) bool {
	t.Helper()
	var n int
	if err := env.Seeder(t).DB().QueryRowContext(t.Context(),
		`SELECT count(*) FROM tenants WHERE id = $1`, id.String()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

func storeCtx(t *testing.T, id shared.TenantID) context.Context {
	return appshared.WithScope(t.Context(), appshared.Scope{TenantID: id})
}

func TestTxManager_CommitsAndRollsBack(t *testing.T) {
	t.Parallel()
	tx := NewTxManager(openApp(t, 2))
	kept, dropped := shared.TenantID(pgtest.NewID()), shared.TenantID(pgtest.NewID())
	if err := tx.WithinTx(storeCtx(t, kept), func(ctx context.Context) error { return insertTenant(ctx, kept) }); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("use case failed")
	err := tx.WithinTx(storeCtx(t, dropped), func(ctx context.Context) error {
		if err := insertTenant(ctx, dropped); err != nil {
			return err
		}
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatalf("WithinTx = %v, want fn's error", err)
	}
	if !tenantExists(t, kept) || tenantExists(t, dropped) {
		t.Errorf("committed row exists = %v, rolled back row exists = %v; want true, false",
			tenantExists(t, kept), tenantExists(t, dropped))
	}
}

func TestTxManager_NestedCallIsASavepointInTheSameTransaction(t *testing.T) {
	t.Parallel()
	tx := NewTxManager(openApp(t, 2))
	id := shared.TenantID(pgtest.NewID())
	failure := errors.New("inner failed")
	err := tx.WithinTx(storeCtx(t, id), func(ctx context.Context) error {
		if err := insertTenant(ctx, id); err != nil {
			return err
		}
		inner := tx.WithinTx(ctx, func(ctx context.Context) error {
			db, err := conn(ctx)
			if err != nil {
				return err
			}
			var seen int
			if err := db.Raw(`SELECT count(*) FROM tenants WHERE id = ?`, id.String()).Row().Scan(&seen); err != nil {
				return err
			}
			if seen != 1 {
				t.Error("the nested call does not see the outer call's uncommitted row")
			}
			if err := renameTenant(ctx, id, "Desfeito"); err != nil {
				return err
			}
			return failure
		})
		if !errors.Is(inner, failure) {
			return inner
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	name := tenantName(t, id)
	if name != "Loja" {
		t.Errorf("name = %q, want the inner update undone and the outer insert kept", name)
	}
}

func TestTxManager_PanicRollsBackAndPropagates(t *testing.T) {
	t.Parallel()
	tx := NewTxManager(openApp(t, 2))
	id := shared.TenantID(pgtest.NewID())
	func() {
		defer func() {
			if r := recover(); r != "boom" {
				t.Errorf("recovered %v, want the panic to reach the caller", r)
			}
		}()
		_ = tx.WithinTx(storeCtx(t, id), func(ctx context.Context) error {
			if err := insertTenant(ctx, id); err != nil {
				t.Fatal(err)
			}
			panic("boom")
		})
	}()
	if tenantExists(t, id) {
		t.Error("the row written before the panic was committed")
	}
}

func TestTxManager_CanceledDuringFnRollsBack(t *testing.T) {
	t.Parallel()
	tx := NewTxManager(openApp(t, 2))
	id := shared.TenantID(pgtest.NewID())
	ctx, cancel := context.WithCancel(storeCtx(t, id))
	// fn succeeds, but its context is gone before the commit: the commit
	// fails and the row must not stay.
	err := tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := insertTenant(ctx, id); err != nil {
			return err
		}
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("WithinTx = %v, want context.Canceled from the failed commit", err)
	}
	if tenantExists(t, id) {
		t.Error("the row of a canceled transaction was committed")
	}
}

// TestTxManager_CanceledCommitReportsTheCancellation is the regression for
// a flaky failure of the test above. database/sql rolls a transaction back
// from its own goroutine when the context ends; if that goroutine wins the
// race, Commit says sql.ErrTxDone, and a request that went away looked like
// a server failure. Here the test waits until that rollback has happened,
// so the race is lost every time.
func TestTxManager_CanceledCommitReportsTheCancellation(t *testing.T) {
	t.Parallel()
	tx := NewTxManager(openApp(t, 2))
	id := shared.TenantID(pgtest.NewID())
	ctx, cancel := context.WithCancel(storeCtx(t, id))

	err := tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := insertTenant(ctx, id); err != nil {
			return err
		}
		cancel()
		waitRolledBack(ctx, t)
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("WithinTx = %v, want context.Canceled", err)
	}
	if tenantExists(t, id) {
		t.Error("the row of a canceled transaction was committed")
	}
}

// waitRolledBack waits until database/sql has rolled back the transaction
// in ctx after ctx ended: a statement on an uncanceled context then fails
// with sql.ErrTxDone.
func waitRolledBack(ctx context.Context, t *testing.T) {
	t.Helper()
	tx, err := conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	db := tx.WithContext(context.WithoutCancel(ctx))
	deadline := time.Now().Add(5 * time.Second)
	for !errors.Is(db.Exec("SELECT 1").Error, sql.ErrTxDone) {
		if time.Now().After(deadline) {
			t.Fatal("database/sql never rolled back the canceled transaction")
		}
		runtime.Gosched()
	}
}

// TestTxManager_CanceledNestedCallStillRollsBack is the regression for a
// nested call whose own context is canceled: its rollback to the savepoint
// must still run, or the outer commit keeps the inner writes.
func TestTxManager_CanceledNestedCallStillRollsBack(t *testing.T) {
	t.Parallel()
	tx := NewTxManager(openApp(t, 2))
	id := shared.TenantID(pgtest.NewID())
	err := tx.WithinTx(storeCtx(t, id), func(ctx context.Context) error {
		if err := insertTenant(ctx, id); err != nil {
			return err
		}
		innerCtx, cancel := context.WithCancel(ctx)
		inner := tx.WithinTx(innerCtx, func(ctx context.Context) error {
			if err := renameTenant(ctx, id, "Desfeito"); err != nil {
				return err
			}
			cancel()
			return ctx.Err()
		})
		if !errors.Is(inner, context.Canceled) {
			return inner
		}
		return nil // the outer work goes on without the failed inner part
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := tenantName(t, id); got != "Loja" {
		t.Errorf("name = %q, want the canceled inner update undone", got)
	}
}

// TestTxManager_RefusesToCommitWhenAnUndoFails: if the rollback to a
// savepoint fails, the inner writes may still be in the transaction, so the
// whole transaction rolls back even though the outer fn succeeded.
func TestTxManager_RefusesToCommitWhenAnUndoFails(t *testing.T) {
	t.Parallel()
	tx := NewTxManager(openApp(t, 2))
	id := shared.TenantID(pgtest.NewID())
	failure := errors.New("inner failed")
	var inner error
	err := tx.WithinTx(storeCtx(t, id), func(ctx context.Context) error {
		if err := insertTenant(ctx, id); err != nil {
			return err
		}
		inner = tx.WithinTx(ctx, func(ctx context.Context) error {
			db, err := conn(ctx)
			if err != nil {
				return err
			}
			// Releasing the savepoint makes the rollback to it fail.
			if err := db.Exec("RELEASE SAVEPOINT sp_1").Error; err != nil {
				return err
			}
			return failure
		})
		return nil
	})
	if !errors.Is(inner, failure) {
		t.Errorf("nested WithinTx = %v, want fn's error kept", inner)
	}
	if err == nil {
		t.Error("the transaction committed after a failed rollback to a savepoint")
	}
	if tenantExists(t, id) {
		t.Error("the outer row was committed although the transaction was broken")
	}
}

// TestTxManager_RefusesToCommitABrokenTransaction covers an undo that never
// reached the server (a busy connection, say): Postgres does not know the
// transaction is broken, so only the TxManager's own flag stops the commit.
func TestTxManager_RefusesToCommitABrokenTransaction(t *testing.T) {
	t.Parallel()
	tx := NewTxManager(openApp(t, 2))
	id := shared.TenantID(pgtest.NewID())
	broken := errors.New("undo never ran")
	err := tx.WithinTx(storeCtx(t, id), func(ctx context.Context) error {
		if err := insertTenant(ctx, id); err != nil {
			return err
		}
		c, _ := ctx.Value(txKey{}).(*txConn) // WithinTx always puts one there
		c.root.breakWith(broken)
		return nil
	})
	if !errors.Is(err, broken) {
		t.Errorf("WithinTx = %v, want the broken transaction refused", err)
	}
	if tenantExists(t, id) {
		t.Error("a broken transaction was committed")
	}
}

func TestTxManager_RLSRefusesARowOutsideTheScope(t *testing.T) {
	t.Parallel()
	tx := NewTxManager(openApp(t, 2))
	scoped, other := shared.TenantID(pgtest.NewID()), shared.TenantID(pgtest.NewID())
	err := tx.WithinTx(storeCtx(t, scoped), func(ctx context.Context) error { return insertTenant(ctx, other) })
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
		t.Errorf("insert of another store's row = %v, want RLS to refuse it (42501)", err)
	}
	if tenantExists(t, other) {
		t.Error("RLS let a row outside the scope in")
	}
}
