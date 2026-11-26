# Encomendo — Build Plan and Progress

Tracks how much of the system is built, milestone by milestone. The milestones and their order come from `docs/architecture.md` section 11; the scope of each item comes from sections 4, 7 and 8 of the same document. This file records **status only**. If it disagrees with the architecture or the product docs, those win.

**Last audited:** 2026-11-24. Run locally: backend lint (golangci-lint v2.15.0), unit and integration tests pass; web lint, typecheck and tests pass. Mobile `./gradlew check` and `:androidApp:assembleDebug` pass in a container (`scripts/mobile-docker.sh`) after raising `compileSdk` to 37 and fixing the ktlint setup. **Not run:** CI.

## How to keep this file honest

- Tick a box (`[x]`) only when the work is merged, its tests exist at every level the change touches, and `make lint test` passes (plus `make test-integration` when adapters changed). That is the Definition of Done in `CLAUDE.md`.
- A management feature is done only when **both** the web admin screen and the Android screen work against the same endpoints. Track them as separate boxes and tick the feature only when both are ticked.
- Mark partial work in words next to the unticked box (`(partial: …)`). Never tick a half-done item.
- Update the summary table in the same PR that ticks boxes. Counts are plain boxes: `grep -c '\[x\]'` per section.
- A new decision that the architecture does not cover gets an ADR in `docs/adr/` and a line under "Decisions log" at the bottom.

Legend: `[x]` done · `[ ]` not done · **Blocked** means waiting on an open question.

---

## Summary

| # | Milestone | Done | Total | State |
| --- | --- | --- | --- | --- |
| 1 | Skeleton | 22 | 24 | Almost done: a verified green CI run and the worker shell remain |
| 2 | Identity and tenancy | 21 | 26 | In progress: backend contract, database, domain, auth adapter, data adapters, HTTP handlers, the suspended-store check, configuration, wiring and the backend e2e checks (plan steps 2.1 to 2.7); web auth client, every auth screen and Equipe; mobile API client, session, every auth screen and Equipe |
| 3 | Catalog and suppliers | 0 | 14 | Not started |
| 4 | Customers, orders and deadlines | 0 | 18 | Not started |
| 5 | Purchasing and receiving | 0 | 10 | Not started |
| 6 | Inventory and production | 0 | 9 | Not started |
| 7 | Store page | 0 | 12 | Not started |
| 8 | Hardening | 0 | 8 | Not started |
|   | **All milestones** | **43** | **121** | **about 36%** |

Percentages count checkboxes, and milestones differ a lot in size (milestone 4 is the heart of the product), so read them as a rough guide, not a schedule.

### What exists today

- **Backend:** config, slog logger, HTTP server with the middleware chain (request ID, logging, recover, security headers), RFC 9457 problem responses, `/healthz` and `/readyz`, GORM open with the pgx driver, Redis client, goose migration runner, the first migration (`0001_roles_and_extensions.sql`), `domain/shared` (typed UUIDv7 IDs, `Money`, `Page`, `Principal`, `Role`, sentinel errors), the `identity` permission map, `app/shared` ports with fakes and `authz`, and their contract suites in `test/contract/sharedports`. No bounded context yet, no `testkit`.
- **Web:** pnpm workspace with `admin` and `store` apps, `packages/api` (fetch wrapper, health call, generated types) and `packages/ui` (tokens, `BaseButton`, `StatusBadge`, phase colours). Admin shows a Hoje placeholder behind a dashboard layout. The store app resolves the tenant from the subdomain and renders a shell.
- **Mobile:** KMP shell with `core:domain` (`WorkflowPhase`), `core:ui` (theme, status tones), `core:database` (Room 3, sync-state entity), `core:network` (Ktor client, problem+json → `ApiError`, bearer with one refresh on 401, `/auth/refresh`, `SessionTurns` shared by every call that spends the refresh token, requests kept in the store they were made for), `core:security` (`TokenStore`: the session's tokens, store, role and permissions, Keystore-sealed in DataStore), `core:data` (the session-backed API client, login and logout, store switch and creation, the session renewed at each start, the store used last, `ApiError` → `Failure`) and `core:testing`, `core:navigation` (auth graph while signed out, the store picker while signed in without a store, management graph under the account bar once a store is open, with the session's permissions), `core:di` (Koin, one API client: `https://api.encomendo.com.br` in release, the local stack in debug), the login, sign-up and Esqueci a senha screens, the store picker (choose or create a store), the account bar (store, person, Trocar de loja, Sair) and a Hoje placeholder.
- **Delivery:** `docker compose` stack, API and web Dockerfiles, Postgres init script, five GitHub workflows, Dependabot, PR template, ADRs 0001–0018, an OpenAPI file with the health endpoints plus the milestone 2 `/auth/*`, tenancy and members contract (not served yet).

