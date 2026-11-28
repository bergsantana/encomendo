# Encomendo — Build Plan and Progress

Tracks how much of the system is built, milestone by milestone. The milestones and their order come from `docs/architecture.md` section 11; the scope of each item comes from sections 4, 7 and 8 of the same document. This file records **status only**. If it disagrees with the architecture or the product docs, those win.

**Last audited:** 2026-11-27, at the close of milestone 3. Run locally: backend lint (golangci-lint v2.15.0), unit, contract and integration tests (Postgres, RLS, Redis, and storage on Silo) pass; the coverage gate passes on the merged unit and integration profiles (94.6% overall, 96.6% domain and app); web lint, typecheck, unit tests and the Playwright suite against the real stack pass; mobile `./gradlew check` and `:androidApp:assembleDebug` pass in a container (`scripts/mobile-docker.sh`). CI is green on every milestone 3 PR (#52 to #71). **Not run:** the Android device walkthrough, which waits for KVM in the WSL VM.

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
| 2 | Identity and tenancy | 26 | 26 | Done: everything is built and checked against the real stack on web and Android, suspended stores included (#47) |
| 3 | Catalog and suppliers | 13 | 14 | Almost done: backend, web and the native Android screens are merged (PRs #52 to #72); the last box is checked in the Android shell in milestone 3.5 (ADR 0029) |
| 3.5 | Android shell | 4 | 6 | In progress: ADR 0029 accepted, every existing admin screen at phone size, and the Capacitor shell built; the release-build check and the device walkthrough remain |
| 4 | Customers, orders and deadlines | 9 | 17 | In progress: the contract (#78), the domain (#79), the migrations (#80) and the application layer are built |
| 5 | Purchasing and receiving | 0 | 9 | Not started |
| 6 | Inventory and production | 0 | 9 | Not started |
| 7 | Store page | 0 | 13 | Not started |
| 8 | Hardening | 0 | 9 | Not started |
|   | **All milestones** | **74** | **127** | **about 58%** (58.3%) |

Percentages count checkboxes, and milestones differ a lot in size (milestone 4 is the heart of the product), so read them as a rough guide, not a schedule.

### What exists today

- **Backend:** config, slog logger, HTTP server with the middleware chain (request ID, logging, recover, security headers, rate limits, authn), RFC 9457 problem responses, `/healthz` and `/readyz`, GORM with the pgx driver, Redis client and cache, goose migrations `0001`–`0011`, and the bounded contexts built so far:
  - `domain/shared`, `identity` and `tenancy` (milestones 1–2; the default status workflow is a reference in tenant settings until the workflow context lands in milestone 4), served through `/auth/*`, tenants, settings and members;
  - milestone 3: `domain/catalog` (items, sourcing mode, categories, "Serve em" fields, customization options, kits, kit swap, photos), `domain/supplier` (suppliers, supplier items with lead time, cost and a backup) and `domain/schedule` days; `app/catalog` and `app/supplier` with fakes and owner-only cost views; Postgres repositories with staff-only RLS for internal tables; a Redis-backed cache with tenant-prefixed keys; `adapter/storage`, the S3-compatible photo store with signed uploads staged under `uploads/` (R2 in production, Silo in development and CI); and the catalog and supplier HTTP operations.
- **Web:** pnpm workspace with `admin` and `store` apps, `packages/api` (fetch wrapper, generated types, auth, tenancy, members, catalog, suppliers and the photo upload) and `packages/ui`. Admin has login, sign-up, the store picker, Hoje, Equipe and Catálogo (Itens with categories and fields, Kits with swappable items, photos, "No portal"; Custo and Margem only for the owner). Playwright runs the admin and Catálogo flows against the real stack. The store app resolves the tenant from the subdomain and renders a shell.
- **Mobile:** KMP shell with the milestone 2 session, auth and Equipe screens, plus `feature/catalogo`: the Itens list and form, the Kits list and editor, and photo upload and thumbnails (Coil with a bearer-less client). `core:data` caches recent catalog reads in Room per store, keeps costs apart by role, and Sair clears it (ADR 0026). The Fornecedores data layer is built; its screens come with Compras.
- **Delivery:** `docker compose` stack (postgres, redis, migrate, api, web, Mailpit and Silo for photos), API and web Dockerfiles, five GitHub workflows, Dependabot, PR template, ADRs 0001–0028, an OpenAPI file covering the health endpoints, milestone 2 and the whole milestone 3 catalog and supplier API (all served). `scripts/android-emulator-docker.sh` runs the Android walkthroughs in Docker.

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
- [x] `domain/identity` (user, client account, email, password policy, password reset code, role, permission) and `domain/tenancy` (tenant, membership, invitation, settings), with tests (the tenant plan moved to billing, an open question in `docs/architecture.md` section 12: no product document defines plans; decided by the user on 2026-11-26)
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
- [x] Web: "Escolha a loja" marks a suspended store "Suspensa", and `tenant_suspended` shows the message and goes back to the picker
- [x] Android: the same

### Done when

- [x] A staff token cannot read another tenant's rows through the API or the database; an expired or tampered token is rejected on every route (`backend/test/e2e`: another store's token reaches nothing through the admin routes; expired, tampered, `alg: none`, unknown-key and wrong-audience tokens are 401 on every protected operation in `openapi.yaml`; the database half is the RLS suite)
- [x] Sign-up → create tenant → log in works on web and Android against the same endpoints (web: `web/e2e/admin.spec.ts` against the real stack, step 2.8; Android: walked on an emulator in Docker against the same stack, step 2.11, with `scripts/android-emulator-docker.sh`)
- [x] Members: invite, change role and remove on web and Android (web: step 2.8; Android: invite, change role and remove walked on the emulator against the real stack, step 2.11)
- [x] Coverage gate (90% domain + app, 80% overall) passes with real code behind it (the CI `coverage` job, on every milestone 2 PR)

---

## Milestone 3 — Catalog and suppliers

Goal: the owner can describe what they sell, where it comes from and what it costs; the store reads it fast.

Plan: `docs/plans/m3-catalog-suppliers.md` (steps 3.1–3.17). Decisions: ADR 0024 and ADR 0025.

### Backend

- [x] `domain/catalog`: item, sourcing mode, category, attribute ("Serve em"), customization option, kit, kit swap (each kit keeps or loses its discount on a swap, both modes tested; ADR 0023) (step 3.3)
- [x] `domain/supplier`: supplier, supplier item (lead time, cost, backup supplier) (#54; `schedule.Days` too)
- [x] `app/catalog` and `app/supplier` services, cached queries, fakes (#61, step 3.6)
- [x] Before the first internal table (suppliers, costs): decide, in an ADR, an RLS tier that keeps internal tables out of scopes with no staff user, such as store page requests. Today a store page scope reads all of its store's rows, and only the queries keep internal data out (security review of `m2-db`) (decided in ADR 0024: staff-only policies and item costs in their own table; built in `0008_catalog` and `0009_suppliers`, step 3.4)
- [x] Migrations for catalog and suppliers, with RLS policies and tests (`0008_catalog`, `0009_suppliers`; the RLS suite's sixth, staff-only case; step 3.4)
- [x] `adapter/postgres` repositories; `adapter/redis` cache (tenant-prefixed keys, `g:` for globals, Redis-down fallback) with integration tests
- [x] `adapter/storage`: S3-compatible photo storage (Cloudflare R2 in production, ADR 0023; signed `PUT` URLs and a confirm step, MinIO in dev and tests, so no local storage, ADR 0024) (step 3.9; Silo, the MinIO fork, replaces MinIO since its images left Docker Hub; ADR 0028)
- [x] HTTP handlers, OpenAPI entries, and the "clients never see costs or suppliers" visibility test (in M3: RLS sixth case, public view types, no cost or margin for non-owners; the store-route part lands with the first store route in M7, ADR 0025)

### Web

- [x] `packages/api`: `catalog.ts`, `suppliers.ts` (#55)
- [x] Catálogo screens: items, sourcing mode, supplier, cost, price, margin, "No portal", photos; Kits tab with swappable items (#59, step 3.13; checked against the real stack by the Playwright flow in `web/e2e/catalogo.spec.ts`, step 3.17)

### Mobile

- [x] `feature/catalogo`: items and kits, with offline cache of recent reads (#64, step 3.16; the emulator walkthrough against the real stack is pending KVM, step 3.17)
- [x] Fornecedores tab data layer (screens ship with Compras in milestone 5) (#57)

### Follow-ups (not milestone boxes)

- Milestone 2's `nullableString` in `adapter/http/handler/dto/tenancy.go` can move to `dto.Optional[string]`, the type the catalog requests use to tell a missing field from a null.

### Done when

- [ ] A catalog item with a supplier and lead time can be created in the web admin and in the Android shell and appears on the other (reworded by ADR 0029; partial: the web side is checked, since a Playwright flow creates an item with a new supplier, a lead time, a cost and a photo against `make stack-up`, and a second flow shows an item created through the API in the web list; the shell walkthrough is step 3.5.5)
- [x] Kit swap price and the Kit Scrambler CB 500X reference case are covered by tests (`domain/catalog`, `domain/schedule`)

---

## Milestone 3.5 — Android shell

Goal: the Android management app is a Capacitor shell that opens the hosted web admin (ADR 0029), and every existing admin screen works at phone size. Plan: `docs/plans/m3_5-android-shell.md`.

- [x] ADR 0029 accepted, and the docs follow it: architecture 8.2 and 8.4, rule 6 in `CLAUDE.md`, `mobile/` frozen, and this plan
- [x] `web/apps/android`: the Capacitor shell, with app id `br.com.encomendo.app`, the hosted admin URL per build, navigation limited to the admin host, external links (`wa.me`, store pages) opened outside the app, the back button, and release signing read from outside the repository (step 3.5.2; hardened after the security review: Capacitor's native HTTP, cookie and WebView plugins replaced by inert stubs, file routes and foreign frames refused, old WebViews refused)
- [x] Phone layouts: login, sign-up, Esqueci a senha, the store picker, the menu, Hoje and Equipe at 390 × 844 (step 3.5.3)
- [x] Phone layouts: Catálogo, covering Itens, Kits, the drawers and dialogs, and photos from the camera or gallery through the shell (step 3.5.4)
- [ ] A signed release APK can be built and installed by following `web/apps/android/README.md`; CI builds the shell (that needs the user's workflow edit)

### Done when

- [ ] Milestone 3's flows run in the shell on a phone or emulator against the local stack, and an item created in the shell appears on the web, and the other way round

---

## Milestone 4 — Customers, orders and deadlines

Goal: the core of the product. An order moves through its store's status list, and the delivery date and delay alerts are always right.

Plan: `docs/plans/m4-customers-orders-deadlines.md` (steps 4.1–4.23). Decisions: ADR 0030.

### Backend

- [x] `domain/workflow`: phases, status workflow, `validate.go`, four launch presets, each with its own test (step 4.2)
- [x] Migration `0003_workflows` with the preset seed (shipped in M2, ADR 0006; step 4.8 loads every seeded preset through `workflow.New` and checks it against `testkit.PresetSpecs`)
- [x] `domain/customer`; customers migration (steps 4.4 and 4.7: `0012_customers`)
- [x] `domain/order`: aggregate, items, item status per sourcing mode, customization snapshot, payments (deposit and balance), `Advance` with phase guards, tracking token, events (steps 4.5 and 4.6)
- [x] `domain/schedule`: delivery date, arrival limit, `at_risk` and `late` alerts (pure functions, fuzz or table tests) (step 4.3)
- [x] Orders migration, with RLS and the client read policy (a client account reads only its own linked orders) (step 4.7: `0013_orders`, `0014_idempotency`, `0015_settings_time_zone`; staff-only sourcing and messages)
- [x] `app/workflow`, `app/customer`, `app/order` (create, confirm deposit, release without deposit, advance, record balance, hand over, change delivery date → `wa.me` link, cancel), `app/schedule` (steps 4.8 to 4.11; `schedule.Recalculate` waits for M5's promised dates to redate)
- [ ] Outbox (`outbox_repo`) in the same transaction as order writes
- [ ] Tracking tokens (`adapter/auth/tracking.go`); `adapter/notify/whatsapp_link.go`
- [ ] Idempotency-Key storage for POST endpoints
- [ ] HTTP handlers and OpenAPI: orders, customers, status workflows, payments, Hoje queries
- [x] "New orders take the store's current workflow and keep it" use case test; client-visibility test on tracking responses (step 4.10: `TestCreate_TakesTheCurrentWorkflowAndKeepsItAfterTheOwnerPicksAnother`, `TestTrackingView_HasNoInternalField`; the HTTP half joins in 4.14)

### Web

- [ ] Hoje (summary cards, orders by delivery date, "Precisa de você"), Pedidos (list by status, detail with item × deadline timeline), Clientes, Financeiro
- [ ] Workflow choice in Settings, with a preview of its steps

### Phone (the web admin at phone size, opened by the Android shell; ADR 0029)

- [ ] Hoje, Pedidos, Clientes, Financeiro and the workflow setting work at phone size

### Done when

- [x] Every preset workflow has tests for each allowed and forbidden move and each phase guard (`domain/workflow` `TestPresets_Moves`, `domain/order` `TestAdvance_<Preset>_EveryMove_WithGuardHeldAndBroken`)
- [ ] Changing a delivery date returns a `wa.me` link and nothing is sent automatically

**Answered (ADR 0023):** the owner records payments by hand, with no PIX in v1. Stores pick one of the four presets and cannot rename statuses. The deposit is 50% by default, assembly 3 days and safety margin 2 days.

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

### Phone (the web admin at phone size, opened by the Android shell; ADR 0029)

- [ ] Compras and receiving work at phone size, with workshop sizes on the receiving screens, and item and invoice photos from the camera through the shell

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

### Phone (the web admin at phone size, opened by the Android shell; ADR 0029)

- [ ] Estoque and Produção work at phone size, with the production board at workshop sizes

### Done when

- [ ] An order mixing `to_order`, `stock` and `in_house` items gets a delivery date from the slowest item, on desktop and in the Android shell

---

## Milestone 7 — Store page

Goal: a client opens `{slug}.encomendo.com.br`, builds an order, pays the deposit and follows it, without needing an account.

### Backend

- [ ] `domain/portal`: storefront, template, branding, sections, subdomain (reserved words blocked)
- [ ] Portal migration with the niche template seed (templates are data)
- [ ] `app/portal` and `app/storefront`: catalog, kits, Monte o seu preview (price, deposit and date calculated by the backend), place order, track by token
- [ ] `adapter/http/middleware/storefront.go`: tenant from the Host subdomain, and a test that store page routes attach client principals only, so a staff token on a store page route is never taken for one (security review of steps 3.4 and 3.7)
- [ ] Client accounts: email code and Google sign-in (`adapter/auth/email_code.go`, `google.go`), `app/clientarea`, `/api/v1/me/*`, SMTP notifier
- [ ] Client-visibility tests across the whole store page, tracking and client area surface
- [ ] Serve photos publicly for published items and kits: stable public URLs or short-lived signed ones the store page can cache (milestone 3 signs URLs per admin caller only, ADR 0027), with a test that an unpublished or archived item's photos never leave the admin routes

### Web: store app

- [ ] Kits prontos, Monte o seu with live price, deposit and date, checkout, tracking (steps from the order's workflow, client labels)
- [ ] Optional login and Minhas encomendas across stores, also at `conta.encomendo.com.br`

### Web: admin, desktop and phone

- [ ] Portal builder on web: template, logo, brand colour, cover photo, sections
- [ ] The portal builder works at phone size, and its live preview of the store page works inside the Android shell

### Delivery

- [ ] Wildcard DNS and TLS for `*.encomendo.com.br` (Cloudflare DNS, Caddy DNS-01 with the Cloudflare module, ADR 0023)

### Done when

- [ ] A client places an order with no account, tracks it by link, and later adds it to Minhas encomendas

**Answered (ADR 0023):** the store page is a Vite SPA, and the server fills in each store's title, description and Open Graph tags. A store page is only `{slug}.encomendo.com.br`, with no custom domains in v1. Google sign-in ships with the email code.

---

## Milestone 8 — Hardening

- [ ] Redis rate limits per IP and per user on every public route
- [ ] Audit log views for the owner (desktop and phone)
- [ ] API e2e suite: sign up → tenant → workflow → items and kit → order on the store page → deposit → purchases → receive → assemble → delivery or pickup (`test/e2e`)
- [ ] Playwright suites for the main admin and store flows against docker compose
- [ ] Playwright flows at phone size for the order and receiving screens, and a smoke test of the Android shell
- [ ] Nightly mutation testing on `internal/domain`
- [ ] Load test of store page reads
- [ ] Backup and restore drill, written up in `docs/`
- [ ] Store names sort the way Portuguese sorts them: a pt-BR collation in the repository and the fake, and the order stated in the port contract suite (milestone 2 kept byte order: "Água" after "Zebra"; decided by the user on 2026-11-26)

---

## Cross-cutting tracks

These are checked in every milestone. Use them to spot a layer that is falling behind.

| Track | Status |
| --- | --- |
| Backend coverage gate (≥ 90% domain + app, ≥ 80% overall) | Script and CI job exist and pass with milestone 2 and 3 code behind them (see "Last audited" for the numbers) |
| Port contract suites (`test/contract/`) | Shared, identity, tenancy, supplier and catalog ports (items, kits, `Photos` and `PhotoStore`) run against the fakes in `make test` and against the real adapters (Postgres, Redis, Silo) in `make test-integration` |
| RLS suite | Running since milestone 2; milestone 3 added the staff-only tier (`item_costs`, `suppliers`, `supplier_items`, and `audit_log` reads) as its sixth case |
| `docs/api/openapi.yaml` kept in sync | In sync with the server through milestone 3: `/auth/*`, tenancy, members, catalog, suppliers and photos are all served |
| Web types generated from OpenAPI | Working (`make api-types`, checked in `web.yml`) |
| Management on every device: each admin screen at desktop and phone size, opened by the Android shell (ADR 0029) | Hoje, Equipe and Catálogo on desktop; their phone layouts and the shell are milestone 3.5. The native app (`mobile/`) is frozen with Hoje, Equipe and Catálogo built |
| Client visibility (no suppliers, costs, margins, "Pode atrasar" for clients) | The milestone 3 tests: public view types carry no cost or margin, the RLS staff-only case reads zero rows in a store page scope, and costs reach the owner only. The store page routes and their end-to-end test land in milestone 7 |
| ADRs | 0001–0028 written |

## Order of migrations

The architecture numbers the migrations in section 7, but `0004_tenancy` references `status_workflows` from `0003_workflows`, while the build order puts identity and tenancy (milestone 2) before workflows (milestone 4). One of two things has to give:

- Land `0003_workflows` (tables and the preset seed only) in milestone 2, with the workflow domain code following in milestone 4; or
- Make `tenant_settings.status_workflow_id` nullable in `0004` and add the foreign key when workflows land.

Decided in ADR 0006: ship `0003_workflows` (tables and seed) in milestone 2.

## Open questions

Do not decide these silently. Copied from `docs/architecture.md` section 12. All four were answered on 2026-11-26 (ADR 0023).

| Question | Answer |
| --- | --- |
| Payments: record manually first, or PIX from milestone 4? | Recorded by hand; no PIX in v1 |
| Store page SEO: is a Vite SPA enough? | Vite SPA, with the store's title, description and Open Graph tags set by the server |
| Custom domains per tenant in v1? | No; `{slug}.encomendo.com.br` only |
| Status workflows: presets only, or can stores rename or build their own? Confirm the preset wording with real shops. | Presets only; the wording is still to be checked with real shops |
| Offline use of the management app: cached orders, an offline receiving checklist and production board, queued writes (was M4 to M6) | Dropped for v1 by ADR 0029; to decide after v1, as a service worker or a bundled shell with its own ADR |

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
| 2026-11-26 | Client IP behind the proxies, rate-limit numbers, keys and failing open | ADR 0021 |
| 2026-11-26 | A suspended store is refused on every admin request at once and shown in the picker as "Suspensa" | ADR 0022 |
| 2026-11-26 | The tenant plan moves to billing, an open question; milestone 2 has none | `docs/architecture.md` section 12 |
| 2026-11-25 | Fake transactions undo the writes of repository fakes on failure; secrets are drawn through `appshared.SecretGenerator` | ADR 0009 (amended) |
| 2026-11-25 | `tenancy.Lookup` also uses invitations up for `identity`; accepting runs in three transactions | ADR 0011 (refined) |
| 2026-11-25 | Android requests stay in the store they were made for: same-scope retries, tokens only to screens of their scope | ADR 0018 |
| 2026-11-25 | Ed25519 seeds named by thumbprint, rotated in three deploys, never printed; `at+jwt` with a one-string `aud` and at most 15 minutes; argon2id at RFC 9106's second option, within a memory budget and a 3-second wait | ADR 0019 |
| 2026-11-25 | The Android management menu is a navigation drawer, its entries in the web's order and filtered by permission; a screen whose permission goes leaves for Hoje | ADR 0020 |
| 2026-11-26 | v1 scope for milestones 3–7: payments by hand, preset workflows only, Vite SPA with server-set meta tags, subdomains only, per-kit swap discount, R2 photos, Cloudflare DNS-01, Google sign-in in M7 | ADR 0023 |
| 2026-11-26 | Milestone 3: internal tables staff-only in RLS, item costs in their own table; stores create categories and fields; options are choices with extra price and days; photos by signed URL to R2, MinIO in dev and tests | ADR 0024 |
| 2026-11-26 | Milestone 3 plan: costs and margins owner-only (`identity.SeesCosts`), `catalog:read` for every role, cost per supplier item, catalog writes item sourcing (no cycle with `supplier`), `items.ready_days` for the store page, owner-picked swap alternatives, attributes as value lists, archive only, suppliers created inside the item form | ADR 0025 |
| 2026-11-27 | Android catalog cache: the API's JSON per store and per costs view, offline fallback on `Failure.Offline` only, no offline writes, Sair clears it; photo bytes and thumbnails go through a client with no bearer | ADR 0026 |
| 2026-11-27 | Catalog use case choices: an alternative counts as "in the kit", refusals name rows, tri-state cost fields, ids kept only when the row's own, names ordered by `lower(name)`, store page cached 5 minutes | ADR 0027 |
| 2026-11-27 | Photo uploads are staged under `uploads/` and promoted after the confirm check; one lifecycle rule; the storage adapter returns typed errors and signs for the public endpoint | ADR 0028 |
| 2026-11-27 | The user's decision: Silo, the maintained MinIO fork, replaces MinIO in compose, CI and tests since MinIO's images left Docker Hub; pinned by digest, the service keeps the name `minio`, the apps see only an S3 host; R2 stays the production store | ADR 0024 (amended), ADR 0028 |
| 2026-11-27 | `audit_log` reads are staff-only (a scope with a tenant and no user reads nothing); inserts keep the tenant check; internal tables stay out of the audit until audit views hide costs (milestone 8) | migration `0010_audit_log_staff_reads` (security review of step 3.4) |
| 2026-11-27 | The user's decision: the Android management app becomes a Capacitor shell that opens the hosted web admin (`web/apps/android`, app id `br.com.encomendo.app`, private signed APKs first, then the Play Store); the native `mobile/` app is frozen; rule 6 now means every admin screen works at desktop and phone sizes; offline use is dropped for v1; milestone 3.5 builds it | ADR 0029 |
| 2026-11-27 | Milestone 4 order model: staff create orders in the admin; kit lines; staff-only `order_item_sourcing`; promise vs forecast dates in a per-store time zone (the user's choice); items moved by hand until M5 and stock items at lead time until M6 (the user's choice); payments by hand with voids; a new tracking link on every send (the user's choice); `Idempotency-Key` in Postgres in the write's transaction; the user's choices on roles (`order:manage`, `order:override`, `customer:write`), extras (Frete, Desconto, Sinal per order) and changes to paid orders (seller-confirmed approval, before Montagem, "Valor a devolver") | ADR 0030 |
