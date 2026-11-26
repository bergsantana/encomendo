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
		"DATABASE_URL":        "postgres://encomendo_app@localhost:5432/encomendo",
		"REDIS_URL":           "redis://localhost:6379/0",
		"AUTH_SIGNING_SEED":   "a-seed-adapter-auth-parses",
		"AUTH_RESET_CODE_KEY": DevResetCodeKey,
		"ADMIN_ORIGIN":        "http://admin.localhost:8080",
		"SMTP_ADDR":           "mailpit:1025",
		"SMTP_FROM":           "nao-responda@encomendo.com.br",
	}
}

// productionEnv is a production configuration with every safe setting.
func productionEnv() map[string]string {
	e := minimalEnv()
	e["APP_ENV"] = "production"
	e["AUTH_ISSUER"] = "https://api.encomendo.com.br"
	e["AUTH_RESET_CODE_KEY"] = "q0Fcl2nSQ0n6X3zW1T8Vq0Fcl2nSQ0n6X3zW1T8Vq0E="
	e["ADMIN_ORIGIN"] = "https://admin.encomendo.com.br"
	e["API_HOST"] = "api.encomendo.com.br"
	e["TRUSTED_PROXIES"] = "172.30.0.10,172.30.0.11"
	return e
}

func TestLoad_ProductionWithSafeSettings(t *testing.T) {
	t.Parallel()

	cfg, err := Load(env(productionEnv()))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Auth.RefreshCookie != HostRefreshCookie || !cfg.SMTP.RequireTLS || len(cfg.Auth.ResetCodeKey) < 32 {
		t.Errorf("defaults = %+v %+v", cfg.Auth, cfg.SMTP)
	}
	if len(cfg.TrustedProxies) != 2 || cfg.TrustedProxies[0].String() != "172.30.0.10/32" {
		t.Errorf("TrustedProxies = %v", cfg.TrustedProxies)
	}
	if got := strings.Join(cfg.Origins.HostLabels(), ","); got != "admin,api" {
		t.Errorf("HostLabels = %q", got)
	}
}

// Outside development and test, every setting that is safe only on a local
// stack is refused (ADRs 0010, 0011, 0019 and 0021).
func TestLoad_ProductionRefusesLocalSettings(t *testing.T) {
	t.Parallel()

	tests := map[string]struct{ key, value, want string }{
		"the plain cookie":        {"AUTH_REFRESH_COOKIE", PlainRefreshCookie, "AUTH_REFRESH_COOKIE"},
		"mail without TLS":        {"SMTP_REQUIRE_TLS", "false", "SMTP_REQUIRE_TLS"},
		"an http admin origin":    {"ADMIN_ORIGIN", "http://admin.encomendo.com.br", "ADMIN_ORIGIN"},
		"the dev reset-code key":  {"AUTH_RESET_CODE_KEY", DevResetCodeKey, "development key"},
		"no issuer":               {"AUTH_ISSUER", "", "AUTH_ISSUER"},
		"no trusted proxies":      {"TRUSTED_PROXIES", "", "TRUSTED_PROXIES"},
		"no signing seed":         {"AUTH_SIGNING_SEED", "", "AUTH_SIGNING_SEED"},
		"an IPv4-mapped proxy":    {"TRUSTED_PROXIES", "::ffff:10.0.0.0/104", "IPv4-mapped"},
		"a proxy that is no addr": {"TRUSTED_PROXIES", "caddy", "not an address"},
		// base64 ignores unused trailing bits: the same bytes, spelled
		// otherwise, must not pass.
		"the dev key respelled":  {"AUTH_RESET_CODE_KEY", strings.TrimSuffix(DevResetCodeKey, "k=") + "l=", "AUTH_RESET_CODE_KEY"},
		"the dev key split":      {"AUTH_RESET_CODE_KEY", DevResetCodeKey[:20] + "\n" + DevResetCodeKey[20:], "AUTH_RESET_CODE_KEY"},
		"an ftp admin origin":    {"ADMIN_ORIGIN", "ftp://admin.encomendo.com.br", "ADMIN_ORIGIN"},
		"a proxy range":          {"TRUSTED_PROXIES", "172.30.0.0/24", "is a range"},
		"every address":          {"TRUSTED_PROXIES", "0.0.0.0/0", "is a range"},
		"a proxy with host bits": {"TRUSTED_PROXIES", "172.30.0.10/24", "host bits"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			e := productionEnv()
			e[tt.key] = tt.value
			if _, err := Load(env(e)); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Load error = %v, want one about %s", err, tt.want)
			}
		})
	}
}

func TestLoad_LocalDefaults(t *testing.T) {
	t.Parallel()

	e := minimalEnv()
	e["AUTH_REFRESH_COOKIE"] = PlainRefreshCookie
	e["SMTP_REQUIRE_TLS"] = "false"
	cfg, err := Load(env(e))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.TrustedProxies) != len(DefaultTrustedProxies()) || cfg.Auth.Issuer == "" || cfg.SMTP.FromName != "Encomendo" {
		t.Errorf("local defaults = %v %q %q", cfg.TrustedProxies, cfg.Auth.Issuer, cfg.SMTP.FromName)
	}
}

func TestLoad_RefusesAShortResetCodeKey(t *testing.T) {
	t.Parallel()

	e := minimalEnv()
	e["AUTH_RESET_CODE_KEY"] = "c2hvcnQ="
	if _, err := Load(env(e)); err == nil || !strings.Contains(err.Error(), "AUTH_RESET_CODE_KEY") {
		t.Errorf("Load error = %v", err)
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

	values := productionEnv()
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