---

## Milestone 1 — Skeleton

Goal: the monorepo builds, runs and is checked by CI before any business code lands.

### Backend

- [x] Go module, `.golangci.yml`, `backend/Makefile`
- [x] `platform/config`: environment config with tests
- [x] `platform/logger`: slog JSON logger with tests
- [x] HTTP server with timeouts and graceful shutdown (`adapter/http/server.go`)
- [x] Middleware: chain, request ID, logging, recover, security headers (with tests)
- [x] `respond`: JSON decode with size limit, `problem+json` errors (with tests)
- [x] `/healthz` and `/readyz` handlers with tests
- [x] `adapter/postgres`: GORM + pgx open, slog logger adapter (with tests)
- [x] `adapter/redis`: client and ping (with tests)
- [x] `cmd/migrate`: goose runner (`up`, `down`, `reset`, `status`)
- [x] Migration `0001_roles_and_extensions.sql`
- [x] Migration integration test: roles and up → reset → up (`test/integration/postgres`)
- [x] `cmd/api/main.go` composition root
- [x] `backend/go.sum` committed
- [ ] `cmd/worker` shell (entry point only; the outbox relay and deadline check come in milestone 6)

### Web

- [x] pnpm workspace, `admin` and `store` apps, shared tsconfig and eslint config
- [x] `packages/api`: fetch wrapper (problem+json → `ApiError`), health call, types generated from OpenAPI (with tests)
- [x] `packages/ui`: Repair Shop tokens, `BaseButton`, `StatusBadge` (icon + word, colour from phase) (with tests)
- [x] Admin shell: router, dashboard layout, Hoje placeholder (with tests); Playwright smoke test

### Mobile

- [x] KMP shell: `androidApp`, `shared:core:{domain,ui,database,navigation,di}`, `shared:feature:hoje`
- [x] Koin modules, Room 3 database, management navigation graph, Hoje placeholder

### Delivery

- [x] `deploy/` compose stack (postgres, redis, migrate, api, web) and Dockerfiles
- [x] Five workflows: `backend.yml`, `web.yml`, `mobile.yml`, `security.yml`, `release.yml`; Dependabot; PR template
- [ ] All five workflows green on `main` (not yet observed; no CI run recorded)

---

## Milestone 2 — Identity and tenancy

Goal: a business owner can sign up, own a tenant, invite staff and log in on web and Android, with RLS proven by tests.

### Backend

