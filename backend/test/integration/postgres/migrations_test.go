//go:build integration

// Package postgres_test runs the migrations against a real Postgres started
// with the same init script as docker compose, and checks the role setup the
// tenant isolation depends on.
package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres"
	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres/migrations"
	"github.com/bergsantana/encomendo/backend/internal/platform/config"
)

// Credentials from deploy/postgres/init.sql (local and CI only).
const (
	ownerUser, ownerPassword = "encomendo_owner", "encomendo_owner_dev"
	appUser, appPassword     = "encomendo_app", "encomendo_app_dev"
	database                 = "encomendo"
)

func startPostgres(t *testing.T) (host, port string) {
	t.Helper()
	ctx := context.Background()

	initScript, err := filepath.Abs("../../../../deploy/postgres/init.sql")
	if err != nil {
		t.Fatal(err)
	}
	ctr, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("postgres"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		tcpostgres.WithInitScripts(initScript),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second)),
	)
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}

	host, err = ctr.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mapped, err := ctr.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatal(err)
	}
	return host, mapped.Port()
}

func connect(t *testing.T, host, port, user, password string) *sql.DB {
	t.Helper()
	dsn := (&url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(user, password),
		Host:     fmt.Sprintf("%s:%s", host, port),
		Path:     database,
		RawQuery: "sslmode=disable",
	}).String()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestMigrations_UpDownUp(t *testing.T) {
	host, port := startPostgres(t)
	owner := connect(t, host, port, ownerUser, ownerPassword)
	ctx := context.Background()

	provider, err := goose.NewProvider(goose.DialectPostgres, owner, migrations.FS)
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("first up: %v", err)
	}
	if _, err := provider.DownTo(ctx, 0); err != nil {
		t.Fatalf("down to 0: %v", err)
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
	host, port := startPostgres(t)
	owner := connect(t, host, port, ownerUser, ownerPassword)
	ctx := context.Background()

	provider, err := goose.NewProvider(goose.DialectPostgres, owner, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("up: %v", err)
	}

	app := connect(t, host, port, appUser, appPassword)
	var superuser, bypassRLS bool
	err = app.QueryRowContext(ctx,
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
}

// TestOpen_ConnectsAsAppRole checks the adapter the API uses: pool settings
// apply, the statement timeout is set on every connection, and Ping works.
func TestOpen_ConnectsAsAppRole(t *testing.T) {
	host, port := startPostgres(t)
	ctx := context.Background()

	cfg := config.Database{
		URL:                fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", appUser, appPassword, host, port, database),
		MaxOpenConns:       4,
		MaxIdleConns:       2,
		ConnMaxLifetime:    time.Minute,
		StatementTimeout:   1500 * time.Millisecond,
		SlowQueryThreshold: time.Second,
	}
	db, err := postgres.Open(ctx, cfg, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := db.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	var timeout string
	if err := db.Gorm().WithContext(ctx).Raw("SHOW statement_timeout").Scan(&timeout).Error; err != nil {
		t.Fatalf("read statement_timeout: %v", err)
	}
	if timeout != "1500ms" {
		t.Errorf("statement_timeout = %q, want 1500ms", timeout)
	}
}

func TestOpen_FailsFastOnBadCredentials(t *testing.T) {
	host, port := startPostgres(t)

	cfg := config.Database{
		URL:          fmt.Sprintf("postgres://%s:wrong@%s:%s/%s?sslmode=disable", appUser, host, port, database),
		MaxOpenConns: 1, MaxIdleConns: 1, StatementTimeout: time.Second,
	}
	if _, err := postgres.Open(context.Background(), cfg, slog.New(slog.DiscardHandler)); err == nil {
		t.Fatal("Open succeeded with a wrong password")
	}
}
