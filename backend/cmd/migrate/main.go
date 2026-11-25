// Command migrate applies the embedded SQL migrations as encomendo_owner.
//
// Usage:
//
//	migrate up        apply every pending migration
//	migrate down      roll back the latest migration
//	migrate reset     roll back every migration (local and CI only)
//	migrate status    list migrations and whether each is applied
//
// CI runs up, reset and up again to prove every Down section works.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
	"github.com/pressly/goose/v3"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres/migrations"
	"github.com/bergsantana/encomendo/backend/internal/platform/config"
	"github.com/bergsantana/encomendo/backend/internal/platform/logger"
)

const usage = "usage: migrate up|down|reset|status"

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) != 1 {
		return errors.New(usage)
	}
	cfg, err := config.LoadMigrate(os.Getenv)
	if err != nil {
		return err
	}
	log := logger.New(out, cfg.Log, "migrate")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	return runCommand(ctx, provider, args[0], log)
}

func runCommand(ctx context.Context, p *goose.Provider, cmd string, log *slog.Logger) error {
	var (
		results []*goose.MigrationResult
		err     error
	)
	switch cmd {
	case "up":
		results, err = p.Up(ctx)
	case "down":
		var res *goose.MigrationResult
		res, err = p.Down(ctx)
		if res != nil {
			results = append(results, res)
		}
	case "reset":
		results, err = p.DownTo(ctx, 0)
	case "status":
		return logStatus(ctx, p, log)
	default:
		return fmt.Errorf("unknown command %q; %s", cmd, usage)
	}
	for _, r := range results {
		log.Info("migration", "version", r.Source.Version, "direction", r.Direction, "duration", r.Duration)
	}
	if err != nil {
		return fmt.Errorf("migrate %s: %w", cmd, err)
	}
	log.Info("migrations done", "command", cmd, "count", len(results))
	return nil
}

func logStatus(ctx context.Context, p *goose.Provider, log *slog.Logger) error {
	statuses, err := p.Status(ctx)
	if err != nil {
		return fmt.Errorf("migration status: %w", err)
	}
	for _, s := range statuses {
		log.Info("migration", "version", s.Source.Version, "path", s.Source.Path, "state", s.State)
	}
	return nil
}
