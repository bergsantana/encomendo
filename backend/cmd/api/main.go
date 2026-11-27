// Command api is the Encomendo HTTP API and the composition root: the only
// place that knows every concrete type. It reads config, opens Postgres and
// Redis, builds the adapters and services, and serves until SIGINT/SIGTERM.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/auth"
	apihttp "github.com/bergsantana/encomendo/backend/internal/adapter/http"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/middleware"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/ratelimit"
	"github.com/bergsantana/encomendo/backend/internal/adapter/inproc"
	"github.com/bergsantana/encomendo/backend/internal/adapter/notify"
	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres"
	"github.com/bergsantana/encomendo/backend/internal/adapter/redis"
	"github.com/bergsantana/encomendo/backend/internal/adapter/storage"
	"github.com/bergsantana/encomendo/backend/internal/adapter/system"
	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	appsupplier "github.com/bergsantana/encomendo/backend/internal/app/supplier"
	apptenancy "github.com/bergsantana/encomendo/backend/internal/app/tenancy"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
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
	if err := apihttp.CheckTimeouts(cfg.HTTP); err != nil {
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

	mail, err := notify.NewSMTP(notify.SMTPConfig{
		Addr: cfg.SMTP.Addr, From: cfg.SMTP.From, FromName: cfg.SMTP.FromName,
		Username: cfg.SMTP.Username, Password: cfg.SMTP.Password, AdminOrigin: cfg.Origins.Admin,
		Timeout: cfg.SMTP.Timeout, RequireTLS: cfg.SMTP.RequireTLS,
	}, log)
	if err != nil {
		return err
	}
	// Reset codes go out off the request path: wait for them, or a code
	// in flight is lost at shutdown (ADR 0011).
	defer mail.Wait()

	router, err := buildRouter(cfg, log, wiring{db: db, cache: cache, mail: mail})
	if err != nil {
		return err
	}
	log.Info("starting api", "env", cfg.Env)
	return apihttp.NewServer(cfg.HTTP, router, log).Run(ctx)
}

// wiring is what the router's dependencies are built on.
type wiring struct {
	db    *postgres.DB
	cache *redis.Client
	mail  *notify.SMTP
}

// services are the use cases and the adapters the handlers stand on.
type services struct {
	identity   *appidentity.Service
	tenancy    *apptenancy.Service
	catalog    *appcatalog.Service
	suppliers  *appsupplier.Service
	tokens     *auth.AccessTokens
	limiter    ratelimit.Limiter
	pseudonyms ratelimit.Pseudonyms
}

func buildRouter(cfg config.Config, log *slog.Logger, w wiring) (http.Handler, error) {
	svc, err := buildServices(cfg, log, w)
	if err != nil {
		return nil, err
	}
	svc.pseudonyms, err = ratelimit.NewPseudonyms(ratelimit.DerivePseudonymKey(cfg.Auth.ResetCodeKey))
	if err != nil {
		return nil, err
	}
	svc.limiter = redis.NewRateLimiter(w.cache, system.NewClock(), log)
	h, err := buildHandlers(cfg, log, svc)
	if err != nil {
		return nil, err
	}
	h.Health = handler.NewHealth(log, readinessTimeout,
		handler.Check{Name: "postgres", Run: w.db.Ping},
		handler.Check{Name: "redis", Run: w.cache.Ping},
	)
	guard, err := middleware.CrossOrigin(cfg.Origins.Admin, log)
	if err != nil {
		return nil, err
	}
	limits := middleware.NewRateLimits(svc.limiter, ratelimit.NewClientIP(cfg.TrustedProxies), svc.pseudonyms, log)
	if !cfg.Env.IsLocal() {
		limits.WarnOnPrivateClients()
	}
	return apihttp.NewRouter(log, h, apihttp.RouterOptions{
		HSTS: cfg.HTTP.HSTS, AuthGuard: guard, RateLimits: limits,
		Authn:  middleware.NewAuthn(svc.tokens, redis.NewDenylist(w.cache, log), log),
		Stores: svc.tenancy,
	})
}

func buildHandlers(cfg config.Config, log *slog.Logger, svc services) (apihttp.Handlers, error) {
	cookie, err := handler.NewRefreshCookie(cfg.Auth.RefreshCookie)
	if err != nil {
		return apihttp.Handlers{}, err
	}
	authH, err := handler.NewAuth(handler.AuthDeps{
		UseCases: svc.identity, Verifier: svc.tokens, Limiter: svc.limiter, Pseudonyms: svc.pseudonyms, Cookie: cookie, Log: log,
	})
	if err != nil {
		return apihttp.Handlers{}, err
	}
	tenancyH, err := handler.NewTenancy(handler.TenancyDeps{UseCases: svc.tenancy, Limiter: svc.limiter, Pseudonyms: svc.pseudonyms, Log: log})
	if err != nil {
		return apihttp.Handlers{}, err
	}
	catalogH, err := handler.NewCatalog(handler.CatalogDeps{UseCases: svc.catalog, Limiter: svc.limiter, Log: log})
	if err != nil {
		return apihttp.Handlers{}, err
	}
	suppliersH, err := handler.NewSuppliers(handler.SuppliersDeps{UseCases: svc.suppliers, Log: log})
	if err != nil {
		return apihttp.Handlers{}, err
	}
	return apihttp.Handlers{Auth: authH, Tenancy: tenancyH, Catalog: catalogH, Suppliers: suppliersH}, nil
}

