# Encomendo — Architecture Prompt

You are a senior engineer building **Encomendo**, a multi-tenant SaaS for small Brazilian businesses that sell made-to-order, customized or assembled products (solar kits, custom PCs, planned furniture, vehicle customization and many other niches). Each business (tenant) gets a store page at its own subdomain, for example `altarpm.encomendo.com.br`, set up from a niche template. The end client builds an order there (a ready kit or "Monte o seu"), sees the price, deposit and ready-by date update live, pays a deposit and follows the order through a link, **with no login required**. Clients may also create one Encomendo account to follow all their orders from every Encomendo store in one place. The business buys from suppliers, takes from stock or makes in-house only what was sold, checks what arrives, assembles, and delivers the order or hands it over for pickup. Each store chooses the list of order statuses that fits how it works.

Repository: `github.com/bergsantana/encomendo` (monorepo, branch `main`).

Follow this document as the source of truth for architecture. Product behaviour comes from the Encomendo product overview and design document; when they disagree with this prompt on product behaviour, the design document wins. If something here is ambiguous, choose the option that keeps the dependency rule, tenant isolation and testability intact, and record the decision as an ADR in `docs/adr/`.

---

## 1. Stack

| Part | Technology |
| --- | --- |
| Backend | Go (latest stable), ports and adapters (hexagonal), modular monolith |
| HTTP | Standard library `net/http` router (Go 1.22+ method patterns), no web framework |
| Database | PostgreSQL 17+, **GORM** over the pgx driver, row level security (RLS) |
| Migrations | goose, SQL files only, embedded in the binary |
| Cache | Redis 7+ (go-redis v9), used from the service layer |
| Auth | JWT access tokens (EdDSA / Ed25519, golang-jwt v5), opaque rotating refresh tokens, argon2id passwords; optional client accounts (email code or Google); opaque tracking tokens |
| Web | Vue 3 + TypeScript + Vite + Pinia + Vue Router, pnpm workspace (admin app + store page app with the client area) |
| Mobile | **Kotlin Multiplatform** with Compose Multiplatform UI, **Android only in v1** (iOS postponed) — management (backoffice) only |
| UI | Brazilian Portuguese (pt-BR); design tokens from the Repair Shop design system |
| CI | GitHub Actions |
| Local dev | Docker Compose |

---

## 2. Non-negotiable rules