- [x] `domain/shared`: typed IDs (UUIDv7), `Money`, `Page`, `Principal`, `Role`
- [ ] `domain/identity` (user, client account, email, password policy, password reset code, role, permission) and `domain/tenancy` (tenant, membership, invitation, settings, plan), with tests (partial: everything but the tenant plan, which no product document defines yet; see `domain/tenancy/doc.go`)
- [x] `app/shared`: `TxManager` and `Scope`, `Cache`, `Clock`, `IDGenerator`, `EventPublisher`, `authz.Require`; fakes and their contract suites (ADR 0009)
- [x] `app/identity` and `app/tenancy` services with fakes: Register, Login, Refresh, Logout, SwitchTenant, password reset, accept invitation, CreateTenant, InviteMember, RevokeInvitation, ChangeRole, RemoveMember, UpdateSettings
- [x] `adapter/auth`: EdDSA JWT with `kid` rotation, opaque refresh tokens with rotation and reuse detection, Argon2id (ADR 0019; rotation and reuse detection live in `app/identity`, run here on the real tokens)
- [x] Migrations `0002_identity` (with `password_reset_codes`), `0003_workflows` (ADR 0006), `0004_tenancy` (with `invitations`, ADR 0011), `0005_rls_helpers`, `0006_platform`, `0007_rls_policies` (numbered as they land, ADR 0014)
- [x] `adapter/postgres`: `TxManager` that sets `app.tenant_id`, `errors.go` mapping, identity and tenancy repositories (`TxManager` with all three settings and savepoints, `errors.go`, the outbox publisher, and the identity and tenancy repositories on GORM models, all passing their port contracts; the locks of ADR 0013 tested with overlapping requests)
- [x] HTTP: `authn` middleware, cross-origin refusal on `/auth` (no CORS, ADR 0010), rate limits, auth, tenancy and members handlers, new problem codes; signing keys from config, with the development key refused outside `development` and `test` (ADR 0019) (plan steps 2.1 to 2.6: the middleware, the rate limits of ADR 0021, the eighteen handlers checked against `openapi.yaml`, the suspended-store check of ADR 0022, and the configuration that refuses every local-only setting outside development)
- [x] `adapter/notify/smtp.go` for invitations and password reset codes; Mailpit in the dev stack (ADR 0011) (Mailpit in the base stack at `MAILPIT_PORT`; `SMTP_REQUIRE_TLS=false` and an `http` admin origin are refused outside development)
- [x] OpenAPI: `/auth/*`, tenancy and members endpoints (ADRs 0010, 0011)
- [x] RLS test suite (five cases per tenant table) and auth security tests (expired, wrong `aud`, `alg: none`, unknown `kid`, refresh reuse) (the RLS suite with the user-scope policies of ADR 0010; the auth tests in `adapter/auth`, with forged and tampered tokens and fuzzing)
- [x] Contract suites for the identity and tenancy ports, run against fakes and real adapters (the fakes, the Postgres repositories, the inproc ports, the argon2id hasher, the secret generator and the token issuer)

### Web

- [x] `packages/api` auth client with refresh once on 401; session store (token in memory, refresh from cookie)
- [x] Login, sign-up, tenant picker, forgot password, accept invitation, route guards (`requiresAuth`, `permission`), `usePermission`
- [x] Members (`features/members`): list, invite, change role, remove

### Mobile

- [x] Ktor client with bearer-and-refresh plugin and problem+json mapping (`core:network`; `/auth/refresh` is its `TokenRefresher`, in `core:data`)
- [x] `TokenStore` (Keystore + DataStore) and auth graph before the management graph (`core:security`, `RootViewModel`)
- [x] Login, sign-up, tenant picker, forgot password screens, `PermissionGate` (plus logout and creating a store; the gate is fed with the stored session's permissions. Management destinations are filtered by permission, as the web's `meta.permission`, in the menu drawer of ADR 0020: Hoje is open to every role, Equipe needs `members:manage`)
- [x] `feature/members`: list, invite, change role, remove (Equipe in the menu for `members:manage`, ADR 0020)

### Suspended stores (added 2026-11-26, ADR 0022)

- [x] Backend: a suspended store is refused on every tenant-scoped admin route (403 `tenant_suspended`), never selected by login, refresh or switch, and shown in the membership list with `status`
- [ ] Web: "Escolha a loja" marks a suspended store "Suspensa", and `tenant_suspended` shows the message and goes back to the picker
- [ ] Android: the same

### Done when

- [x] A staff token cannot read another tenant's rows through the API or the database; an expired or tampered token is rejected on every route (`backend/test/e2e`: another store's token reaches nothing through the admin routes; expired, tampered, `alg: none`, unknown-key and wrong-audience tokens are 401 on every protected operation in `openapi.yaml`; the database half is the RLS suite)
- [ ] Sign-up → create tenant → log in works on web and Android against the same endpoints (partial: the web half passes against the real stack in `web/e2e/admin.spec.ts`, step 2.8; Android in step 2.11)
- [ ] Members: invite, change role and remove on web and Android (partial: the web half passes against the real stack, step 2.8; Android in step 2.11)
- [x] Coverage gate (90% domain + app, 80% overall) passes with real code behind it (the CI `coverage` job, on every milestone 2 PR)

---

## Milestone 3 — Catalog and suppliers

Goal: the owner can describe what they sell, where it comes from and what it costs; the store reads it fast.

### Backend

