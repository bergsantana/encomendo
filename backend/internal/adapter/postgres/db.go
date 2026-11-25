package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/bergsantana/encomendo/backend/internal/platform/config"
)

// pingTimeout bounds the connection check at startup.
const pingTimeout = 5 * time.Second

// DB is the application's database handle, connected as encomendo_app.
type DB struct {
	gorm *gorm.DB
	sql  *sql.DB
}

// Open connects to Postgres with the pgx driver and returns a GORM handle set
// up for this codebase: no default transactions (the TxManager owns them), a
// server-side statement timeout, a bounded pool and a slog-backed logger.
func Open(ctx context.Context, cfg config.Database, log *slog.Logger) (*DB, error) {
	connCfg, err := pgx.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	// Set on every connection, so a runaway query cannot hold a pooled
	// connection longer than the timeout.
	connCfg.RuntimeParams["statement_timeout"] = strconv.FormatInt(cfg.StatementTimeout.Milliseconds(), 10)
	connCfg.RuntimeParams["application_name"] = "encomendo-api"

	sqlDB := stdlib.OpenDB(*connCfg)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	gormDB, err := gorm.Open(gormpostgres.New(gormpostgres.Config{Conn: sqlDB}), &gorm.Config{
		SkipDefaultTransaction: true,
		DisableAutomaticPing:   true, // pinged below, with a timeout
		Logger:                 newGormLogger(log, cfg.SlowQueryThreshold),
	})
	if err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("open gorm: %w", err)
	}

	db := &DB{gorm: gormDB, sql: sqlDB}
	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := db.Ping(pingCtx); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return db, nil
}

// Gorm returns the pool's GORM handle, for the TxManager and for checks that
// need no scope, such as tests of the connection itself. Repositories never
// use it: they take the transaction from the context (conn.go), so no query
// runs without an RLS scope.
func (db *DB) Gorm() *gorm.DB { return db.gorm }

// Ping checks that Postgres answers. It backs the readiness probe.
func (db *DB) Ping(ctx context.Context) error {
	if err := db.sql.PingContext(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}
	return nil
}

// Close closes the connection pool.
func (db *DB) Close() error {
	if err := db.sql.Close(); err != nil {
		return fmt.Errorf("close postgres: %w", err)
	}
	return nil
}