1. **No generated code in the backend.** Every line of Go is written by hand: no sqlc, no `gorm/gen`, no mockgen/mockery, no oapi-codegen, no wire/fx. GORM itself is allowed (it is a library, not a generator). The web and mobile clients *may* use generators (e.g. openapi-typescript, Room/KSP).
2. **Dependency rule.** Code points inward only: `adapter → app → domain`. `internal/domain` imports only the Go standard library. The app layer owns the ports (interfaces). Only `cmd/api/main.go` knows every concrete type.
3. **Tenant isolation is enforced three times:** JWT principal (or the store page's subdomain) → app-layer authorization → Postgres RLS. Never rely on only one. A client account crosses tenants only to read its own orders (section 5.5).
4. **GORM never creates schema.** `AutoMigrate` is never called. Schema comes only from SQL migrations.
5. **Heavy testing.** Every feature ships with tests at the levels described in section 9. Every bug fix starts with a failing test.
6. **Clean code everywhere:** small packages per bounded context, descriptive names (no `utils`, `common`, `helpers`), small interfaces defined next to their consumer, constructor injection, no globals, no `init()`, functions around 40 lines max, errors wrapped with context and never swallowed.
7. **Feature parity for management.** The web admin app and the mobile app are both management apps with the same features. A management feature is done only when its web screen and its mobile screen both ship against the same API endpoints. Clients never use the management apps: they use the store page and, if they log in, the client area, both in the browser.
8. **Clients never see internal data.** Store page and tracking responses never contain suppliers, costs, margins or "Pode atrasar" alerts. They use their own view types, and tests assert it.
9. **WhatsApp through links only.** No paid WhatsApp API. Encomendo builds `wa.me` links with a prefilled message, the user taps to send, and the link is kept in the order history. Conversations stay in WhatsApp.
10. **Rules hang on phases, names come from data.** Each store picks its own list of order statuses (section 4.2, Status workflows). Code branches on the fixed order phase, never on a status key or label.

---

## 3. Monorepo layout

```text
encomendo/
├── backend/                 Go module: github.com/bergsantana/encomendo/backend
├── web/                     pnpm workspace: admin + store page + shared packages
├── mobile/                  Kotlin Multiplatform app: Android in v1, iOS later (management)
├── deploy/
│   ├── docker-compose.yml       base stack: postgres, redis, migrate, api, web (CI + e2e)
│   ├── docker-compose.dev.yml   development overlay: hot reload (ADR 0007)
│   ├── docker-compose.prod.yml  production stack with Caddy, secrets from env
│   ├── api.Dockerfile           multi-stage, distroless final image
│   ├── web.Dockerfile           build + nginx
│   ├── web.dev.Dockerfile       Vite dev servers
│   ├── caddy/, web/             Caddyfile; nginx template
│   └── postgres/                init.sql (dev roles), init-prod.sh, shared grants
├── docs/
│   ├── architecture.md      this document
│   ├── product/             design-document.md, product-overview.md
│   ├── adr/                 0001-ports-and-adapters.md, 0002-rls.md, 0003-jwt.md, 0004-status-workflows.md …
│   └── api/openapi.yaml     hand-written API contract, checked by API tests
├── .github/
│   ├── workflows/           backend.yml, web.yml, mobile.yml, security.yml, release.yml
│   ├── dependabot.yml
│   └── pull_request_template.md
├── Makefile                 dev-up, test, test-integration, lint, migrate
└── README.md
```

---

## 4. Backend

### 4.1 Top level

```text
backend/
├── cmd/
│   ├── api/main.go            composition root: config → db/redis → adapters → services → http
│   ├── migrate/main.go        runs embedded SQL migrations as encomendo_owner
│   └── worker/main.go         outbox relay + daily deadline check (Pode atrasar / Atrasada)
├── internal/
│   ├── domain/                entities, value objects, rules (stdlib only)
│   ├── app/                   use cases + the ports they need
│   ├── adapter/               http, postgres (GORM), redis, auth, storage, notify, inproc
│   ├── platform/              config, logger (slog), graceful shutdown, observability
│   └── testkit/               test builders (e.g. testkit.NewOrder().WithItems(2).Build())
├── test/
│   ├── contract/              port contract suites run against fakes AND real adapters
│   ├── integration/           testcontainers: postgres/, rls/, redis/
│   └── e2e/                   black-box API tests against docker compose
├── .golangci.yml
├── go.mod
└── Makefile
```

### 4.2 Domain layer (`internal/domain/`)

```text
internal/domain/
├── shared/
│   ├── id.go              typed IDs (TenantID, UserID, OrderID …) over UUIDv7
│   ├── money.go           Money{cents int64, currency string} — never float
│   ├── errors.go          ErrNotFound, ErrConflict, ErrForbidden, ErrInvalid + ValidationError
│   ├── page.go            cursor pagination value types
│   └── principal.go       Principal{Audience, UserID, TenantID, Role, ClientAccountID}
├── identity/              user.go, client_account.go, email.go, password_policy.go, role.go, permission.go
├── tenancy/               tenant.go, membership.go, settings.go, plan.go
├── catalog/               item.go, sourcing_mode.go, category.go, attribute.go ("Serve em"),
│                          customization_option.go, kit.go, kit_swap.go
├── supplier/              supplier.go, supplier_item.go (lead time, cost, backup supplier)
├── purchasing/            purchase_line.go, supplier_order.go, supplier_message.go, receipt.go
├── customer/              customer.go, address.go, phone.go
├── inventory/             stock_item.go, movement.go, reservation.go
├── workflow/              phase.go, status_workflow.go, workflow_status.go, validate.go
├── order/                 order.go, item.go, item_status.go, customization.go, payment.go,
│                          transitions.go, tracking_token.go, events.go
├── schedule/              deadline.go, readiness.go, alerts.go
├── production/            job.go, stage.go, capacity.go, assignment.go
└── portal/                storefront.go, template.go, branding.go, sections.go, subdomain.go
```

Each context also has `errors.go` for its sentinel errors and a `_test.go` per file.

**UI words ↔ code names** (the UI is pt-BR; code is English):

| UI (pt-BR) | Code |
| --- | --- |
| Sob encomenda / Estoque / Produção própria | `SourcingMode`: `to_order` / `stock` / `in_house` |
| Sinal / Saldo | `PaymentKind`: `deposit` / `balance` |
| Precisa chegar até | `ArrivalLimit` |
| Pode atrasar / Atrasada | `Alert`: `at_risk` / `late` |
| Margem de segurança | `SafetyMargin` |
| Monte o seu / Kit pronto | custom order / `Kit` |
| Entrega / Retirada | `Handover`: `delivery` / `pickup` |
| Lista de status da loja | `workflow.StatusWorkflow` (statuses with a `workflow.Phase`) |

**Domain rules to encode:**

- Aggregates protect their invariants: constructors validate, fields are unexported, state changes go through methods.
- **Sourcing belongs to each catalog item**, not the tenant: `catalog.SourcingMode` is `to_order`, `stock` or `in_house`. One order can mix all three. Tenant settings turn `stock_enabled` and `production_enabled` on or off; when off, that mode is unavailable and its menu is hidden.
- **Kits:** a ready kit has a fixed price and marks which items the client may swap. A swap costs kit price − removed item + new item, both at single-item price; the kit discount stays and the date is recalculated (`kit_swap.go`).
- **Status workflows (each store picks one).** The order lifecycle has fixed **phases** in code, and each store chooses a **status list** that names the steps its clients and staff see:
  - `workflow.Phase` (`phase.go`): `draft → awaiting_deposit → confirmed → sourcing → assembling → ready → handed_over`, plus `cancelled`. Phases carry every rule; statuses carry only names and order.
  - A `workflow.StatusWorkflow` is an ordered list of `WorkflowStatus{key, label (pt-BR), client label (pt-BR, plain words for the store page), phase}`. Several statuses may share a phase (for example "Compras" and "Itens chegando" are both `sourcing`), and phases never go backwards along the list.
  - Every workflow contains `draft`, `awaiting_deposit`, `ready` and `handed_over`. `confirmed`, `sourcing` and `assembling` are optional; `cancelled` is implicit. `validate.go` enforces this, and every preset has a test.
  - Workflows are global presets stored as data (seeded by a migration), read-only to tenants, like templates. The owner picks one in settings, and the niche template suggests a default. Launch presets, wording to confirm with real shops:

    | Preset | Statuses (phase) |
    | --- | --- |
    | `encomenda_montagem` | Rascunho (draft) → Aguardando sinal (awaiting_deposit) → Compras (sourcing) → Itens chegando (sourcing) → Em montagem (assembling) → Pronto (ready) → Entregue (handed_over) |
    | `orcamento_primeiro` | Orçamento (draft) → Aguardando sinal (awaiting_deposit) → Aprovado (confirmed) → Em preparação (sourcing) → Pronto (ready) → Entregue (handed_over) |
    | `producao_propria` | Rascunho (draft) → Aguardando sinal (awaiting_deposit) → Na fila (sourcing) → Em produção (sourcing) → Acabamento (assembling) → Pronto (ready) → Entregue (handed_over) |
    | `pronta_entrega` | Rascunho (draft) → Aguardando sinal (awaiting_deposit) → Separando (sourcing) → Pronto para retirada (ready) → Retirado (handed_over) |

  - An order keeps the workflow id and version it was created with. Changing the store's workflow affects new orders only, and a published workflow version never changes (an edit is a new version).
  - `Order.Advance(to StatusKey, by Principal, now)` (`transitions.go`) moves to any later status of the order's workflow, but only if the guard of every phase it enters holds; moving back is allowed only between statuses of the same phase. It returns an error for any other move. Guards:
    - leave `awaiting_deposit`: the deposit is paid, or the owner released the order without it;
    - enter `assembling`: every item is received (or ready / reserved) and checked;
    - enter `ready` in a workflow without `assembling`: the same check as for `assembling`;
    - enter `handed_over`: the balance is paid; records whether it was `delivery` or `pickup`;
    - `cancelled` is reachable before `handed_over`; it releases stock reservations and cancels purchase lines not yet sent, and flags lines already sent for the owner.
  - An order with nothing to source can pass `sourcing` at once, because its guards already hold. A workflow without `assembling` uses 0 assembly days in the deadline.
  - The client's tracking page shows the workflow's statuses as its steps, using the client labels.
- **Item lifecycle** (`item_status.go`), per sourcing mode:
  - `to_order`: `to_buy → sent_to_supplier → in_transit → received → installed`
  - `stock`: `reserved → installed` (or the `to_order` path with an `out_of_stock` warning when there is not enough on hand)
  - `in_house`: `planned → in_production → ready → installed`
  - A received item is checked as `ok` or `defective`; a defective item goes back to `to_buy` or `in_production`.
- **Deadline** (`schedule`, pure functions):
  - delivery date = latest ready date among the order's items + assembly days + safety margin.
  - Ready date: `to_order` = release date (deposit date, or the owner's release) + supplier lead time, replaced by the supplier's promised date once recorded; `stock` = today if enough is on hand, otherwise supplier lead time; `in_house` = production days + wait for free weekly capacity.
  - arrival limit ("Precisa chegar até") = delivery date − assembly days.
  - `at_risk` when an item's expected date is after the arrival limit; `late` when the expected date passed and the item has not arrived. Alerts sit on top of the status; they are not statuses.
  - Defaults live in tenant settings (deposit 50%, assembly 3 days, safety margin 2 days) and can be overridden per kit and per order.
  - Reference test: Kit Scrambler CB 500X = 15 days (leather seat, the slowest) + 3 assembly + 2 margin = 20 days.
- **Purchasing:** by default purchase lines are generated only after the deposit; tenant setting `purchase_before_deposit` lets the owner change this. Every purchase line points to one order item (so one order and one client) or to a stock restock. Lines are grouped by supplier into supplier orders. `supplier_message.go` builds the supplier text and must never include the client's name or phone.
- **Receiving:** a receipt records the checklist result per line, a photo of the item and a photo of the invoice; the order knows when it is complete.
- **Payments:** deposit and balance are recorded against the order; the balance is due at delivery or pickup.
- **Changing the date:** when the owner changes the delivery date, the order records the change and returns a `wa.me` link to warn the client. The client is never notified automatically.
- An order item stores a snapshot of item name, price, sourcing mode and chosen options at order time; later catalog edits never change past orders.
- `identity.Permission` constants (`order:read`, `order:write`, `purchase:write`, `receiving:write`, `catalog:write`, `inventory:adjust`, `production:write`, `finance:read`, `members:manage`, `portal:write`, `settings:write` …) and the role → permission map live in code, are reviewed and tested like any rule.
- **Client accounts** are global, not tenant-scoped. An order is linked to a client account when the client is logged in at checkout, or later when a logged-in client opens the order's tracking link and chooses "Adicionar às minhas encomendas". Linking never merges or exposes the per-store `customer` records.
- Each aggregate carries a `version int` for optimistic locking.

Aggregate shape:

```go
// internal/domain/order/order.go
type Order struct {
    id           shared.OrderID
    tenantID     shared.TenantID
    customer     shared.CustomerID
    items        []Item
    workflow     workflow.StatusWorkflow // the version the order was created with
    status       workflow.StatusKey
    total        shared.Money
    payments     []Payment
    deliveryDate time.Time
    version      int
}

func New(tenant shared.TenantID, customer shared.CustomerID, wf workflow.StatusWorkflow, items []Item, now time.Time) (*Order, error)
func (o *Order) Advance(to workflow.StatusKey, by shared.Principal, now time.Time) error
func (o *Order) Phase() workflow.Phase
func (o *Order) Total() shared.Money
func (o *Order) DepositDue() shared.Money
```

### 4.3 Application layer and ports (`internal/app/`)

```text
internal/app/
├── shared/
│   ├── tx.go             TxManager port: WithinTx(ctx, fn) — sets the tenant for RLS
│   ├── cache.go          Cache port: Get/Set/Delete/DeleteByPrefix with TTL
│   ├── clock.go          Clock port (fixed clock in tests)
│   ├── idgen.go          IDGenerator port (UUIDv7)
│   ├── events.go         EventPublisher port → outbox table
│   └── authz.go          Require(ctx, perm) — reads Principal from ctx, checks role map
├── identity/             service.go (Register, Login, Refresh, Logout, SwitchTenant;
│                         client: RequestEmailCode, VerifyEmailCode, LoginWithGoogle), ports.go, inputs.go
├── tenancy/              service.go (CreateTenant, InviteMember, ChangeRole, UpdateSettings,
│                         ChooseStatusWorkflow), ports.go
├── workflow/             queries.go (list presets, get one by id and version; cached), ports.go
├── catalog/              service.go (items, kits, swaps), queries.go (cached reads), ports.go, inputs.go
├── supplier/             service.go (suppliers, lead times, costs, backup supplier), ports.go
├── purchasing/           service.go (GeneratePurchases, SendSupplierOrder → wa.me link,
│                         RecordPromisedDate, Receive), ports.go
├── customer/             service.go, ports.go
├── inventory/            service.go (Receive, Adjust, Reserve, Release, low-stock alerts), ports.go
├── order/                service.go (Create, ConfirmDeposit, ReleaseWithoutDeposit, Advance,
│                         RecordBalance, HandOver, ChangeDeliveryDate → wa.me link, Cancel),
│                         queries.go (Hoje, lists), ports.go, inputs.go
├── schedule/             service.go (Recalculate one order; CheckAll for the worker), ports.go
├── production/           service.go (StartJob, CompleteStage, Assign, SetWeeklyCapacity), ports.go
├── portal/               service.go (ApplyTemplate, UpdateBranding, ToggleSections, Publish), queries.go, ports.go
├── storefront/           service.go (public, login optional: Catalog, Kits, Preview price/deposit/date
│                         for Monte o seu, PlaceOrder, Track by token), ports.go
└── clientarea/           service.go (client audience: MyOrders across stores, OrderDetail, LinkOrder), ports.go
```

Every package has `service_test.go` using hand-written in-memory fakes from `internal/app/<ctx>/fake/`.

Ports example:

```go
// internal/app/order/ports.go

// Driving port: what HTTP (and any future adapter) calls.
type UseCases interface {
    Create(ctx context.Context, in CreateInput) (OrderView, error)
    Advance(ctx context.Context, id shared.OrderID, to workflow.StatusKey) (OrderView, error)
    Get(ctx context.Context, id shared.OrderID) (OrderView, error)
    List(ctx context.Context, f ListFilter, p shared.PageRequest) (shared.Page[OrderSummary], error)
}

// Driven ports: what the service needs.
type Repository interface {
    Insert(ctx context.Context, o *order.Order) error
    Update(ctx context.Context, o *order.Order) error // optimistic lock on version
    ByID(ctx context.Context, id shared.OrderID) (*order.Order, error)
    List(ctx context.Context, f ListFilter, p shared.PageRequest) (shared.Page[OrderSummary], error)
}

type StockReserver interface {
    Reserve(ctx context.Context, items []order.Item) error
    Release(ctx context.Context, orderID shared.OrderID) error
}

// WorkflowSource returns the store's current status workflow for new orders,
// and a given version for existing ones.
type WorkflowSource interface {
    Current(ctx context.Context) (workflow.StatusWorkflow, error)
    Version(ctx context.Context, id shared.WorkflowID, version int) (workflow.StatusWorkflow, error)
}
```

**How every write use case runs:**

1. `authz.Require(ctx, identity.PermOrderWrite)` — authorization lives in the app layer so every adapter gets it. Public store page use cases need no Principal: they run with the tenant resolved from the subdomain and, for tracking, a valid tracking token. When a client is logged in, their `client` Principal is attached so the order links to their account. Client area use cases require a `client` Principal.
2. Validate input; build or load the aggregate.
3. `tx.WithinTx(ctx, func(ctx context.Context) error { … })` wraps repository calls, stock reservation and the outbox event in one Postgres transaction.
4. After commit, invalidate affected cache keys.
5. Return a view type, never the domain struct. Store page and tracking views are separate types without internal fields.

Services receive dependencies through constructors, e.g. `order.NewService(repo, stock, workflows, schedule, tx, cache, events, clock, ids)`. No service imports another service; cross-context calls go through a port (e.g. `StockReserver` implemented by `adapter/inproc/stock_reserver.go` on top of `inventory.UseCases`, and `WorkflowSource` by `adapter/inproc/workflow_source.go` on top of `workflow` queries and tenant settings).

### 4.4 Adapters (`internal/adapter/`)

```text
internal/adapter/
├── http/
│   ├── server.go              http.Server with timeouts, graceful shutdown
│   ├── router.go              all routes: /api/v1/admin/…, /api/v1/store/…, /api/v1/me/…, /auth/…
│   ├── middleware/
│   │   ├── chain.go           tiny middleware composer
│   │   ├── requestid.go, recover.go, logging.go, cors.go, securityheaders.go
│   │   ├── authn.go           verifies JWT, puts Principal in ctx
│   │   ├── storefront.go      resolves the tenant from the Host subdomain ({slug}.encomendo.com.br)
│   │   └── ratelimit.go       Redis-backed, per IP + per user
│   ├── respond/
│   │   ├── json.go            Decode (size limit, DisallowUnknownFields) / Encode
│   │   └── problem.go         domain errors → RFC 9457 problem+json + status code
│   └── handler/
│       ├── auth.go, tenancy.go, workflow.go, catalog.go, supplier.go, purchasing.go, customer.go,
│       │   inventory.go, order.go, production.go, portal.go, storefront.go, health.go
│       └── dto/               request/response structs + mappers
├── postgres/
│   ├── db.go                  gorm.Open with the pgx driver: app role, pool size, statement timeout,
│   │                          SkipDefaultTransaction: true, slog logger adapter
│   ├── tx.go                  TxManager: db.Transaction + set_config('app.tenant_id', …, true)
│   ├── conn.go                returns the tx *gorm.DB from ctx (WithContext); repos never take a tx param
│   ├── errors.go              gorm.ErrRecordNotFound + pg codes (23505, 23503, 40001) → domain errors
│   ├── models/                GORM structs with tags, one file per context, + toDomain/fromDomain mappers
│   ├── identity_repo.go, tenancy_repo.go, workflow_repo.go, catalog_repo.go, supplier_repo.go,
│   │   purchasing_repo.go, customer_repo.go, inventory_repo.go, order_repo.go, production_repo.go,
│   │   portal_repo.go, outbox_repo.go
│   └── migrations/
│       ├── embed.go           //go:embed *.sql
│       └── 0001_… .sql        (section 7)
├── redis/
│   ├── client.go
│   ├── cache.go               implements app/shared.Cache, JSON values, tenant-prefixed keys
│   ├── ratelimiter.go         sliding window (Lua script in a .lua file)
│   └── idempotency.go         Idempotency-Key storage for POST endpoints
├── auth/
│   ├── jwt.go                 issue/verify access tokens (EdDSA, kid rotation)
│   ├── refresh.go             opaque refresh tokens: 32 random bytes, stored as SHA-256
│   ├── tracking.go            opaque order tracking tokens: 32 random bytes, stored as SHA-256
│   ├── email_code.go          one-time client login codes: 6 digits, 10 min, stored hashed, attempt limit
│   ├── google.go              verifies Google ID tokens (OIDC: issuer, audience, signature via JWKS)
│   └── argon2id.go            PasswordHasher port
├── inproc/
│   ├── stock_reserver.go      order.StockReserver backed by inventory.UseCases
│   └── workflow_source.go     order.WorkflowSource backed by workflow queries + tenant settings
├── storage/                   local.go (dev), s3.go (S3-compatible) for item, invoice, receiving and logo/cover photos
└── notify/                    whatsapp_link.go (wa.me links with prefilled text, no paid API),
                               smtp.go (staff invites, password reset, client login codes)
```

**GORM repository rules:**

- Persistence models live in `postgres/models/` and carry the `gorm` tags. Domain structs never have ORM tags. Map with `toDomain()` / `fromDomain()`.
- Use GORM's builder (`Where`, `Joins`, `Preload`); use `Raw` only where the builder gets unclear (reports, stock sums, purchases grouped by supplier).
- Optimistic locking: updates add `Where("version = ?", v)` and return `ErrConflict` when `RowsAffected == 0`.
- `SkipDefaultTransaction: true` — the TxManager owns transactions.
- Do not embed `gorm.Model` and do not use `DeletedAt` (no hidden soft-delete filter).
- `Updates(struct)` skips zero values: use `Select("*")` or a map when a field must become 0, "" or false.
- `Preload` only what the use case returns.
- GORM logger goes through slog; slow-query threshold from config.

**Redis cache rules:**

- Keys always start with the tenant: `t:{tenantID}:catalog:item:{id}`. `cache.go` refuses a key without a tenant prefix (RLS does not protect Redis). Global presets (templates, status workflows) use a `g:` prefix and are read-only.
- Cache only hot, safe-to-be-slightly-stale reads: store page catalog and kits, portal config/template, item detail, status workflow presets. TTL 5–10 min plus explicit invalidation after writes.
- Never cache order, tracking or customer data, or anything that depends on the caller's permissions.
- A Redis outage degrades to Postgres reads; it never fails a request.

**Composition root:** `cmd/api/main.go` reads config, opens GORM and Redis, builds each adapter, injects them into service constructors, hands services to handlers, starts the server. Plain and explicit; the only file importing every package.

---

## 5. Auth, authorization and RLS

### 5.1 Tokens

Business users (owners, admins, staff) always log in. Clients may log in, but never have to: ordering and tracking work without an account.

| Token | Format | Lifetime | Client storage | Contents |
| --- | --- | --- | --- | --- |
| Access | JWT, EdDSA, `kid` header | 15 min | Memory (web); encrypted DataStore + Keystore (Android) | `sub`, `tid`, `role`, `aud`, `jti`, `iat`, `exp`, `iss` |
| Refresh | Opaque 32 random bytes | 30 days, rotated on every use | httpOnly Secure SameSite=Strict cookie (web; client cookie on `.encomendo.com.br` so one login works on every store page); Keystore (Android) | Server stores only SHA-256 hash + `family_id`, `user_id` or `client_account_id`, `tenant_id` |

- `aud` is `backoffice` (staff, carries `tid` and `role`) or `client` (carries `sub` = client account id, no `tid`). Client tokens are rejected on admin routes and staff tokens on `/api/v1/me/*`.
- Refresh rotation with reuse detection: a reused refresh token revokes its whole family.
- `POST /auth/switch-tenant` issues a new access token for another tenant the user belongs to.
- `jwt.go` pins the algorithm (reject `none` and HS*), checks `iss`, `aud`, `exp`, `nbf` (30 s leeway), loads keys by `kid`.
- Logout deletes the refresh family; urgent revocation uses a Redis `jti` denylist until expiry.
- Login rate-limited per IP and per email; passwords hashed with argon2id.

### 5.2 Client access without an account

- An order placed on the store page identifies the client by name and WhatsApp phone (unique per tenant); a returning phone links to the same customer.
- Each order gets an opaque tracking token (32 random bytes, stored only as SHA-256). The tracking page is `{slug}.encomendo.com.br/p/{token}`; the owner shares it through a `wa.me` link, and the client's confirmation screen shows it too.
- The tracking endpoint returns only the client view (forecast, steps from the order's status workflow, items in plain words, payment summary), is rate-limited per IP, and never reveals whether a token exists in another tenant. The owner can rotate the token.
- Order placement is rate-limited per IP and phone and requires an `Idempotency-Key`.

### 5.3 Client accounts

- Optional. Login with a one-time code sent by email or with Google; no passwords for clients. (Codes by WhatsApp or SMS would need a paid API.)
- One global account per client, used on every store page and in the client area (`Minhas encomendas`).
- Orders link to the account at checkout (logged in) or by claiming a tracking link while logged in.
- The client sees, for each linked order: store name, status, forecast, items in plain words and payment summary — the same client view as tracking.
- Because the client cookie is shared across `*.encomendo.com.br`, tenants can never add scripts or custom HTML to store pages (the fixed layout in 8.3 guarantees this).
- Code requests are rate-limited per IP and per email; account deletion removes the account and unlinks its orders.

### 5.4 Authorization (RBAC + policy)

| Role | Can do |
| --- | --- |
| `owner` | Everything in the tenant, finance, billing, status workflow choice, delete tenant |
| `admin` | Catalog, suppliers, purchasing, orders, inventory, production, portal, members (not owner) |
| `staff` | Orders, receiving and production stages; read catalog and stock |
| `viewer` | Read only, no costs or margins |
| `client` (no tenant) | Own profile; read own linked orders from every store; link an order by its tracking link |

`authz.Require(ctx, perm)` runs at the start of each use case. Rules finer than a role (e.g. costs and margins visible only to the owner, as on the order screen) are domain policy functions, each with its own tests. Clients only hide UI by permission; the backend is the authority.

### 5.5 Row level security

1. Migrations run as `encomendo_owner` (table owner). The API connects as `encomendo_app`: `NOSUPERUSER NOBYPASSRLS`, not the owner, only `SELECT/INSERT/UPDATE/DELETE` grants.
2. Every tenant table: `ENABLE ROW LEVEL SECURITY` and `FORCE ROW LEVEL SECURITY`.
3. `TxManager.WithinTx` opens a GORM transaction and runs `SELECT set_config('app.tenant_id', $1, true)` (transaction-local, safe with pooling). It also sets `app.user_id` for audit triggers.
4. No tenant set → `current_setting('app.tenant_id', true)` is NULL → zero rows (fail closed).
5. Store page routes resolve the tenant from the Host subdomain, then set the same variable; public reads expose only published data.
6. Cross-tenant platform jobs (the deadline check) use a separate `encomendo_platform` role from `cmd/worker`, never from the API.
7. Client area requests set `app.client_account_id` instead of `app.tenant_id`. The tables a client reads (`orders`, `order_items`, `payments`, `tenants` name only, and the global `status_workflows` / `workflow_statuses` for step names) get a second, `SELECT`-only policy: `client_account_id = current_setting('app.client_account_id', true)::uuid` (the global workflow tables need no tenant policy). No write policy uses it, and supplier, cost and purchasing tables have no client policy at all.

```sql
ALTER TABLE orders ENABLE ROW LEVEL SECURITY;
ALTER TABLE orders FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON orders
  USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
```

---

## 6. API conventions

- Base paths: `/auth/*`, `/api/v1/admin/*` (staff), `/api/v1/me/*` (client accounts: profile, orders from every store, link an order), `/api/v1/store/*` (public; tenant from the Host subdomain: catalog, kits, Monte o seu preview, order placement, tracking by token), `/healthz`, `/readyz`.
- Status workflows: `GET /api/v1/admin/status-workflows` lists the presets; `PUT /api/v1/admin/settings/status-workflow` chooses one (owner). Order responses carry the status key, its label, its client label and its phase, so clients never need their own copy of the list.
- JSON only; errors as RFC 9457 `application/problem+json` with a stable `code` field.
- Cursor pagination (`?cursor=&limit=`), max limit enforced.
- `Idempotency-Key` header supported on POST endpoints that create resources (orders, purchases, receipts, stock movements, uploads).
- Optimistic locking surfaced as `409 Conflict` with code `version_conflict`.
- Endpoints that produce a WhatsApp message return the `wa.me` URL; the client opens it.
- `docs/api/openapi.yaml` is written by hand and kept in sync; API tests validate responses against it.

---

## 7. Database schema and migrations

```text
internal/adapter/postgres/migrations/
├── 0001_roles_and_extensions.sql   pgcrypto, citext; grants for encomendo_app
├── 0002_identity.sql               users, client_accounts, client_identities, client_login_codes, refresh_tokens
├── 0003_workflows.sql              status_workflows, workflow_statuses (global) + launch presets seed
├── 0004_tenancy.sql                tenants, memberships, tenant_settings (references status_workflows)
├── 0005_rls_helpers.sql            current_tenant_id() function, audit trigger function
├── 0006_catalog.sql                categories, items, item_attributes, customization_options, kits, kit_items, media
├── 0007_suppliers.sql              suppliers, supplier_items
├── 0008_customers.sql              customers, customer_addresses
├── 0009_inventory.sql              stock_items, stock_movements, stock_reservations
├── 0010_orders.sql                 orders, order_items, order_status_history, payments, order_messages
├── 0011_purchasing.sql             supplier_orders, purchase_lines, receipts, receipt_photos
├── 0012_production.sql             production_jobs, production_stages, production_capacity
├── 0013_portal.sql                 templates (global), storefronts, storefront_sections
├── 0014_platform.sql               audit_log, outbox_events, idempotency_keys
└── 0015_rls_policies.sql           ENABLE + FORCE RLS and policies for every tenant table
```

Each file has `-- +goose Up` and `-- +goose Down`; CI runs up → down → up.

| Table | Tenant-scoped | Notes |
| --- | --- | --- |
| `users` | No | Staff identities; `email citext unique`, `password_hash` |
| `client_accounts`, `client_identities` | No | Global client accounts; identities: verified email (`citext unique`) or Google `sub` |
| `client_login_codes` | No | Hashed one-time codes, `expires_at`, attempts |
| `refresh_tokens` | No | `token_hash`, `family_id`, `tenant_id`, `expires_at`, `used_at`, `revoked_at` |
| `status_workflows`, `workflow_statuses` | No | Global presets, read-only to tenants; `(id, version)` immutable once published; statuses with `key`, `label`, `client_label`, `phase`, `position` |
| `tenants` | Self | `slug unique` (the subdomain label; reserved words blocked), `niche`, `status`; policy `id = current tenant` |
| `memberships` | Yes | `(tenant_id, user_id) unique`, `role`; policy also lets a user see their own memberships for tenant switching |
| `tenant_settings` | Yes | `status_workflow_id` + `status_workflow_version`, `stock_enabled`, `production_enabled`, `purchase_before_deposit`, `deposit_percent` (50), `assembly_days` (3), `safety_margin_days` (2), currency, WhatsApp phone |
| `items` | Yes | `sourcing_mode`, `price_cents`, `cost_cents`, `production_days`, `published` ("No portal"), `version` |
| `item_attributes` | Yes | Template-defined fields such as "Serve em" (motorcycle model, roof type, room) |
| `customization_options` | Yes | Per-item client options with `jsonb` rules |
| `kits`, `kit_items` | Yes | Fixed kit price; `swappable` flag per kit item; assembly and margin overrides |
| `suppliers`, `supplier_items` | Yes | Lead time days, cost, backup supplier |
| `customers` | Yes | Name, WhatsApp phone unique per tenant; optional `client_account_id` |
| `stock_items`, `stock_movements`, `stock_reservations` | Yes | Movements append-only; on-hand quantity derived from them; minimum quantity per item |
| `orders`, `order_items` | Yes | Items carry a `jsonb` snapshot and an item status; orders carry `workflow_id`, `workflow_version`, `status_key`, `delivery_date`, `assembly_days`, `safety_margin_days`, `handover` (delivery/pickup), `tracking_token_hash`, optional `client_account_id`, `version` |
| `order_status_history` | Yes | Who, from, to (status key and phase), when; includes date changes |
| `payments` | Yes | `kind` (deposit/balance), amount, method, recorded by, when |
| `order_messages` | Yes | Every `wa.me` link generated for the order: to whom, text, when |
| `supplier_orders`, `purchase_lines` | Yes | A line points to one order item, or to a restock (`order_item_id` null); expected and promised dates; status |
| `receipts`, `receipt_photos` | Yes | `ok` / `defective` per line; item photo and invoice photo |
| `production_jobs`, `production_stages`, `production_capacity` | Yes | One job per in-house item; ordered stages; weekly capacity |
| `templates` | No | Global niche presets: categories, attributes, default sourcing per category, sample kits, wording, suggested status workflow; read-only to tenants |
| `storefronts`, `storefront_sections` | Yes | Template id, logo, brand color, cover photo; sections on/off (Kits prontos, Monte o seu, Acompanhar pedido) |
| `audit_log`, `outbox_events`, `idempotency_keys` | Yes | Written by triggers or in the same transaction |

**Conventions:** UUIDv7 primary keys generated in Go. Every tenant table has `tenant_id uuid not null references tenants`, an index starting with `tenant_id`, `created_at`, `updated_at`. Money = `bigint` cents + `char(3)` currency. Composite foreign keys `(tenant_id, id)` so a row can never reference another tenant's row.

---

## 8. Clients

### 8.1 Web (`web/`)

```text
web/
├── pnpm-workspace.yaml
├── packages/
│   ├── api/                    http.ts (fetch wrapper: base URL, JSON, problem+json → ApiError, 401 → refresh once),
│   │                           auth.ts, catalog.ts, suppliers.ts, purchasing.ts, orders.ts, customers.ts,
│   │                           inventory.ts, production.ts, portal.ts, workflows.ts, store.ts,
│   │                           types/ (generated from docs/api/openapi.yaml with openapi-typescript)
│   └── ui/                     Repair Shop design tokens, status badges (icon + word), BaseButton, BaseInput,
│                               DataTable, Modal, Toast …
└── apps/
    ├── admin/src/              main.ts, App.vue, router/ (meta: requiresAuth, permission), stores/session.ts,
    │                           composables/ (usePermission, usePagination, useForm), layouts/,
    │                           features/{auth,hoje,pedidos,compras,producao,estoque,catalogo,clientes,
    │                                     financeiro,members,portal,settings}/{views,components,store.ts}
    └── store/src/              main.ts (tenant from the Host subdomain), one fixed layout,
                                sections/{KitsProntos,MonteOSeu,AcompanharPedido},
                                features/{kits,monte-o-seu,checkout,tracking,login,minhas-encomendas}
```

- Admin menu: Hoje · Pedidos · Compras · Produção · Estoque · Catálogo · Clientes · Financeiro. Produção and Estoque appear only when the tenant turns them on. Fornecedores is a tab inside Compras; Kits is a tab inside Catálogo. The status workflow is chosen in settings, with a preview of its steps.
- Inside a feature: view → Pinia store → `packages/api`. Components never call `fetch`; views hold no business logic.
- Access token in memory only; refresh cookie is httpOnly. On load, call `/auth/refresh`.
- `usePermission('order:write')` only hides UI.
- Status badges show the label from the order's workflow; their color comes from the phase, never from the label (`draft`, `awaiting_deposit`, `confirmed` → A comprar / Planejado; `sourcing` → Aguardando fornecedor / Em produção; `assembling` → Em montagem; `ready` → Pronto; `handed_over` → Entregue; `cancelled` → neutral). Alerts sit on top with their own icon and word.
- The store page is mobile-first and light (client phone sizes: 16–17 px body, 48 px controls). Login is optional and never blocks ordering or tracking; `Minhas encomendas` is reachable from every store page and at `conta.encomendo.com.br`, served by the same app.
- Tests: Vitest + Vue Test Utils, MSW for API mocks, Playwright for main flows against docker compose.

### 8.2 Mobile — Kotlin Multiplatform (`mobile/`)

One Kotlin Multiplatform app, **Android only in v1** (iOS postponed), with **every feature of the web admin app**: the full management side for owners, admins and staff, also used in the workshop (larger targets: 18 px body, 56 px targets, 64 px primary action). Clients never use it. Domain, data, networking, ViewModels and Compose Multiplatform UI live in `shared/` so an iOS shell can be added later without a rewrite; `androidApp` is a thin shell. Keep platform code behind `expect`/`actual` as if iOS existed, but do not add iOS targets, code or CI until it is scheduled.

**Libraries:** Ktor client (OkHttp engine) + kotlinx.serialization; Room 3 (`androidx.room3`, the multiplatform Room) with `BundledSQLiteDriver`; DataStore; Koin; AndroidX Lifecycle ViewModel and Navigation Compose (multiplatform builds); Coil 3; kotlinx-datetime. **Do not use** Hilt or Retrofit (Android-only).

```text
mobile/
├── settings.gradle.kts, build.gradle.kts, gradle/libs.versions.toml
├── build-logic/                    convention plugins, once module setup duplication justifies them (ADR 0005)
├── shared/                         KMP target: androidTarget (iosArm64, iosSimulatorArm64 later)
│   ├── core/
│   │   ├── domain/                 commonMain only: models, repository interfaces, use cases, Permission
│   │   ├── network/                Ktor client, kotlinx.serialization DTOs, Auth bearer plugin with refresh,
│   │   │                           ApiError mapping (problem+json)
│   │   ├── database/               Room KMP: entities, DAOs, migrations (offline cache)
│   │   ├── data/                   repository implementations, mappers, offline write queue
│   │   ├── security/               TokenStore expect/actual: Keystore + DataStore (Android)
│   │   ├── platform/               expect/actual: camera + photo picker, share sheet, open links (wa.me),
│   │   │                           background retry
│   │   ├── ui/                     Repair Shop theme, status badges (label from workflow, color from phase),
│   │   │                           shared composables, UiState helpers, PermissionGate
│   │   ├── navigation/             RootNavHost, BackofficeNavGraph
│   │   ├── di/                     Koin modules: networkModule, databaseModule, dataModule, platformModule
│   │   └── testing/                fakes of every repository, test dispatchers, builders
│   └── feature/                    each feature: XScreen.kt, XViewModel.kt, XUiState.kt, di/XModule.kt
│       ├── auth/                   Login, SignUp, TenantPicker, ForgotPassword
│       ├── hoje/                   summary cards, orders by delivery date, "Precisa de você" alerts, today's purchases
│       ├── pedidos/                OrderList (by workflow status), OrderDetail (item × deadline timeline, delay banner,
│       │                           ways to fix, payments, change date + warn client)
│       ├── compras/                by supplier, send via wa.me, promised dates, Fornecedores tab,
│       │                           ReceiveItems (checklist, item + invoice photos, Tudo certo / Com defeito)
│       ├── producao/               production board, stages, weekly capacity
│       ├── estoque/                StockList, AdjustStockSheet, movements, low-stock alerts
│       ├── catalogo/               items (sourcing, supplier, lead time, cost, price, margin, "No portal"), kits
│       ├── clientes/               CustomerList, CustomerDetail
│       ├── financeiro/             deposits, balances, margins (owner)
│       ├── members/                invite, change role
│       ├── portal/                 PortalBuilder: template, logo, brand color, cover photo, sections,
│       │                           live preview (WebView of the store page preview URL)
│       └── settings/               tenant settings, status workflow, stock/production on/off, defaults, WhatsApp number
└── androidApp/                     MainActivity, EncomendoApplication (startKoin), WorkManager retry
```

**Mobile rules:**

- Layers: Screen (Compose Multiplatform) → ViewModel exposing `StateFlow<UiState>` → use case in `core:domain` → repository interface → implementation in `core:data` (uses `core:network` and `core:database`).
- Code goes in `commonMain` unless it touches a platform API; then use `expect` in `commonMain` with a small `actual` implementation in `androidMain`, behind an interface.
- After login the app opens the management graph, filtered by permissions.
- Auth bearer plugin refreshes once on 401 and logs out if refresh fails.
- Offline: recent orders, the receiving checklist and the production board are cached in Room; writes made offline (including receiving photos) queue and retry with WorkManager and an `Idempotency-Key`.
- ViewModels obtained with `koinViewModel()`.
- Tests: kotlin.test, Turbine and coroutines-test in `commonTest` with fakes from `core:testing`; Ktor `MockEngine` for `core:network`; in-memory Room tests; Compose Multiplatform UI tests for order and receiving screens. Shared tests run on the JVM and Android.

### 8.3 Templates are data, the store page has one layout

A template is a niche preset stored as data: categories, item attributes ("Serve em"), default sourcing per category, sample kits, wording (e.g. Peças → Módulos, Montagem → Instalação) and a suggested status workflow. It is configuration, not separate code.

A store page is saved as template id + branding (logo, brand color, cover photo) + sections on/off in `jsonb`. Status colors, contrast and layout stay fixed so every store page stays readable and there is one front end to support. Only the Vue store app renders it; the mobile portal builder previews it in a WebView.

### 8.4 Parity map

| Feature | Web (Vue) | Mobile module (shared/) | Who uses it |
| --- | --- | --- | --- |
| Hoje | `admin/features/hoje` | `feature/hoje` | owner, admin, staff |
| Pedidos | `admin/features/pedidos` | `feature/pedidos` | owner, admin, staff |
| Compras, fornecedores, receiving | `admin/features/compras` | `feature/compras` | owner, admin; staff receives |
| Produção | `admin/features/producao` | `feature/producao` | owner, admin, staff |
| Estoque | `admin/features/estoque` | `feature/estoque` | owner, admin; staff read-only |
| Catálogo and kits | `admin/features/catalogo` | `feature/catalogo` | owner, admin |
| Clientes | `admin/features/clientes` | `feature/clientes` | owner, admin |
| Financeiro | `admin/features/financeiro` | `feature/financeiro` | owner |
| Members and roles | `admin/features/members` | `feature/members` | owner, admin |
| Portal builder | `admin/features/portal` | `feature/portal` | owner, admin |
| Tenant settings, status workflow | `admin/features/settings` | `feature/settings` | owner |
| Store page: Kits prontos, Monte o seu, checkout, tracking | `store/features/*` | — (web only, in the client's browser) | clients, login optional |
| Minhas encomendas (orders from every store) | `store/features/minhas-encomendas` | — (web only) | logged-in clients |

---

## 9. Testing strategy

Coverage gate: **≥ 90% on `internal/domain` + `internal/app`, ≥ 80% overall** for the backend.

| Level | What it proves | Tools | Where |
| --- | --- | --- | --- |
| Domain unit | Invariants, money math, kit swap pricing, deadline formula and alerts, item transitions, `workflow` validation, and every allowed/forbidden move and phase guard for **every preset workflow** | `testing`, table-driven, native fuzzing (slugs, money parsing) | `internal/domain/**` |
| Use case | Authorization, orchestration, events, cache invalidation, purchases released only after deposit unless the setting allows, new orders take the store's current workflow and keep it after the store changes | Hand-written fakes, fixed clock, fixed ID generator | `internal/app/**/service_test.go` |
| Port contract | Fakes behave like real adapters | One suite per port, run against fake and real adapter | `test/contract/` |
| Repository integration | GORM queries and model mappers correct; locking; constraint errors → domain errors | testcontainers-go, Postgres, migrations applied | `test/integration/postgres/` |
| RLS | No tenant can read/insert/update/delete another tenant's rows; a client account reads only its own linked orders, across tenants, and writes nothing | Two seeded tenants and two client accounts, queries as `encomendo_app` | `test/integration/rls/` |
| Cache | Tenant prefix, TTL, invalidation, Redis-down fallback | testcontainers-go, Redis | `test/integration/redis/` |
| HTTP | Routing, status codes, problem+json, DTO validation, auth middleware, subdomain resolution | `httptest` + fake use cases | `internal/adapter/http/**` |
| Client visibility | Store page and tracking responses never contain suppliers, costs, margins or `at_risk`; supplier messages never contain client name or phone | `httptest` + real services with fakes | `internal/adapter/http/**`, `internal/domain/purchasing` |
| Auth security | Expired, wrong `aud`, `alg: none`, unknown `kid`, refresh reuse; client token on admin routes and staff token on `/me`; login code expiry and attempt limit; Google token with wrong audience; tracking token guessing and cross-tenant lookup | `httptest` + real auth adapter | `internal/adapter/auth/` |
| API e2e | Sign up → tenant → choose status workflow → items and kit → order placed on the store page → deposit → purchases → receive → assemble → delivery or pickup | Go client against docker compose | `test/e2e/` |
| Web | Components, stores, API client, flows (admin and store page) | Vitest, Vue Test Utils, MSW, Playwright | `web/**` |
| Mobile (KMP, Android) | ViewModels, use cases, repositories, screens | kotlin.test + Turbine in commonTest, Ktor MockEngine, fakes, Compose UI tests; JVM + Android | `mobile/**` |

Rules:

- Go tests run with `-race -shuffle=on -count=1`; no test depends on another's order or data.
- Every fake passes the same contract suite as its real adapter.
- The RLS suite checks five cases per tenant table: select, insert with another tenant's id, update, delete, and no tenant set (expects zero rows). Tables with a client policy also check: another client's orders (zero rows) and any write as a client (rejected).
- Nightly mutation testing on `internal/domain`.

---

## 10. GitHub CI

| Workflow | Trigger | Jobs |
| --- | --- | --- |
| `backend.yml` | `backend/**` PR + push to main | lint (gofmt, go vet, golangci-lint) → unit (domain, app, http, `-race`) → integration (testcontainers: repos, RLS, cache) → migrations (up/down/up) → coverage gate → docker build; e2e on PRs to main + nightly |
| `web.yml` | `web/**` | `pnpm install --frozen-lockfile` → eslint + `vue-tsc --noEmit` → Vitest + coverage → build both apps → Playwright against docker compose |
| `mobile.yml` | `mobile/**` | Ubuntu: JDK 21 → detekt + ktlint → shared tests on JVM → Android lint → `assembleDebug` (APK artifact) |
| `security.yml` | Every PR + weekly | govulncheck, gitleaks (no CodeQL or dependency review: ADR 0008) |
| `release.yml` | Tag `v*` | Build + push images to GHCR, run migrations job, signed release APK on the GitHub release |

- Cache Go modules, pnpm store and Gradle with `actions/setup-*` built-in caching.
- Dependabot weekly for Go modules, npm, Gradle and GitHub Actions.
- Signing keys and registry tokens live in GitHub environment secrets, available only to `release.yml`.
- All workflows are required checks on `main`.

---

## 11. Build order

Build security and tenancy first. Each milestone ends with all tests green in CI and both management clients updated.

1. **Skeleton** — monorepo, docker compose, `platform/config` + logger, `/healthz`, goose runner, GORM setup, five CI workflows with jobs passing, KMP shell (shared, androidApp) with Koin, Room KMP and the management navigation graph.
2. **Identity and tenancy** — users, tenants, memberships, tenant settings, JWT + refresh rotation, RBAC map, `TxManager` with tenant setting, RLS policies + RLS test suite. Login, sign-up and tenant picker on web and mobile.
3. **Catalog and suppliers** — items with sourcing mode, attributes, kits and swaps, suppliers with lead times and costs, photos, Redis cache for store page reads. Catálogo screens on web and mobile.
4. **Customers, orders and deadlines** — status workflows (phases, presets, validation, choice in settings), order aggregate with phase guards, item state machines, deposit and balance, deadline calculation and alerts, tracking tokens, outbox. Hoje, Pedidos, Clientes, Financeiro and the workflow setting on web and mobile.
5. **Purchasing and receiving** — purchase lines after deposit, supplier orders through `wa.me`, promised dates, receiving checklist with photos. Compras screens on web and mobile, including photo capture.
6. **Inventory and production** — stock reservations and low-stock alerts, production board and weekly capacity, worker for outbox and the daily deadline check. Estoque and Produção on web and mobile.
7. **Store page** — niche templates as data, portal builder, store page on `{slug}.encomendo.com.br` (wildcard DNS and TLS) with Kits prontos, Monte o seu (live price, deposit and date), checkout and tracking (steps from the order's workflow); optional client accounts (email code, Google) and Minhas encomendas across stores.
8. **Hardening** — rate limits, audit log views, full e2e suites on all clients, load test of store page reads, backup and restore drill.

---

## 12. How to work

- Start with **milestone 1** unless told otherwise. Before writing code, list the files you will create for the milestone, then implement them.
- Work in small, reviewable commits/PRs, one bounded context or concern at a time. Each PR includes its tests and passes all CI checks.
- Write tests first for domain rules and use cases.
- Keep `docs/api/openapi.yaml` in sync with every endpoint you add or change.
- Record any architecture decision not covered here as an ADR in `docs/adr/`.
- Do not add dependencies beyond those named here without a short justification in the PR description.
- Never weaken a rule in section 2 to make something easier; if a rule blocks you, stop and explain the trade-off.

### Open questions (do not decide silently — ask)

- Payments: record deposits and balances manually first, or integrate PIX from milestone 4?
- Store page SEO: is a Vite SPA enough, or does the store page need server rendering?
- Custom domains per tenant (`loja.cliente.com.br`) in v1, or only `{slug}.encomendo.com.br`?
- Status workflows: presets only in v1, or may a store later rename statuses or build its own list? Confirm the preset wording with real shops.