- [ ] `domain/catalog`: item, sourcing mode, category, attribute ("Serve em"), customization option, kit, kit swap (swap price rule tested)
- [ ] `domain/supplier`: supplier, supplier item (lead time, cost, backup supplier)
- [ ] `app/catalog` and `app/supplier` services, cached queries, fakes
- [ ] Before the first internal table (suppliers, costs): decide, in an ADR, an RLS tier that keeps internal tables out of scopes with no staff user, such as store page requests. Today a store page scope reads all of its store's rows, and only the queries keep internal data out (security review of `m2-db`)
- [ ] Migrations for catalog and suppliers, with RLS policies and tests
- [ ] `adapter/postgres` repositories; `adapter/redis` cache (tenant-prefixed keys, `g:` for globals, Redis-down fallback) with integration tests
- [ ] `adapter/storage`: local and S3-compatible photo storage
- [ ] HTTP handlers, OpenAPI entries, and the "clients never see costs or suppliers" visibility test

### Web

- [ ] `packages/api`: `catalog.ts`, `suppliers.ts`
- [ ] Catálogo screens: items, sourcing mode, supplier, cost, price, margin, "No portal", photos; Kits tab with swappable items

### Mobile

- [ ] `feature/catalogo`: items and kits, with offline cache of recent reads
- [ ] Fornecedores tab data layer (screens ship with Compras in milestone 5)

### Done when

- [ ] A catalog item with a supplier and lead time can be created on web and on Android and appears on the other
- [ ] Kit swap price and the Kit Scrambler CB 500X reference case are covered by tests

---

## Milestone 4 — Customers, orders and deadlines

Goal: the core of the product. An order moves through its store's status list, and the delivery date and delay alerts are always right.

### Backend

- [ ] `domain/workflow`: phases, status workflow, `validate.go`, four launch presets, each with its own test
- [ ] Migration `0003_workflows` with the preset seed
- [ ] `domain/customer`; customers migration
- [ ] `domain/order`: aggregate, items, item status per sourcing mode, customization snapshot, payments (deposit and balance), `Advance` with phase guards, tracking token, events
- [ ] `domain/schedule`: delivery date, arrival limit, `at_risk` and `late` alerts (pure functions, fuzz or table tests)
- [ ] Orders migration, with RLS and the client read policy (a client account reads only its own linked orders)
- [ ] `app/workflow`, `app/customer`, `app/order` (create, confirm deposit, release without deposit, advance, record balance, hand over, change delivery date → `wa.me` link, cancel), `app/schedule`
- [ ] Outbox (`outbox_repo`) in the same transaction as order writes
- [ ] Tracking tokens (`adapter/auth/tracking.go`); `adapter/notify/whatsapp_link.go`
- [ ] Idempotency-Key storage for POST endpoints
- [ ] HTTP handlers and OpenAPI: orders, customers, status workflows, payments, Hoje queries
- [ ] "New orders take the store's current workflow and keep it" use case test; client-visibility test on tracking responses

### Web

- [ ] Hoje (summary cards, orders by delivery date, "Precisa de você"), Pedidos (list by status, detail with item × deadline timeline), Clientes, Financeiro
- [ ] Workflow choice in Settings, with a preview of its steps

### Mobile

- [ ] `feature/hoje`, `feature/pedidos`, `feature/clientes`, `feature/financeiro`, workflow choice in `feature/settings`
- [ ] Offline cache of recent orders in Room

### Done when

- [ ] Every preset workflow has tests for each allowed and forbidden move and each phase guard
- [ ] Changing a delivery date returns a `wa.me` link and nothing is sent automatically

**Blocked on an open question:** payments are recorded manually first or integrate PIX from this milestone (architecture section 12). Build the manual path unless told otherwise, and ask before touching PIX.

---

## Milestone 5 — Purchasing and receiving

Goal: the business buys only what was sold, tells the supplier without leaking the client, and checks what arrives.

### Backend

- [ ] `domain/purchasing`: purchase line, supplier order, supplier message (never carries client name or phone, with test), receipt (`ok` / `defective`)
- [ ] Purchasing migration, with RLS and tests
- [ ] `app/purchasing`: `GeneratePurchases` (after deposit unless `purchase_before_deposit`), `SendSupplierOrder` → `wa.me` link, `RecordPromisedDate`, `Receive`
- [ ] Postgres repository (grouping by supplier with `Raw` where clearer); receipt photo upload through storage
- [ ] HTTP handlers and OpenAPI

