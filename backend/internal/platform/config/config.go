package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Environment names the deployment the process runs in.
type Environment string

// Supported environments. Production turns on stricter defaults such as HSTS.
const (
	EnvDevelopment Environment = "development"
	EnvTest        Environment = "test"
	EnvProduction  Environment = "production"
)

// Config is everything the API process needs to start.
type Config struct {
	Env      Environment
	Log      Log
	HTTP     HTTP
	Database Database
	Redis    Redis
}

// Log configures the structured logger.
type Log struct {
	Level  string // debug, info, warn, error
	Format string // json or text
}

// HTTP configures the API server.
type HTTP struct {
	Addr              string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	// HSTS adds Strict-Transport-Security; on in production only, because
	// local development runs over plain HTTP.
	HSTS bool
}

// Database configures the connection pool for the encomendo_app role.
type Database struct {
	URL                string
	MaxOpenConns       int
	MaxIdleConns       int
	ConnMaxLifetime    time.Duration
	StatementTimeout   time.Duration
	SlowQueryThreshold time.Duration
}

// Redis configures the cache and rate-limit client.
type Redis struct {
	URL string
}

// Migrate is the configuration of cmd/migrate, which connects as the table
// owner (encomendo_owner) instead of the application role.
type Migrate struct {
	Log         Log
	DatabaseURL string
}

// Load reads the API configuration through getenv (os.Getenv in main, a map
// in tests). It returns every problem at once, so a broken deployment shows
// all missing or invalid settings in one error.
func Load(getenv func(string) string) (Config, error) {
	r := reader{getenv: getenv}
	env := Environment(r.str("APP_ENV", string(EnvDevelopment)))
	cfg := Config{
		Env: env,
		Log: r.log(),
		HTTP: HTTP{
			Addr:              r.str("HTTP_ADDR", ":8080"),
			ReadHeaderTimeout: r.duration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
			ReadTimeout:       r.duration("HTTP_READ_TIMEOUT", 15*time.Second),
			WriteTimeout:      r.duration("HTTP_WRITE_TIMEOUT", 30*time.Second),
			IdleTimeout:       r.duration("HTTP_IDLE_TIMEOUT", 60*time.Second),
			ShutdownTimeout:   r.duration("SHUTDOWN_TIMEOUT", 15*time.Second),
			HSTS:              env == EnvProduction,
		},
		Database: Database{
			URL:                r.required("DATABASE_URL"),
			MaxOpenConns:       r.positiveInt("DATABASE_MAX_OPEN_CONNS", 10),
			MaxIdleConns:       r.positiveInt("DATABASE_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime:    r.duration("DATABASE_CONN_MAX_LIFETIME", 30*time.Minute),
			StatementTimeout:   r.duration("DATABASE_STATEMENT_TIMEOUT", 5*time.Second),
			SlowQueryThreshold: r.duration("DATABASE_SLOW_QUERY_THRESHOLD", 200*time.Millisecond),
		},
		Redis: Redis{URL: r.required("REDIS_URL")},
	}
	r.check(env == EnvDevelopment || env == EnvTest || env == EnvProduction,
		"APP_ENV must be development, test or production, got %q", env)
	r.check(cfg.Database.MaxIdleConns <= cfg.Database.MaxOpenConns,
		"DATABASE_MAX_IDLE_CONNS (%d) must not exceed DATABASE_MAX_OPEN_CONNS (%d)",
		cfg.Database.MaxIdleConns, cfg.Database.MaxOpenConns)
	return cfg, r.err()
}

// LoadMigrate reads the configuration of cmd/migrate.
func LoadMigrate(getenv func(string) string) (Migrate, error) {
	r := reader{getenv: getenv}
	cfg := Migrate{Log: r.log(), DatabaseURL: r.required("MIGRATE_DATABASE_URL")}
	return cfg, r.err()
}

// reader collects parse errors so Load can report all of them together.
type reader struct {
	getenv func(string) string
	errs   []error
}

func (r *reader) log() Log {
	l := Log{
		Level:  strings.ToLower(r.str("LOG_LEVEL", "info")),
		Format: strings.ToLower(r.str("LOG_FORMAT", "json")),
	}
	r.check(l.Level == "debug" || l.Level == "info" || l.Level == "warn" || l.Level == "error",
		"LOG_LEVEL must be debug, info, warn or error, got %q", l.Level)
	r.check(l.Format == "json" || l.Format == "text",
		"LOG_FORMAT must be json or text, got %q", l.Format)
	return l
}

func (r *reader) str(key, fallback string) string {
	if v := strings.TrimSpace(r.getenv(key)); v != "" {
		return v
	}
	return fallback
}

func (r *reader) required(key string) string {
	v := strings.TrimSpace(r.getenv(key))
	r.check(v != "", "%s is required", key)
	return v
}

func (r *reader) duration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(r.getenv(key))
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		r.check(false, "%s must be a positive duration such as 5s, got %q", key, raw)
		return fallback
	}
	return d
}

func (r *reader) positiveInt(key string, fallback int) int {
	raw := strings.TrimSpace(r.getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		r.check(false, "%s must be a positive integer, got %q", key, raw)
		return fallback
	}
	return n
}

func (r *reader) check(ok bool, format string, args ...any) {
	if !ok {
		r.errs = append(r.errs, fmt.Errorf(format, args...))
	}
}

func (r *reader) err() error {
	if len(r.errs) == 0 {
		return nil
	}
	return fmt.Errorf("invalid configuration: %w", errors.Join(r.errs...))
}
