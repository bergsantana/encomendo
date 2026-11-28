package config

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
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
	Auth     Auth
	Origins  Origins
	SMTP     SMTP
	Storage  Storage
	// TrustedProxies are the proxies whose X-Forwarded-For the API reads
	// (ADR 0021): exact addresses in production, the private ranges by
	// default in development and test.
	TrustedProxies []netip.Prefix
}

// IsLocal reports whether the process runs in development or test, where
// the fixed development keys and plain-HTTP settings are allowed.
func (e Environment) IsLocal() bool { return e == EnvDevelopment || e == EnvTest }

// Auth configures staff sessions (ADRs 0010, 0011 and 0019).
type Auth struct {
	// SigningSeed signs access tokens; VerifySeeds are older keys still
	// accepted during a rotation. adapter/auth.LoadKeys parses them and
	// refuses the development key outside development.
	SigningSeed string
	VerifySeeds []string
	// Issuer is the iss of every access token.
	Issuer string
	// ResetCodeKey is the HMAC key reset codes are stored under, at least
	// 32 bytes. The rate limits' pseudonym key is derived from it.
	ResetCodeKey []byte
	// RefreshCookie is the refresh cookie's name: __Host-encomendo_refresh,
	// or encomendo_refresh on a local stack over plain HTTP.
	RefreshCookie string
}

// Origins names where the apps live.
type Origins struct {
	// Admin is the admin app's origin, as browsers send it in Origin:
	// the cross-origin check compares against it and invitation links
	// point at it.
	Admin string
	// APIHost is the API's host name, if it has one of its own; its first
	// label is reserved as a store address.
	APIHost string
	// StoreTemplate is the origin of a store's page with {slug} where the
	// store's address goes, such as https://{slug}.encomendo.com.br. Tracking
	// links are built from it (ADR 0030).
	StoreTemplate string
}

// SMTP configures the mail of invitations and password reset codes.
type SMTP struct {
	Addr, From, FromName string
	Username, Password   string
	Timeout              time.Duration
	// RequireTLS refuses a server without STARTTLS; off only for a local
	// mail catcher (ADR 0011).
	RequireTLS bool
}

// The names of the refresh cookie (ADR 0010).
const (
	HostRefreshCookie  = "__Host-encomendo_refresh"
	PlainRefreshCookie = "encomendo_refresh"
)

// DevResetCodeKey is the fixed reset-code key of the local and CI stacks.
// It is in the repository, so Load refuses it outside development and
// test.
const DevResetCodeKey = "AxEa9VTpecLRcumgvGFhpYTkZR2pF4jzai/nIen2Dtk="

// minResetCodeKey is the shortest reset-code key accepted (ADR 0011).
const minResetCodeKey = 32

// DefaultTrustedProxies are the private and loopback ranges, where the
// compose networks live. They are the default in development and test only
// (ADR 0021).
func DefaultTrustedProxies() []netip.Prefix {
	return []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("172.16.0.0/12"),
		netip.MustParsePrefix("192.168.0.0/16"),
		netip.MustParsePrefix("127.0.0.0/8"),
		netip.MustParsePrefix("fc00::/7"),
		netip.MustParsePrefix("::1/128"),
	}
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
		Redis:          Redis{URL: r.required("REDIS_URL")},
		Auth:           r.auth(env),
		Origins:        r.origins(env),
		SMTP:           r.smtp(env),
		Storage:        r.storage(env),
		TrustedProxies: r.proxies(env),
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

func (r *reader) auth(env Environment) Auth {
	a := Auth{
		SigningSeed:   r.required("AUTH_SIGNING_SEED"),
		VerifySeeds:   r.list("AUTH_VERIFY_SEEDS"),
		Issuer:        r.str("AUTH_ISSUER", ""),
		RefreshCookie: r.str("AUTH_REFRESH_COOKIE", HostRefreshCookie),
	}
	if a.Issuer == "" {
		r.check(env.IsLocal(), "AUTH_ISSUER is required outside development")
		a.Issuer = "encomendo-" + string(env)
	}
	if raw := r.required("AUTH_RESET_CODE_KEY"); raw != "" {
		a.ResetCodeKey = r.resetCodeKey(env, raw)
	}
	r.check(a.RefreshCookie == HostRefreshCookie || a.RefreshCookie == PlainRefreshCookie,
		"AUTH_REFRESH_COOKIE must be %s or %s, got %q", HostRefreshCookie, PlainRefreshCookie, a.RefreshCookie)
	r.check(env.IsLocal() || a.RefreshCookie != PlainRefreshCookie,
		"AUTH_REFRESH_COOKIE %s is for a local stack over plain HTTP only (ADR 0010)", PlainRefreshCookie)
	return a
}