func buildServices(cfg config.Config, log *slog.Logger, w wiring) (services, error) {
	keys, err := auth.LoadKeys(cfg.Auth.SigningSeed, cfg.Auth.VerifySeeds, cfg.Env.IsLocal())
	if err != nil {
		return services{}, err
	}
	// The kids are safe to show, and tell which keys an instance runs with
	// during a rotation (ADR 0019).
	log.Info("access token keys", "signing_kid", keys.KID(), "kids", keys.KIDs())
	clock, ids, secrets := system.NewClock(), system.NewIDs(), auth.NewSecrets()
	tokens, err := auth.NewAccessTokens(keys, cfg.Auth.Issuer, clock)
	if err != nil {
		return services{}, err
	}
	hasher, err := auth.NewHasher(auth.DefaultArgon2Params(), auth.DefaultMaxConcurrentHashes, auth.DefaultMaxHashWait)
	if err != nil {
		return services{}, err
	}
	tx := postgres.NewTxManager(w.db)
	users := postgres.NewUsers()
	tenancyDeps := apptenancy.Deps{
		Tenants: postgres.NewTenants(), Members: postgres.NewMembers(), Invitations: postgres.NewInvitations(),
		Users: inproc.NewTenancyUsers(appidentity.NewDirectory(users)), Workflows: postgres.NewWorkflowPresets(),
		Mailer: w.mail, Tx: tx, Clock: clock, IDs: ids, Secrets: secrets, Events: postgres.NewOutbox(),
		Slugs: tenancy.NewSlugPolicy(cfg.Origins.HostLabels()...),
	}
	lookup := apptenancy.NewLookup(tenancyDeps)
	identity, err := appidentity.NewService(appidentity.Deps{
		Users: users, Refresh: postgres.NewRefreshTokens(), ResetCodes: postgres.NewResetCodes(),
		Memberships: inproc.NewIdentityMemberships(lookup), Invitations: inproc.NewIdentityInvitations(lookup),
		Hasher: hasher, Tokens: tokens, Denylist: redis.NewDenylist(w.cache, log), Mailer: w.mail,
		Tx: tx, Clock: clock, IDs: ids, Secrets: secrets, ResetCodeKey: cfg.Auth.ResetCodeKey,
	})
	if err != nil {
		return services{}, err
	}
	tenancySvc := apptenancy.NewService(tenancyDeps)
	suppliers, catalog, err := buildCatalog(cfg.Storage, basePorts{tx: tx, ids: ids, cache: redis.NewCache(w.cache, log)}, tenancySvc, log)
	if err != nil {
		return services{}, err
	}
	return services{identity: identity, tenancy: tenancySvc, catalog: catalog, suppliers: suppliers, tokens: tokens}, nil
}

// basePorts are the adapters every context's service is built on.
type basePorts struct {
	tx    appshared.TxManager
	ids   appshared.IDGenerator
	cache appshared.Cache
}

// buildCatalog wires the supplier and catalog contexts. The catalog reads
// supplier names and the store's defaults through inproc ports, so the
// dependency runs one way, catalog to supplier and tenancy (ADR 0025).
func buildCatalog(
	store config.Storage, base basePorts, tenancySvc *apptenancy.Service, log *slog.Logger,
) (*appsupplier.Service, *appcatalog.Service, error) {
	photos, err := newPhotoStore(store, log)
	if err != nil {
		return nil, nil, err
	}
	suppliers := appsupplier.NewService(appsupplier.Deps{Suppliers: postgres.NewSuppliers(), Tx: base.tx, IDs: base.ids})
	catalog := appcatalog.NewService(appcatalog.Deps{
		Categories: postgres.NewCategories(), Attributes: postgres.NewAttributes(), Items: postgres.NewItems(),
		Kits: postgres.NewKits(), Photos: postgres.NewPhotos(), Storage: photos,
		Suppliers: inproc.NewCatalogSuppliers(suppliers), Defaults: inproc.NewCatalogDefaults(tenancySvc),
		Cache: base.cache, Tx: base.tx, IDs: base.ids,
	})
	return suppliers, catalog, nil
}

// newPhotoStore opens the photo bucket's client. The secret goes straight
// from the configuration's Secret into the client. Neither the
// configuration nor storage.Config is logged or printed: both redact the
// secret, but the client is the only place that needs its value.
func newPhotoStore(c config.Storage, log *slog.Logger) (*storage.S3, error) {
	return storage.New(storage.Config{
		Endpoint: c.Endpoint, PublicEndpoint: c.PublicEndpoint, Bucket: c.Bucket, Region: c.Region,
		AccessKey: c.AccessKey, SecretKey: c.SecretKey.Value(), UseSSL: c.UseSSL,
	}, log)
}

func closeAndLog(log *slog.Logger, name string, closeFn func() error) {
	if err := closeFn(); err != nil {
		log.Error("close failed", "resource", name, "err", err)
	}
}