### Web

- [ ] Compras: by supplier, send via `wa.me`, promised dates; Fornecedores tab
- [ ] Receber itens: checklist, item and invoice photos, Tudo certo / Com defeito

### Mobile

- [ ] `feature/compras` with the same flows, camera and photo picker (`expect`/`actual`)
- [ ] Offline receiving checklist, queued writes retried with WorkManager and an `Idempotency-Key`

### Done when

- [ ] Order with deposit paid → purchase lines appear → supplier message copied → items received on a phone with no signal → synced later

---

## Milestone 6 — Inventory and production

Goal: stores that keep stock or make things themselves get the same dates and alerts.

### Backend

- [ ] `domain/inventory` (stock item, append-only movements, reservation) and `domain/production` (job, stage, capacity, assignment)
- [ ] Inventory and production migrations, with RLS and tests
- [ ] `app/inventory` (receive, adjust, reserve, release, low-stock alerts) and `app/production` (start job, complete stage, assign, weekly capacity)
- [ ] `adapter/inproc/stock_reserver.go` implementing `order.StockReserver`
- [ ] `cmd/worker`: outbox relay and the daily Pode atrasar / Atrasada check
- [ ] HTTP handlers and OpenAPI; Produção and Estoque hidden when the tenant setting is off

### Web

- [ ] Estoque (list, adjust, movements, low-stock) and Produção (board, stages, capacity)

### Mobile

- [ ] `feature/estoque` and `feature/producao`, with the production board cached offline

### Done when

- [ ] An order mixing `to_order`, `stock` and `in_house` items gets a delivery date from the slowest item, on both clients

---

## Milestone 7 — Store page

Goal: a client opens `{slug}.encomendo.com.br`, builds an order, pays the deposit and follows it, without needing an account.

### Backend

- [ ] `domain/portal`: storefront, template, branding, sections, subdomain (reserved words blocked)
- [ ] Portal migration with the niche template seed (templates are data)
- [ ] `app/portal` and `app/storefront`: catalog, kits, Monte o seu preview (price, deposit and date calculated by the backend), place order, track by token
- [ ] `adapter/http/middleware/storefront.go`: tenant from the Host subdomain
- [ ] Client accounts: email code and Google sign-in (`adapter/auth/email_code.go`, `google.go`), `app/clientarea`, `/api/v1/me/*`, SMTP notifier
- [ ] Client-visibility tests across the whole store page, tracking and client area surface

### Web: store app