// resetCodeKey decodes the reset-code key strictly and in its one canonical
// spelling, and refuses the development key by its bytes outside
// development: base64 has other spellings of the same bytes, so comparing
// the text would let one through (ADR 0011).
func (r *reader) resetCodeKey(env Environment, raw string) []byte {
	key, err := base64.StdEncoding.Strict().DecodeString(raw)
	if err != nil || base64.StdEncoding.EncodeToString(key) != raw || len(key) < minResetCodeKey {
		r.check(false, "AUTH_RESET_CODE_KEY must be at least %d bytes in standard base64 (openssl rand -base64 32)", minResetCodeKey)
		return nil
	}
	dev, _ := base64.StdEncoding.DecodeString(DevResetCodeKey)
	r.check(env.IsLocal() || !bytes.Equal(key, dev), "AUTH_RESET_CODE_KEY is the development key; set one of your own")
	return key
}

func (r *reader) origins(env Environment) Origins {
	o := Origins{
		Admin: r.required("ADMIN_ORIGIN"), APIHost: r.str("API_HOST", ""),
		StoreTemplate: r.required("STORE_ORIGIN_TEMPLATE"),
	}
	r.check(env.IsLocal() || strings.HasPrefix(o.Admin, "https://"),
		"ADMIN_ORIGIN must be https outside development, got %q", o.Admin)
	if o.StoreTemplate != "" {
		r.check(strings.Count(o.StoreTemplate, "{slug}") == 1,
			"STORE_ORIGIN_TEMPLATE must contain {slug} once, got %q", o.StoreTemplate)
		r.check(env.IsLocal() || strings.HasPrefix(o.StoreTemplate, "https://"),
			"STORE_ORIGIN_TEMPLATE must be https outside development, got %q", o.StoreTemplate)
	}
	return o
}

// HostLabels returns the first label of the admin and API hosts, which no
// store may take as its address.
func (o Origins) HostLabels() []string {
	var labels []string
	for _, host := range []string{o.Admin, o.APIHost} {
		host = strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
		host, _, _ = strings.Cut(host, ":")
		if label, _, _ := strings.Cut(host, "."); label != "" {
			labels = append(labels, label)
		}
	}
	return labels
}

func (r *reader) smtp(env Environment) SMTP {
	s := SMTP{
		Addr: r.required("SMTP_ADDR"), From: r.required("SMTP_FROM"), FromName: r.str("SMTP_FROM_NAME", "Encomendo"),
		Username: r.str("SMTP_USERNAME", ""), Password: r.str("SMTP_PASSWORD", ""),
		Timeout:    r.duration("SMTP_TIMEOUT", 10*time.Second),
		RequireTLS: r.boolean("SMTP_REQUIRE_TLS", true),
	}
	r.check(env.IsLocal() || s.RequireTLS, "SMTP_REQUIRE_TLS=false is for a local mail catcher only (ADR 0011)")
	return s
}

func (r *reader) proxies(env Environment) []netip.Prefix {
	entries := r.list("TRUSTED_PROXIES")
	if len(entries) == 0 {
		r.check(env.IsLocal(), "TRUSTED_PROXIES is required outside development: the exact addresses of Caddy and nginx (ADR 0021)")
		return DefaultTrustedProxies()
	}
	var out []netip.Prefix
	for _, e := range entries {
		p, err := netip.ParsePrefix(e)
		if err != nil {
			if addr, aerr := netip.ParseAddr(e); aerr == nil {
				p, err = addr.Prefix(addr.BitLen())
			}
		}
		if err != nil {
			r.check(false, "TRUSTED_PROXIES entry %q is not an address or a CIDR", e)
			continue
		}
		// An IPv4-mapped prefix never matches, because addresses are
		// unmapped before the comparison: the proxy would count as the
		// client.
		r.check(!p.Addr().Is4In6(), "TRUSTED_PROXIES entry %q is IPv4-mapped; write it as IPv4", e)
		// 172.30.0.10/24 is a typo, not a wish to trust the whole network.
		r.check(p == p.Masked(), "TRUSTED_PROXIES entry %q has host bits set; write the network or a single address", e)
		// Production names its proxies exactly (ADR 0021): a range could
		// take in an address that is not a proxy.
		r.check(env.IsLocal() || p.IsSingleIP(), "TRUSTED_PROXIES entry %q is a range; name each proxy's address outside development", e)
		out = append(out, p)
	}
	return out
}

func (r *reader) list(key string) []string {
	var out []string
	for _, part := range strings.Split(r.getenv(key), ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func (r *reader) boolean(key string, fallback bool) bool {
	raw := strings.TrimSpace(r.getenv(key))
	if raw == "" {
		return fallback
	}
	b, err := strconv.ParseBool(raw)
	r.check(err == nil, "%s must be true or false, got %q", key, raw)
	if err != nil {
		return fallback
	}
	return b
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
