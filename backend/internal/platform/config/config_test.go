package config

import (
	"strings"
	"testing"
	"time"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func minimalEnv() map[string]string {
	return map[string]string{
		"DATABASE_URL": "postgres://encomendo_app@localhost:5432/encomendo",
		"REDIS_URL":    "redis://localhost:6379/0",
	}
}

func TestLoad_AppliesDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := Load(env(minimalEnv()))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Env != EnvDevelopment {
		t.Errorf("Env = %q, want %q", cfg.Env, EnvDevelopment)
	}
	if cfg.HTTP.Addr != ":8080" {
		t.Errorf("HTTP.Addr = %q, want :8080", cfg.HTTP.Addr)
	}
	if cfg.HTTP.HSTS {
		t.Error("HTTP.HSTS = true outside production")
	}
	if cfg.Database.StatementTimeout != 5*time.Second {
		t.Errorf("StatementTimeout = %v, want 5s", cfg.Database.StatementTimeout)
	}
	if cfg.Log.Level != "info" || cfg.Log.Format != "json" {
		t.Errorf("Log = %+v, want info/json", cfg.Log)
	}
}

func TestLoad_ReadsOverrides(t *testing.T) {
	t.Parallel()

	values := minimalEnv()
	values["APP_ENV"] = "production"
	values["HTTP_ADDR"] = ":9000"
	values["DATABASE_MAX_OPEN_CONNS"] = "20"
	values["DATABASE_STATEMENT_TIMEOUT"] = "2s"
	values["LOG_LEVEL"] = "DEBUG"

	cfg, err := Load(env(values))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !cfg.HTTP.HSTS {
		t.Error("HTTP.HSTS = false in production")
	}
	if cfg.HTTP.Addr != ":9000" {
		t.Errorf("HTTP.Addr = %q, want :9000", cfg.HTTP.Addr)
	}
	if cfg.Database.MaxOpenConns != 20 {
		t.Errorf("MaxOpenConns = %d, want 20", cfg.Database.MaxOpenConns)
	}
	if cfg.Database.StatementTimeout != 2*time.Second {
		t.Errorf("StatementTimeout = %v, want 2s", cfg.Database.StatementTimeout)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want debug (case-insensitive)", cfg.Log.Level)
	}
}

func TestLoad_ReportsEveryProblemAtOnce(t *testing.T) {
	t.Parallel()

	values := map[string]string{
		"APP_ENV":                    "staging",
		"DATABASE_STATEMENT_TIMEOUT": "fast",
		"DATABASE_MAX_OPEN_CONNS":    "-1",
		"LOG_FORMAT":                 "xml",
	}

	_, err := Load(env(values))
	if err == nil {
		t.Fatal("Load returned no error for an invalid environment")
	}

	for _, want := range []string{
		"DATABASE_URL is required",
		"REDIS_URL is required",
		"APP_ENV must be",
		"DATABASE_STATEMENT_TIMEOUT must be a positive duration",
		"DATABASE_MAX_OPEN_CONNS must be a positive integer",
		"LOG_FORMAT must be json or text",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestLoad_RejectsMoreIdleThanOpenConnections(t *testing.T) {
	t.Parallel()

	values := minimalEnv()
	values["DATABASE_MAX_OPEN_CONNS"] = "2"
	values["DATABASE_MAX_IDLE_CONNS"] = "3"

	_, err := Load(env(values))
	if err == nil || !strings.Contains(err.Error(), "must not exceed") {
		t.Fatalf("Load error = %v, want idle/open conflict", err)
	}
}

func TestLoadMigrate_RequiresOwnerURL(t *testing.T) {
	t.Parallel()

	if _, err := LoadMigrate(env(nil)); err == nil {
		t.Fatal("LoadMigrate returned no error without MIGRATE_DATABASE_URL")
	}

	cfg, err := LoadMigrate(env(map[string]string{"MIGRATE_DATABASE_URL": "postgres://owner@db/encomendo"}))
	if err != nil {
		t.Fatalf("LoadMigrate: %v", err)
	}
	if cfg.DatabaseURL != "postgres://owner@db/encomendo" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
}
