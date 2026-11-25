// Command api is the Encomendo HTTP API and the composition root: the only
// place that knows every concrete type. It reads config, opens Postgres and
// Redis, builds the adapters and services, and serves until SIGINT/SIGTERM.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	apihttp "github.com/bergsantana/encomendo/backend/internal/adapter/http"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler"
	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres"
	"github.com/bergsantana/encomendo/backend/internal/adapter/redis"
	"github.com/bergsantana/encomendo/backend/internal/platform/config"
	"github.com/bergsantana/encomendo/backend/internal/platform/logger"
)

// readinessTimeout bounds each /readyz dependency check.
const readinessTimeout = 2 * time.Second

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "api:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	log := logger.New(os.Stdout, cfg.Log, "api")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := postgres.Open(ctx, cfg.Database, log)
	if err != nil {
		return err
	}
	defer closeAndLog(log, "postgres", db.Close)

	cache, err := redis.Open(cfg.Redis)
	if err != nil {
		return err
	}
	defer closeAndLog(log, "redis", cache.Close)

	health := handler.NewHealth(log, readinessTimeout,
		handler.Check{Name: "postgres", Run: db.Ping},
		handler.Check{Name: "redis", Run: cache.Ping},
	)
	router := apihttp.NewRouter(log, apihttp.Handlers{Health: health}, apihttp.RouterOptions{HSTS: cfg.HTTP.HSTS})

	log.Info("starting api", "env", cfg.Env)
	return apihttp.NewServer(cfg.HTTP, router, log).Run(ctx)
}

func closeAndLog(log *slog.Logger, name string, closeFn func() error) {
	if err := closeFn(); err != nil {
		log.Error("close failed", "resource", name, "err", err)
	}
}
