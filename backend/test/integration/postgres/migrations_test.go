//go:build integration

// Package postgres_test runs the Postgres adapter against a real Postgres
// started with the same init scripts as docker compose: the migrations, the
// role setup tenant isolation depends on, the shared port contracts and the
// seeded status workflows.
package postgres_test

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres"
	"github.com/bergsantana/encomendo/backend/internal/platform/config"
	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

// env is one Postgres, migrated, shared by the tests of this package.
var env *pgtest.Env

func TestMain(m *testing.M) {
	os.Exit(pgtest.Run(m, &env))
}

func openApp(t *testing.T) *postgres.DB {
	t.Helper()
	db, err := postgres.Open(t.Context(), config.Database{
		URL:                env.DSN(pgtest.App),
		MaxOpenConns:       4,
		MaxIdleConns:       2,
		ConnMaxLifetime:    time.Minute,
		StatementTimeout:   5 * time.Second,
		SlowQueryThreshold: time.Second,
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestMigrations_UpDownUp runs on its own database, because it drops
// everything the other tests use.
func TestMigrations_UpDownUp(t *testing.T) {
	fresh := pgtest.StartForTest(t)
	provider := fresh.Provider(t)
	ctx := t.Context()

	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("first up: %v", err)
	}
	if _, err := provider.DownTo(ctx, 0); err != nil {
		t.Fatalf("down to 0: %v", err)
	}
	var leftover int
	if err := fresh.Open(t, pgtest.Superuser).QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE n.nspname = 'public' AND c.relkind IN ('r', 'v', 'S', 'f')
		   AND c.relname NOT IN ('goose_db_version', 'goose_db_version_id_seq'))
		+ (SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace WHERE n.nspname = 'public')`).
		Scan(&leftover); err != nil {
		t.Fatal(err)
	}
	if leftover != 0 {
		t.Errorf("%d tables, sequences or functions left after down to 0; every Down must undo its Up", leftover)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("second up: %v", err)
	}

	version, err := provider.GetDBVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sources := provider.ListSources()
	if want := sources[len(sources)-1].Version; version != want {
		t.Errorf("database version = %d, want %d", version, want)
	}
}

// TestAppRole_CannotBypassTenantIsolation guards the role setup that RLS
// relies on: the API role is not a superuser, cannot bypass RLS, does not own
// the schema and cannot create tables.
func TestAppRole_CannotBypassTenantIsolation(t *testing.T) {
	t.Parallel()
	app := env.Open(t, pgtest.App)
	ctx := t.Context()

	var superuser, bypassRLS bool
	err := app.QueryRowContext(ctx,
		`SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user`).Scan(&superuser, &bypassRLS)
	if err != nil {
		t.Fatalf("read role: %v", err)
	}
	if superuser || bypassRLS {
		t.Errorf("encomendo_app: superuser=%v bypassrls=%v, want both false", superuser, bypassRLS)
	}

	if _, err := app.ExecContext(ctx, `CREATE TABLE should_fail (id int)`); err == nil {
		t.Error("encomendo_app created a table; it must not have CREATE on the schema")
	}
	// A temporary table could shadow a table a function names without a
	// schema (deploy/postgres/database.sqlinc revokes TEMP with the rest).
	var temp bool
	if err := app.QueryRowContext(ctx, `SELECT has_database_privilege(current_database(), 'TEMP')`).Scan(&temp); err != nil {
		t.Fatal(err)
	}
	if temp {
		t.Error("encomendo_app may create temporary tables")
	}
}

// TestOpen_ConnectsAsAppRole checks the adapter the API uses: pool settings
// apply, the statement timeout is set on every connection, and Ping works.
func TestOpen_ConnectsAsAppRole(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db, err := postgres.Open(ctx, config.Database{
		URL:                env.DSN(pgtest.App),
		MaxOpenConns:       4,
		MaxIdleConns:       2,
		ConnMaxLifetime:    time.Minute,
		StatementTimeout:   1500 * time.Millisecond,
		SlowQueryThreshold: time.Second,
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := db.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	var timeout, user string
	if err := db.Gorm().WithContext(ctx).Raw("SELECT current_setting('statement_timeout'), current_user").
		Row().Scan(&timeout, &user); err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if timeout != "1500ms" || user != pgtest.App.User() {
		t.Errorf("statement_timeout = %q as %q, want 1500ms as %s", timeout, user, pgtest.App.User())
	}
}

func TestOpen_FailsFastOnBadCredentials(t *testing.T) {
	t.Parallel()
	cfg := config.Database{
		URL:          env.DSN(pgtest.App.WithPassword("wrong")),
		MaxOpenConns: 1, MaxIdleConns: 1, StatementTimeout: time.Second,
	}
	if _, err := postgres.Open(t.Context(), cfg, slog.New(slog.DiscardHandler)); err == nil {
		t.Fatal("Open succeeded with a wrong password")
	}
}
