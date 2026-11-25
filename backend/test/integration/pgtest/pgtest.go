//go:build integration

// Package pgtest starts Postgres for the integration tests: the image CI
// uses, the init scripts of deploy/postgres (so the roles and grants are the
// real ones), and the migrations applied as encomendo_owner. Tests connect as
// encomendo_app to see what the API sees, and as the superuser to seed and
// check rows behind RLS.
package pgtest

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres/migrations"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Role is a database login the tests can use.
type Role struct{ user, password string }

// The roles of deploy/postgres/init.sql. The passwords are the public
// development ones.
var (
	// Superuser bypasses RLS. Tests use it only to seed and to check rows.
	Superuser = Role{"postgres", "postgres"}
	// Owner owns the tables and runs the migrations.
	Owner = Role{"encomendo_owner", "encomendo_owner_dev"}
	// App is the API's role: no superuser, no BYPASSRLS, no DDL.
	App = Role{"encomendo_app", "encomendo_app_dev"}
)

// User is the role's login name.
func (r Role) User() string { return r.user }

// WithPassword is r with another password, to test a failed login.
func (r Role) WithPassword(p string) Role { return Role{r.user, p} }

const database = "encomendo"

// Env is one running Postgres.
type Env struct {
	ctr        *tcpostgres.PostgresContainer
	host, port string
}

// Start runs a Postgres container with the deploy init scripts. It does not
// migrate; call Migrate for that.
func Start(ctx context.Context) (*Env, error) {
	dir, err := deployDir()
	if err != nil {
		return nil, err
	}
	// init.sql includes database.sqlinc (\ir), so both files go to the container.
	ctr, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("postgres"),
		tcpostgres.WithUsername(Superuser.user),
		tcpostgres.WithPassword(Superuser.password),
		tcpostgres.WithInitScripts(filepath.Join(dir, "init.sql"), filepath.Join(dir, "database.sqlinc")),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		if ctr != nil {
			_ = testcontainers.TerminateContainer(ctr) // best effort; the start error is the one to report
		}
		return nil, fmt.Errorf("start postgres: %w", err)
	}
	env := &Env{ctr: ctr}
	if env.host, err = ctr.Host(ctx); err != nil {
		env.Close()
		return nil, fmt.Errorf("container host: %w", err)
	}
	mapped, err := ctr.MappedPort(ctx, "5432/tcp")
	if err != nil {
		env.Close()
		return nil, fmt.Errorf("container port: %w", err)
	}
	env.port = mapped.Port()
	return env, nil
}

// StartForTest is Start for one test, stopped when the test ends.
func StartForTest(t *testing.T) *Env {
	t.Helper()
	env, err := Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(env.Close)
	return env
}

// Run is a TestMain body: it starts Postgres, applies every migration,
// stores the environment in *env, runs the tests and stops the container.
func Run(m *testing.M, env **Env) int {
	ctx := context.Background()
	e, err := Start(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer e.Close()
	if err := e.Migrate(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	*env = e
	return m.Run()
}

// Close stops the container.
func (e *Env) Close() {
	_ = testcontainers.TerminateContainer(e.ctr) // the reaper removes it if this fails
}

// DSN is the connection URL for role r.
func (e *Env) DSN(r Role) string {
	return (&url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(r.user, r.password),
		Host:     e.host + ":" + e.port,
		Path:     database,
		RawQuery: "sslmode=disable",
	}).String()
}

// Open returns a pool connected as r. The pool is closed when t ends.
func (e *Env) Open(t testing.TB, r Role) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", e.DSN(r))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// Provider returns a goose provider over the embedded migrations, connected
// as encomendo_owner like cmd/migrate.
func (e *Env) Provider(t testing.TB) *goose.Provider {
	t.Helper()
	p, err := goose.NewProvider(goose.DialectPostgres, e.Open(t, Owner), migrations.FS)
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	return p
}

// Migrate applies every migration as encomendo_owner.
func (e *Env) Migrate(ctx context.Context) error {
	db, err := sql.Open("pgx", e.DSN(Owner))
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

// NewID returns a fresh random UUIDv7, so tests sharing one database never
// collide.
func NewID() shared.ID {
	var r [10]byte
	_, _ = rand.Read(r[:]) // crypto/rand.Read never fails (Go 1.24+)
	return shared.NewV7(time.Now().UnixMilli(), r)
}

func deployDir() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("locate pgtest source")
	}
	return filepath.Abs(filepath.Join(filepath.Dir(file), "../../../../deploy/postgres"))
}