- [ ] Kits prontos, Monte o seu with live price, deposit and date, checkout, tracking (steps from the order's workflow, client labels)
- [ ] Optional login and Minhas encomendas across stores, also at `conta.encomendo.com.br`

### Web: admin and mobile

- [ ] Portal builder on web: template, logo, brand colour, cover photo, sections
- [ ] `feature/portal` on Android with a WebView preview of the store page

### Delivery

- [ ] Wildcard DNS and TLS for `*.encomendo.com.br`

### Done when

- [ ] A client places an order with no account, tracks it by link, and later adds it to Minhas encomendas

**Blocked on open questions:** store page SEO (Vite SPA or server rendering) and custom domains per tenant (architecture section 12). Ask before building either.

---

## Milestone 8 — Hardening

- [ ] Redis rate limits per IP and per user on every public route
- [ ] Audit log views for the owner (web and Android)
- [ ] API e2e suite: sign up → tenant → workflow → items and kit → order on the store page → deposit → purchases → receive → assemble → delivery or pickup (`test/e2e`)
- [ ] Playwright suites for the main admin and store flows against docker compose
- [ ] Compose UI tests for order and receiving screens on Android
- [ ] Nightly mutation testing on `internal/domain`
- [ ] Load test of store page reads
- [ ] Backup and restore drill, written up in `docs/`

---

## Cross-cutting tracks

These are checked in every milestone. Use them to spot a layer that is falling behind.

| Track | Status |
| --- | --- |
| Backend coverage gate (≥ 90% domain + app, ≥ 80% overall) | Script and CI job exist; there is almost no domain or app code to measure yet |
| Port contract suites (`test/contract/`) | Shared ports (`Cache`, `TxManager`, `EventPublisher`, `Clock`, `IDGenerator`) run against the fakes in `make test`; real adapters join as they land |
| RLS suite | Not started; arrives with the first tenant table in milestone 2 |
| `docs/api/openapi.yaml` kept in sync | Ahead of the server: milestone 2 `/auth/*`, tenancy and members are specified; only `/healthz` and `/readyz` are served until `m2-integrate` |
| Web types generated from OpenAPI | Working (`make api-types`, checked in `web.yml`) |
| Parity: every management feature on web and Android | Hoje, a placeholder on both; Equipe on both |
| Client visibility (no suppliers, costs, margins, "Pode atrasar" for clients) | Nothing to leak yet; the test lands with the first store response |
| ADRs | 0001–0020 written |

## Order of migrations

The architecture numbers the migrations in section 7, but `0004_tenancy` references `status_workflows` from `0003_workflows`, while the build order puts identity and tenancy (milestone 2) before workflows (milestone 4). One of two things has to give:

- Land `0003_workflows` (tables and the preset seed only) in milestone 2, with the workflow domain code following in milestone 4; or
- Make `tenant_settings.status_workflow_id` nullable in `0004` and add the foreign key when workflows land.

Decided in ADR 0006: ship `0003_workflows` (tables and seed) in milestone 2.

## Open questions

Do not decide these silently. Copied from `docs/architecture.md` section 12.

| Question | Blocks |
| --- | --- |
| Payments: record manually first, or PIX from milestone 4? | Milestone 4 |
| Store page SEO: is a Vite SPA enough? | Milestone 7 |
| Custom domains per tenant in v1? | Milestone 7 |
| Status workflows: presets only, or can stores rename or build their own? Confirm the preset wording with real shops. | Milestone 4 |

## Decisions log

| Date | Decision | Where |
| --- | --- | --- |
| — | Ports and adapters in a modular monolith | ADR 0001 |
| — | Tenant isolation with Postgres RLS | ADR 0002 |
| — | JWT access tokens with rotating opaque refresh tokens | ADR 0003 |
| — | Fixed order phases, store-chosen status lists | ADR 0004 |
| — | Room 3 and the Android-KMP library plugin | ADR 0005 |
| 2026-11-24 | Workflow tables and preset seed land in milestone 2 | ADR 0006 |
| 2026-11-24 | Docker Compose for development and deployment, all in `deploy/` | ADR 0007 |
| 2026-11-24 | Drop CodeQL and dependency review (they need GHAS on a private repo) | ADR 0008 |
| 2026-11-25 | `app/shared` imported as `appshared`; RLS scope in the context, derived from the principal | ADR 0009 |
| 2026-11-25 | Staff sessions may have no store; switching is a refresh; refresh token by cookie (web) or body (Android) | ADR 0010 |
| 2026-11-25 | Email in milestone 2: invitations by link, password reset by code; members can be removed | ADR 0011 |
| 2026-11-24 | A failed token refresh ends the session only when the server refuses it; offline keeps it | ADR 0012 |
| 2026-11-25 | A 2-minute retry window for lost refresh answers; server failures are never 401; Retry-After on every 429 | ADR 0013 |
| 2026-11-25 | Migrations take the next free number when they land; M2 platform and policies are 0006 and 0007 | ADR 0014 |
| 2026-11-25 | Android keeps the whole session (store, role, permissions) sealed with its tokens | ADR 0015 |
| 2026-11-25 | Android logout clears the device first, then tells the server without waiting | ADR 0016 |
| 2026-11-25 | Fake transactions undo the writes of repository fakes on failure; secrets are drawn through `appshared.SecretGenerator` | ADR 0009 (amended) |
| 2026-11-25 | `tenancy.Lookup` also uses invitations up for `identity`; accepting runs in three transactions | ADR 0011 (refined) |
| 2026-11-25 | Android requests stay in the store they were made for: same-scope retries, tokens only to screens of their scope | ADR 0018 |
| 2026-11-25 | Ed25519 seeds named by thumbprint, rotated in three deploys, never printed; `at+jwt` with a one-string `aud` and at most 15 minutes; argon2id at RFC 9106's second option, within a memory budget and a 3-second wait | ADR 0019 |
| 2026-11-25 | The Android management menu is a navigation drawer, its entries in the web's order and filtered by permission; a screen whose permission goes leaves for Hoje | ADR 0020 |
