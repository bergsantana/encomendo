# Milestone 4 — Customers, orders and deadlines

Written 2026-11-27 on Opus with `/plan-milestone 4`, against `main` at `b83f31e`. Build each step with `/build-step 4.<n>`.

Goal (build plan): the core of the product. An order moves through its store's status list, and the delivery date and delay alerts are always right.

## Blocked

Nothing. The section 12 questions that touch milestone 4 (payments, preset-only workflows) were answered in ADR 0023: payments by hand, no PIX, presets only, deposit 50%, assembly 3 days, margin 2 days.

Milestone 3.5 still has two open boxes (the signed release APK and the shell walkthrough). They don't block M4: step 4.23 runs the shell walkthrough for M4's screens and can close 3.5's at the same time.

## The user's answers (2026-11-27)

| # | Question | Answer |
| --- | --- | --- |
| Q1 | How do items move before Compras (M5) and Estoque/Produção (M6)? | By hand on the order screen; M5 then drives the same moves |
| Q2 | How does a stock item count before Estoque? | Its supplier lead time (conservative); M6 adds "ready today if on hand" |
| Q3 | How is a tracking link sent again, given tokens are stored only as hashes? | Each send issues a new link; the previous one stops working |
| Q4 | Which time zone decides "today"? | A per-store setting, default `America/Sao_Paulo` |
| Q5 | Who records a payment? | Owner, admin and staff (`order:write`); voiding stays the owner's |
| Q6 | Who cancels or changes the promised date? | Owner and admin (`order:manage`) |
| Q7 | Which extras does an order have? | Frete, a manual Desconto, and a Sinal percent per order |
| Q8 | Can a paid order change? | Yes, when customer and seller agree: the seller confirms the customer's approval, a `wa.me` summary goes to the customer, only before Montagem; a later forecast moves the promise; a lower total shows "Valor a devolver" |
| Q9 | May Claude merge its own M4 PRs? | Yes, once CI is green and the reviewers find nothing blocking. CI workflow and scanner changes still need the user's yes |
| Q10 | When is the M4 wording reviewed? | In this plan's PR: drafted now in `docs/product/design-document.md` (step 4.15 is done here) |
| Q11 | How do the opus and sonnet steps run in one session? | The main session runs on Opus and builds the opus steps; Sonnet subagents in their own worktrees build the sonnet steps |
| Q12 | How big are the PRs? | Grouped: eight PRs (below), not one per step |

The code-level design every step builds from is in `docs/plans/m4-code-design.md`: packages, types and signatures, SQL, endpoints, problem codes, configuration, web files and test names. A step that finds it wrong stops and says so rather than redesigning.

## How the session runs

The whole milestone runs in **one session, without stopping to ask**. Every question was answered above or is settled in ADR 0030 and the code design. If something comes up that neither covers, the session takes the option that keeps the dependency rule, tenant isolation and testability, writes it down as an amendment to ADR 0030, and carries on. It stops only for a decision that can't be undone and could reasonably go either way.

- **Models.** The main session runs on Opus (`claude --model opus`). It builds the opus steps itself and gives each sonnet step to a subagent with `model: sonnet`, `isolation: worktree`, the step's section of this plan and of the code design, and the `/build-step` instructions. The main session reviews what comes back before it goes into the PR.
- **PRs.** Eight, in order. Each PR is green on its own (`make lint test`, plus `make test-integration` when adapters change), runs every reviewer its steps list, ticks its boxes in `docs/build-plan.md`, and is merged by Claude once CI is green and the reviewers find nothing blocking (Q9). The next PR branches from the updated `main`.

| PR | Steps | Branch | Runs on |
| --- | --- | --- | --- |
| A | 4.1 contract | `feat/m4-contract` | Opus |
| B | 4.2–4.6 domain | `feat/m4-domain` | 4.2–4.4 Sonnet subagents in parallel; 4.5–4.6 Opus |
| C | 4.7 migrations and RLS | `feat/m4-migrations` | Opus |
| D | 4.8–4.11 app layer | `feat/m4-app` | 4.8, 4.9, 4.11 Sonnet; 4.10 Opus |
| E | 4.12–4.14 adapters, HTTP, wiring, API e2e | `feat/m4-adapters-http` | 4.12, 4.14 Sonnet; 4.13 Opus |
| F | 4.16–4.18 web API, Configurações, Clientes | `feat/m4-web-settings-clientes` | Sonnet subagent, started once A is merged, alongside B–E |
| G | 4.19–4.22 web Pedidos, order detail, Hoje, Financeiro | `feat/m4-web-pedidos` | Sonnet subagent, after F |
| H | 4.23 real stack, phone, shell, close | `feat/m4-close` | Opus, with a Sonnet subagent for the Playwright specs |

F and G build against MSW from the contract, so they don't wait for the backend; H checks them against the real stack.

- **CI workflow edits** (`.github/workflows/`) still need the user's yes (Q9). M4 needs none; if a step finds it does, it writes the change in its PR description and carries on.
- **Android shell walkthrough.** H needs KVM. If it is still missing, H records the walkthrough as pending, as 3.17 did, and the milestone closes with that one box partial.

## Decisions in this plan

ADR 0030 records them. The short form:

- **Staff create orders in the admin** ("Novo pedido"). The same `app/order` create and quote code serves the M7 store page.
- **A kit is one priced line holding one order item per kit item**, each with its own status and date. Orders are numbered per store, `P-0001`.
- **Internal snapshots are staff-only.** Supplier and cost at order time live in `order_item_sourcing` (staff-only RLS, no client policy, no audit), so the client read policy on `order_items` can never reach them.
- **Promise vs forecast.** `delivery_date` is the promise: computed at creation and again at the deposit or release, then moved only by `ChangeDeliveryDate` (history + `wa.me`). Item expected dates are the forecast. Alerts compare the forecast to promise − assembly days, computed on read with today in the store's time zone.
- **Calendar days**, civil dates, four Brazilian zones; `time/tzdata` embedded.
- **Payments by hand**, several per order, with a method; the owner voids a wrong one.
- **Idempotency in Postgres**, in the same transaction as the write (not Redis, as architecture 4.4 had it).
- **New permissions:** `order:manage` (owner, admin: cancel, date, changes after a payment, discount, deposit percent), `order:override` (owner: release without the deposit, void a payment), `customer:write` (owner, admin). Customers created inside an order need only `order:write`.
- **Extras:** Frete, Desconto, Sinal per order.
- **Changes** before any payment by `order:write`; after a payment by `order:manage`, with the seller confirming the customer's approval, before Montagem.
- **Financeiro is owner-only** (`finance:read`).
- **Hand-offs:** M5 calls purchasing from `Cancel` and decides whether manual moves of bought items stay; M6 adds stock reservation, capacity waits and the daily check.

## Ownership

This session is the only milestone 4 session. It owns `backend/cmd/api/main.go`, `backend/internal/adapter/http/router.go`, `docs/api/openapi.yaml` and the migrations folder. If parallel sessions start:

- 4.1 owns `openapi.yaml` (later changes go through 4.14);
- 4.7 owns the migrations;
- 4.14 owns `router.go` and `main.go`.

## Steps

| Step | Goal | Model |
| --- | --- | --- |
| 4.1 | Contract: workflows, settings, customers, orders, payments, Hoje, Financeiro; new permission and policy | opus |
| 4.2 | `domain/workflow`: phases, workflows, validation, moves, the four presets | sonnet |
| 4.3 | `domain/schedule`: civil dates, ready dates, delivery, arrival limit, alerts | sonnet |
| 4.4 | `domain/customer` | sonnet |
| 4.5 | `domain/order` I: aggregate, lines, snapshots, item lifecycles, pricing | opus |
| 4.6 | `domain/order` II: payments, `Advance` with phase guards, dates, cancel, tracking token, events | opus |
| 4.7 | Migrations: customers, orders, idempotency, settings time zone; RLS with client and staff-only tiers | opus |
| 4.8 | `app/workflow`, workflow repository, `ChooseStatusWorkflow`, time zone setting, `WorkflowSource` | sonnet |
| 4.9 | `app/customer` and its repository | sonnet |
| 4.10 | `app/order` writes: create, quote, edit, deposit, release, advance, items, balance, handover, date, cancel, link | opus |
| 4.11 | `app/order` queries and `app/schedule`: lists, detail, Hoje, Financeiro | sonnet |
| 4.12 | `adapter/postgres` order repository and the outbox in the same transaction | sonnet |
| 4.13 | Tracking tokens, `wa.me` links and `Idempotency-Key` storage | opus |
| 4.14 | HTTP handlers, routes, wiring, problem codes and the visibility tests | sonnet |
| 4.15 | pt-BR wording for Configurações, Clientes, Pedidos, Hoje and Financeiro | done in the plan PR |
| 4.16 | Web `packages/api`: `workflows.ts`, `settings.ts`, `customers.ts`, `orders.ts` | sonnet |
| 4.17 | Web Configurações: workflow choice with preview, defaults, time zone | sonnet |
| 4.18 | Web Clientes | sonnet |
| 4.19 | Web Pedidos: list by status and Novo pedido | sonnet |
| 4.20 | Web order detail: timeline, items, payments, moves, date change, link, cancel | sonnet |
| 4.21 | Web Hoje | sonnet |
| 4.22 | Web Financeiro | sonnet |
| 4.23 | Real stack on desktop, phone and the Android shell; close the milestone | sonnet |

**Order.**

- 4.2, 4.3 and 4.4 are pure and run in parallel once 4.1 is merged (the parallel-sessions wave `m4-workflow`, `m4-schedule`, `m4-customer`).
- 4.5 needs 4.2 and 4.3; 4.6 needs 4.5; 4.7 needs 4.4 and 4.6, so the columns follow the domain.
- 4.8 and 4.9 need 4.7; 4.10 needs 4.6, 4.8 and 4.9; 4.11 needs 4.10; 4.12 and 4.13 need 4.10; 4.14 needs everything above.
- On the web side, 4.15 is done in the plan PR, and 4.16 starts once 4.1 is merged. 4.17 to 4.22 follow 4.16 against MSW, so they can run while the backend is built; each is checked against the real stack in 4.23.

**Models.** Steps marked opus need `/model opus` or `claude --model opus`: 4.1, 4.5, 4.6, 4.7, 4.10 and 4.13.

**Phone size.** Every web step builds its screen at desktop and at 390 × 844 (`web/CLAUDE.md`, Phone layouts). The build plan's phone box ticks in 4.23, after the phone Playwright runs and the shell walkthrough.

---

### 4.1 Contract

- **Goal:** the M4 API in `docs/api/openapi.yaml`, so backend and web build against one contract; the permission and policy it names exist in `domain/identity`.
- **Ticks:** nothing alone; part of "HTTP handlers and OpenAPI …", which ticks in 4.14.
- **Model:** opus. The contract decides which fields reach which role, and which client-facing types exist.
- **Skill:** none. `/new-endpoint` runs per endpoint from 4.14.
- **Files:** `docs/api/openapi.yaml` (owned); `backend/internal/domain/identity/permission.go` + test; `make api-types` output.
- **Work:**
  - **Permissions:** `order:manage` and `customer:write` for owner and admin; `order:override` for owner only. Table tests over the four roles and an unknown one.
  - **Paths, all under `/api/v1/admin`, tenant-scoped:**
    - Workflows: `GET /status-workflows` (presets with statuses: key, label, client label, phase, position); `PUT /settings/status-workflow` `{workflowId, version}`.
    - Settings: `timeZone` joins `EditableSettings` (enum of the four zones).
    - Customers: `GET` (cursor, `limit`, `q` on name or phone) and `POST /customers`; `GET` and `PUT /customers/{id}` (`version`); addresses inside the customer body.
    - Orders:
      - `POST /orders/quote`: lines in, `{total, depositDue, deliveryDate, items[{expectedDate}]}` out, no write (the M7 Preview reuses the shape);
      - `POST /orders` (requires `Idempotency-Key`), returning the order plus `trackingUrl` once;
      - `GET /orders` (cursor, `limit`, `status`, `phase`, `customerId`, `q` on number or customer, `alert`);
      - `GET /orders/{id}`; `PUT /orders/{id}` (lines, handover, address, frete, desconto, sinal percent, days, `customerApproved`, `version`) → order + `whatsappUrl` when the order was already paid;
      - `POST /orders/{id}/advance` `{to, handover?, version}`;
      - `POST /orders/{id}/release` (owner);
      - `POST /orders/{id}/items/{itemId}/move` `{to, check?: ok|defective, version}`;
      - `POST /orders/{id}/items/{itemId}/use-backup-supplier` `{version}` (`order:manage`);
      - `POST /orders/{id}/payments` (requires `Idempotency-Key`) `{kind, amount, method}`; `POST /orders/{id}/payments/{paymentId}/void` `{reason}` (owner);
      - `PUT /orders/{id}/delivery-date` `{date, version}` → order + `{whatsappUrl}`;
      - `POST /orders/{id}/tracking-link` → `{trackingUrl, whatsappUrl}`;
      - `POST /orders/{id}/cancel` `{reason, version}`;
      - `GET /orders/{id}/history`.
    - Hoje: `GET /today` → summary counts, orders by promise date with items ready/total and alerts, "Precisa de você" entries (each with a reason and the order).
    - Financeiro: `GET /finance/summary?from=&to=` (owner).
  - **Schemas:**
    - `OrderStatus` `{key, label, clientLabel, phase}` on every order, so clients never hold a copy of the list.
    - `Order`: number, customer (id, name, phone), status, workflow `{id, version, name, statuses}`, lines, items `{id, name, quantity, unitPrice, sourcingMode, options, status, expectedDate, arrived, check, alerts[]}`, subtotal, freight, discount, total, depositPercent, depositDue, depositPaid, balanceDue, refundDue, payments, deliveryDate, arrivalLimit, assemblyDays, safetyMarginDays, handover, released, version; plus owner-only `cost` and `margin`, **not required**, absent for other roles.
    - `Alert` `{kind: at_risk|late, itemId}`; `Money`, `CivilDate` (`YYYY-MM-DD`).
    - `TrackingOrder`: the client view (number, store name, steps from the workflow with client labels and which is current, forecast date, items in plain words, payment summary). It is **not served** until M7, but its schema exists now so 4.10's visibility test checks it against the contract.
  - **Problem codes (new):** the table in `m4-code-design.md` 5.3: `invalid_transition`, `guard_failed` (with `guard`: `deposit_unpaid`, `items_not_ready`, `balance_unpaid`), `order_not_editable`, `order_closed`, `approval_required`, `payment_exceeds_due`, `invalid_item_move`, `no_backup_supplier`, `item_unavailable`, `phone_taken`, `workflow_not_found`, `idempotency_key_required`, `idempotency_key_reused`. Reused: `version_conflict`, `forbidden`, `not_found`, `invalid`.
- **Tests:** `make api-types`; web typecheck; the permission tests; the repository's OpenAPI lint.
- **Reviewers:** `tenancy-security-reviewer` (owner-only fields, `TrackingOrder` contents), `go-reviewer`.
- **Depends on:** this plan's PR.

### 4.2 `domain/workflow`

- **Goal:** the fixed phases, the status workflow value, its validation and the move rules, with every preset tested.
- **Ticks:** "`domain/workflow`: phases, status workflow, `validate.go`, four launch presets, each with its own test"; with 4.6, "Every preset workflow has tests for each allowed and forbidden move and each phase guard".
- **Model:** sonnet. Architecture 4.2 and ADR 0004 hold every rule.
- **Skill:** `/new-bounded-context`, domain part.
- **Files:** `backend/internal/domain/workflow/{doc,phase,status_workflow,workflow_status,validate,moves,errors}.go` + tests; `testdata/presets.go` (the four presets exactly as `0003_workflows` seeds them).
- **Work:**
  - `Phase` with its order and `cancelled`; `StatusKey`; `WorkflowStatus{key, label, clientLabel, phase, position}`.
  - `validate.go`: keys and positions unique; phases never go back along the list; `draft`, `awaiting_deposit`, `ready`, `handed_over` present; `cancelled` never a row; labels 1–60 characters.
  - `moves.go`: `CanMove(from, to)` — any later status; back only within the same phase; `cancelled` from anything before `handed_over`; `PhasesEntered(from, to)`, which the order's guards walk. `HasPhase(assembling)` for the deadline's 0 assembly days.
- **Tests:** table tests per preset over every pair of statuses (allowed or forbidden); validation failures one by one; fuzz the validator with shuffled lists.
- **Reviewers:** `go-reviewer`.
- **Depends on:** 4.1.

### 4.3 `domain/schedule`

- **Goal:** the deadline as pure functions: civil dates, per-item ready dates, the delivery date, the arrival limit and the alerts.
- **Ticks:** "`domain/schedule`: delivery date, arrival limit, `at_risk` and `late` alerts (pure functions, fuzz or table tests)".
- **Model:** sonnet. Architecture 4.2 and ADR 0030 hold the rules.
- **Skill:** none (the package exists with `Days`).
- **Files:** `backend/internal/domain/schedule/{date,readiness,deadline,alerts}.go` + tests; `domain/shared` gains `CivilDate` if other contexts need it (orders do).
- **Work:**
  - `CivilDate` (year, month, day; `AddDays`, `Before`, `After`, parse and format `YYYY-MM-DD`), and `Today(now time.Time, loc *time.Location)`.
  - `ReadyDate(item ItemTiming, release CivilDate)`: `to_order` and, until M6, `stock` = release + lead days, replaced by a promised date when one is given (M5 fills it); `in_house` = release + production days (+ capacity wait, 0 until M6); option extra days added.
  - `Delivery(readies, assembly, margin)`, built on `Days`; `ArrivalLimit(promise, assembly)`.
  - `Alerts(items, promise, assembly, today)`: `at_risk` when expected > arrival limit; `late` when expected < today and not arrived. Never statuses.
  - A workflow without `assembling` uses 0 assembly days (the caller passes it; documented here).
- **Tests:** the CB 500X case with dates (deposit day 0 → delivery day 20, arrival limit day 17); a promised date later than the limit gives `at_risk`; today after the expected date gives `late`; month and year boundaries; fuzz `CivilDate` parsing and `AddDays` round trips.
- **Reviewers:** `go-reviewer`.
- **Depends on:** 4.1.

### 4.4 `domain/customer`

- **Goal:** the store's customer and its addresses.
- **Ticks:** half of "`domain/customer`; customers migration" (with 4.7).
- **Model:** sonnet.
- **Skill:** `/new-bounded-context`, domain part.
- **Files:** `backend/internal/domain/customer/{doc,customer,address,errors}.go` + tests; `CustomerID`, `AddressID` in `domain/shared/id.go`.
- **Work:** name 1–120 with no control characters; WhatsApp phone through `shared.ValidWhatsAppPhone`; optional email (`identity.Email`'s rules), notes up to 2000; up to 10 addresses (CEP as 8 digits, state as a UF code); optional `clientAccountID` (set only in M7); `version`.
- **Tests:** table tests per rule; fuzz the name and CEP.
- **Reviewers:** `go-reviewer`.
- **Depends on:** 4.1.

### 4.5 `domain/order` I: aggregate, lines, snapshots, item lifecycles, pricing

- **Goal:** the order aggregate as far as its contents go: lines (kit or loose item), item snapshots, the item state machines and the money.
- **Ticks:** part of "`domain/order`: …" (with 4.6).
- **Model:** opus. The aggregate's shape, the kit-line model and what each snapshot carries decide every later step and the client tier.
- **Skill:** `/new-bounded-context`, domain part.
- **Files:** `backend/internal/domain/order/{doc,order,line,item,item_status,customization,snapshot,pricing,errors}.go` + tests; `OrderID`, `OrderItemID`, `LineID`, `PaymentID` in `domain/shared/id.go`; `internal/testkit` order builder.
- **Work:**
  - `New(spec NewSpec)`: tenant, customer, workflow (the version), lines, assembly and margin (store defaults or kit or order overrides), deposit percent, number, now. Starts at the workflow's first `draft` status.
  - `Line`: a kit line (kit id, kit name, swaps, kit price from `PriceWithSwaps` computed by the caller and passed in as a value) holding its items, or one loose item.
  - `Item`: the client-visible snapshot (name, unit price, quantity, sourcing mode, options with extra price and days, ready days) and the internal one (`Sourcing{supplier, backup, cost}`), kept apart so repositories write them to two tables.
  - `item_status.go`: the three lifecycles and the `ok`/`defective` check (defective goes back to `to_buy` or `in_production`); in M4 `stock` items use the `to_order` path (ADR 0030).
  - `Total()`, `DepositDue()` (half up to the cent), `Margin()` when every cost is known.
  - `Change(spec, by, now)`: lines, handover, frete, desconto, sinal percent and days. Before any payment and in `draft`/`awaiting_deposit` it is free; afterwards it needs `CustomerApproved` and is refused once `assembling` (or `ready` without assembling) is entered (`ErrNotEditable`); a later forecast moves the promise; `RefundDue()` when the total falls below what was paid. Which role may do which is checked in the app layer.
  - `Total() = lines + frete − desconto`; the discount can't take the total below the frete.
- **Tests:** table tests for each lifecycle (every allowed and forbidden item move); totals with a kit swap in both modes and options; deposit rounding; editing refused after a payment; the internal snapshot never in the client snapshot type.
- **Reviewers:** `go-reviewer`.
- **Depends on:** 4.2, 4.3.

### 4.6 `domain/order` II: payments, `Advance`, dates, cancel, tracking token, events

- **Goal:** the phase guards and everything that moves an order.
- **Ticks:** "`domain/order`: aggregate, items, item status per sourcing mode, customization snapshot, payments (deposit and balance), `Advance` with phase guards, tracking token, events"; with 4.2, "Every preset workflow has tests for each allowed and forbidden move and each phase guard".
- **Model:** opus. The guards are the product's core rules.
- **Skill:** none (continues the package).
- **Files:** `backend/internal/domain/order/{payment,transitions,dates,cancel,tracking_token,events}.go` + tests.
- **Work:**
  - `RecordPayment`, `VoidPayment` (by the owner; the policy is checked in the app layer, the domain takes a reason); `DepositPaid()`, `BalanceDue()`; refuse over-payment.
  - `ConfirmDeposit(now)` and `Release(by, now)`: both fix the release date, recompute each item's expected date and set the promise (`schedule`), unless the owner already changed it.
  - `Advance(to, by, now, handover)`: `workflow.CanMove`, then the guard of each phase entered: leave `awaiting_deposit` → deposit paid or released; enter `assembling` → every item received and `ok`, or ready; enter `ready` without `assembling` → the same; enter `handed_over` → balance 0, with `delivery` or `pickup`. `ErrInvalidTransition`, `ErrGuardFailed{Guard}`. Nothing to source passes `sourcing` at once.
  - `MoveItem(item, to, check, now)` recomputes that item's expected date.
  - `UseBackupSupplier(item, now)`: only a bought item still `to_buy` with a backup in its snapshot; swaps supplier, lead days and cost, and recomputes.
  - `ChangeDeliveryDate(date, by, now)` records old and new; `Cancel(reason, by, now)` before `handed_over`.
  - `tracking_token.go`: the token's hash type (`shared.Secret`); `RotateTracking(hash)`.
  - `events.go`: `order.created`, `order.advanced`, `order.deposit_confirmed`, `order.released`, `order.payment_recorded`, `order.date_changed`, `order.cancelled`, `order.item_moved` — ids and phases only, no customer name or phone.
  - Every change appends a history entry (who, from, to, status key and phase, date changes).
- **Tests:** for each preset, every allowed and forbidden move with each guard held and broken; release by a non-owner is refused in 4.10 (the domain records `by`); CB 500X end to end in dates; a promise changed by the owner survives a deposit; cancel after `handed_over` refused; events carry no personal data.
- **Reviewers:** `go-reviewer`.
- **Depends on:** 4.5.

### 4.7 Migrations

- **Goal:** the M4 tables with three RLS tiers: tenant, client read, and staff-only.
- **Ticks:** half of "`domain/customer`; customers migration" (with 4.4); "Orders migration, with RLS and the client read policy (a client account reads only its own linked orders)".
- **Model:** opus. Schema and RLS.
- **Skill:** `/new-migration`.
- **Files:** the migrations folder (owned), the next free numbers on `main` (expected `0012`–`0015`); `backend/test/integration/rls/`; `backend/test/integration/postgres/` (migration tests).
- **Work:**
  - `0012_customers`: `customers` (name, `whatsapp_phone` unique `(tenant_id, whatsapp_phone)`, email `citext`, notes, `client_account_id` null, version), `customer_addresses`. `tenant_isolation`; audit triggers.
  - `0013_orders`:
    - `order_counters (tenant_id PK, last_number)`;
    - `orders`: `number` unique per tenant, customer, `workflow_id`/`workflow_version` (composite FK to `status_workflows`), `status_key` (composite FK to `workflow_statuses`), `delivery_date date`, `release_date`, `released_by`, `assembly_days`, `safety_margin_days`, `deposit_percent`, total and currency, `handover` (check), `delivery_address_id`, `tracking_token_hash bytea unique`, `client_account_id`, `cancel_reason`, version;
    - `order_lines` (kit or item, kit snapshot jsonb, price), `order_items` (line, snapshot jsonb, quantity, unit price, sourcing mode, ready days, status, check, expected date, arrived at);
    - `order_item_sourcing` (**staff-only**): supplier id and name, backup supplier id and lead days, cost cents;
    - `order_status_history`, `payments` (kind, amount, method, recorded by, voided at/by, reason), `order_messages` (to, text, url, kind).
    - Policies: `tenant_isolation` on all but `order_item_sourcing` and `order_messages` (`staff_only`: costs, and texts that carry a live tracking link); a second, `SELECT`-only **client policy** on `orders`, `order_lines`, `order_items` and `payments` (`current_tenant_id() IS NULL AND client_account_id = current_client_account_id()`, through the order for child tables); none on history, messages, counters or sourcing.
    - Audit triggers on `orders` (without the token hash), `order_lines`, `order_items` and `payments`; none on `order_item_sourcing` or `order_messages`.
    - The full DDL is in `m4-code-design.md` section 2.
  - `0014_idempotency`: `idempotency_keys` (tenant, key, request hash, status, body jsonb, expires_at; PK `(tenant_id, key)`), `tenant_isolation`.
  - `0015_settings_time_zone`: `tenant_settings.time_zone text NOT NULL DEFAULT 'America/Sao_Paulo'` with a check on the four zones.
- **Tests:** every new table in `tenantTables` (five cases); `order_item_sourcing` and `order_messages` in `staffOnlyTables` (sixth case); a new `clientReadTables` list: a client scope reads its own linked rows across two tenants, zero of another client's, and every write is refused; history, messages and sourcing return zero rows to a client; goose up → down → up; contiguous versions.
- **Reviewers:** `tenancy-security-reviewer`, `go-reviewer`.
- **Depends on:** 4.4, 4.6.

### 4.8 `app/workflow`, workflow repository, workflow choice, time zone

- **Goal:** presets readable and cached, the owner's choice of workflow, the time zone setting, and the `WorkflowSource` port orders use.
- **Ticks:** "Migration `0003_workflows` with the preset seed" (shipped in M2 by ADR 0006; this step adds the test that loads every seeded workflow through `validate.go`); part of "`app/workflow`, `app/customer`, `app/order` …".
- **Model:** sonnet.
- **Skill:** `/new-bounded-context`, app part.
- **Files:** `backend/internal/app/workflow/`; `backend/internal/adapter/postgres/workflow_repo.go`, `models/workflow.go`; `app/tenancy` (`ChooseStatusWorkflow`, `TimeZone` in `Editable` and its validation in `domain/tenancy/settings.go`); `adapter/inproc/workflow_source.go`; `test/contract/workflowports/`.
- **Work:** `Presets(ctx)` and `Version(ctx, id, v)` cached under `g:workflow:…` (10 min); the repository replaces tenancy's `WorkflowPresets` port; `ChooseStatusWorkflow` requires `settings:write`, checks the preset exists, publishes `tenancy.workflow_chosen`; `WorkflowSource.Current` reads settings then the version.
- **Tests:** use cases (denied role, happy path, unknown preset, cache hit and fallback); contract suite against fake and Postgres; the seed-through-validate integration test; time zone validation.
- **Reviewers:** `go-reviewer`; `tenancy-security-reviewer` (cache keys, settings write).
- **Depends on:** 4.2, 4.7.

### 4.9 `app/customer` and its repository

- **Goal:** customer use cases and storage.
- **Ticks:** part of "`app/workflow`, `app/customer`, …".
- **Model:** sonnet.
- **Skill:** `/new-bounded-context`, app part.
- **Files:** `backend/internal/app/customer/`; `adapter/postgres/customer_repo.go`, `models/customer.go`; `test/contract/customerports/`.
- **Work:** `List`, `Get` (`order:read`); `Create` (`order:write`, used by Novo pedido) and `Update` (`customer:write`); `phone_taken` on a duplicate; `FindByPhone` for the order flow (and M7's returning client); no cache (customer data is never cached).
- **Tests:** use cases; contract suite against fake and Postgres; keyset pages by `(lower(name), id)`; the phone conflict mapping.
- **Reviewers:** `go-reviewer`, `tenancy-security-reviewer` (repository).
- **Depends on:** 4.4, 4.7.

### 4.10 `app/order` writes

- **Goal:** every order write use case, with its ports, the owner-only views and the client view.
- **Ticks:** part of "`app/workflow`, `app/customer`, `app/order` (create, confirm deposit, release without deposit, advance, record balance, hand over, change delivery date → `wa.me` link, cancel), `app/schedule`"; "Outbox … in the same transaction as order writes" (the events; the repository half in 4.12); "'New orders take the store's current workflow and keep it' use case test; client-visibility test on tracking responses".
- **Model:** opus. Cross-context ports (catalog, customer, workflow, settings), the owner-only and client views, and the token flow.
- **Skill:** `/new-bounded-context`, app part.
- **Files:** `backend/internal/app/order/{doc,ports,inputs,create,quote,lines,payments,moves,dates,cancel,links,views,tracking_view}.go`, `fake/`; `adapter/inproc/{order_catalog,order_customers,order_settings}.go`; `test/contract/orderports/`.
- **Work:**
  - **Ports:** `Repository` (insert with the next number, update with the version, by id); `Catalog` (resolve lines to snapshots and prices: kit price with swaps, options, ready days, and the staff-only sourcing; refuses archived items), implemented in `inproc` over `app/catalog`; `Customers` over `app/customer`; `WorkflowSource`; `StoreSettings` (defaults, time zone, slug, WhatsApp number); `TrackingTokens` and `Links` (4.13); `Idempotency` (4.13); `appshared` ports.
  - **Use cases:** `Quote` (no write), `Create` (customer by id or new, current workflow, token, `order.created`), `Change` (before a payment: `order:write`, and `order:manage` for discount, deposit percent and days; after one: `order:manage`, `customerApproved`, before Montagem, returns a `wa.me` summary), `RecordPayment` (a deposit that reaches the due confirms it), `Release` (`order:override`), `Advance`, `MoveItem`, `UseBackupSupplier` (`order:manage`), `VoidPayment` (`order:override`), `ChangeDeliveryDate` (`order:manage`) → `wa.me` link + `order_messages`, `SendTrackingLink` (rotate) → link + message, `Cancel` (`order:manage`).
  - **Views:** `OrderView` built with `identity.SeesCosts`; `TrackingView` with steps from the order's workflow and client labels, no supplier, cost, margin, alert or internal status.
- **Tests:** each use case: denied role, happy path, events, version conflict; release by admin refused; **a new order takes the current workflow, and keeps its version after the owner picks another**; owner vs admin vs viewer views; a JSON walk of `TrackingView` refusing `cost`, `margin`, `supplier*`, `leadDays`, `atRisk`, `alerts`; the supplier never in the client snapshot.
- **Reviewers:** `tenancy-security-reviewer`, `go-reviewer`.
- **Depends on:** 4.6, 4.8, 4.9.

### 4.11 `app/order` queries and `app/schedule`

- **Goal:** what the screens read: lists, the detail with alerts, history, Hoje and Financeiro; and `schedule.Recalculate` for one order.
- **Ticks:** completes "`app/workflow`, `app/customer`, `app/order` …, `app/schedule`".
- **Model:** sonnet. 4.10 set the views; this reads them.
- **Skill:** none.
- **Files:** `backend/internal/app/order/{queries,today,finance}.go`; `backend/internal/app/schedule/`.
- **Work:** `List` (filters of 4.1, `alert` filter computed on active orders); `Get`; `History`; `Today` (counts, orders by promise date with items ready/total, "Precisa de você": `late`, `at_risk`, awaiting the deposit past N days, ready but not handed over, balance due at handover); `Finance(from, to)` (`finance:read`); `app/schedule.Recalculate(order)` and a `CheckAll` stub signature the M6 worker will fill.
- **Tests:** fixed clock and time zone (a Manaus store's "today" differs from São Paulo's at 23:30 UTC); each Hoje rule; finance totals with a voided payment; viewer sees no margins.
- **Reviewers:** `go-reviewer`.
- **Depends on:** 4.10.

### 4.12 `adapter/postgres` order repository

- **Goal:** GORM repository for orders, passing its contract against Postgres, with the outbox event in the same transaction.
- **Ticks:** "Outbox (`outbox_repo`) in the same transaction as order writes" (with 4.10).
- **Model:** sonnet. The GORM rules hold the design.
- **Skill:** none.
- **Files:** `backend/internal/adapter/postgres/order_repo.go`, `models/order.go`; `test/integration/postgres/`.
- **Work:** insert takes the number from `order_counters` (`UPDATE … RETURNING`, row created on first use); lines, items, sourcing (a separate write the staff scope can see), payments, history and messages; optimistic locking on `orders.version`; list with `Raw` where the alert and count queries need it; maps on `Select`, never `Select("*")`.
- **Tests:** contract suite on Postgres as `encomendo_app`; two concurrent creates get different numbers; a failed write leaves no outbox row; the client scope reads the order without its sourcing rows; constraint mapping.
- **Reviewers:** `tenancy-security-reviewer`, `go-reviewer`.
- **Depends on:** 4.7, 4.10.

### 4.13 Tracking tokens, `wa.me` links, `Idempotency-Key`

- **Goal:** the three adapters behind 4.10's ports.
- **Ticks:** "Tracking tokens (`adapter/auth/tracking.go`); `adapter/notify/whatsapp_link.go`"; "Idempotency-Key storage for POST endpoints".
- **Model:** opus. Token handling and replay safety.
- **Skill:** none.
- **Files:** `backend/internal/adapter/auth/tracking.go`; `adapter/notify/whatsapp_link.go`; `adapter/postgres/idempotency_repo.go`; `adapter/http/middleware/idempotency.go` (or a handler helper); tests.
- **Work:** 32 random bytes from `SecretGenerator`, SHA-256 at rest, constant-time lookups, no "exists elsewhere" signal; `wa.me/{digits}?text=` with URL-encoded pt-BR text from 4.15, refusing a phone that is not 55 + DDD + number; idempotency: key 1–128 printable characters, request hash, stored response replayed, `idempotency_key_reused` on another body, rows past 24 hours ignored (a cleanup job waits for the M6 worker).
- **Tests:** token guessing (random tokens never match), hash only in storage; links with accents, line breaks and `&`; concurrent retries with one key make one order; replay returns the identical body.
- **Reviewers:** `tenancy-security-reviewer`, `go-reviewer`.
- **Depends on:** 4.7, 4.10.

### 4.14 HTTP handlers, routes, wiring

- **Goal:** the M4 endpoints served, wired in `main.go`, matching `openapi.yaml`.
- **Ticks:** "HTTP handlers and OpenAPI: orders, customers, status workflows, payments, Hoje queries".
- **Model:** sonnet. 4.1 and `/new-endpoint` hold the design.
- **Skill:** `/new-endpoint`, per endpoint.
- **Files:** `adapter/http/handler/{workflow,customer,order,today,finance}.go`, `dto/`; `router.go` and `main.go` (owned); `respond/problem.go` (the new codes); `cmd/api` imports `time/tzdata`; `test/e2e/` (a short order flow).
- **Tests:** handler tests with fake use cases; responses validated against the contract; non-owner responses carry no `cost` or `margin`; `Idempotency-Key` missing → 400 `idempotency_key_required`; e2e: create an order, record the deposit, move items, advance to handover, change the date and get a `wa.me` link with nothing sent.
- **Reviewers:** `go-reviewer`, `tenancy-security-reviewer`.
- **Depends on:** 4.8 to 4.13.

### 4.15 Wording (done in the plan PR)

- **Goal:** the pt-BR text of every M4 screen and message, drafted in `docs/product/design-document.md` for the user to review, as M3 did for Catálogo.
- **Ticks:** none alone.
- **Model:** sonnet.
- **Skill:** none.
- **Files:** `docs/product/design-document.md` (new sections Configurações, Clientes, Pedidos, Pedido, Hoje, Financeiro, and the WhatsApp texts for a date change and the tracking link).
- **Work:** use the words already fixed ("Precisa chegar até", "Pode atrasar", "Atrasada", "Margem de segurança", "Marcar como enviada ao fornecedor", "Tudo certo / Com defeito", "Sinal", "Saldo", "Entrega / Retirada"); every guard and problem code gets a message; niche words as `{Montagem}`; mark the section "Drafted by Claude … for the user to review".
- **Tests:** none (docs).
- **Reviewers:** none; the user reviews.
- **Depends on:** 4.1.

### 4.16 Web `packages/api`

- **Goal:** typed clients for the M4 endpoints.
- **Ticks:** none alone (part of the web boxes).
- **Model:** sonnet.
- **Files:** `web/packages/api/src/{workflows,settings,customers,orders}.ts` + tests; MSW handlers and fixtures in the admin's `testing/` with **two** workflows (`web/CLAUDE.md`).
- **Work:** an `Idempotency-Key` per user action (kept across a retry of the same action); `wa.me` URLs passed through, never built.
- **Tests:** Vitest with MSW for each call, problem codes mapped to `ApiError`.
- **Reviewers:** `web-reviewer`.
- **Depends on:** 4.1.

### 4.17 Web Configurações

- **Goal:** the settings screen: the status workflow with a preview of its steps, the store defaults and the time zone.
- **Ticks:** "Workflow choice in Settings, with a preview of its steps".
- **Model:** sonnet.
- **Files:** `web/apps/admin/src/features/settings/`; router and menu (owner: `settings:write`).
- **Work:** presets as choices, each previewing staff and client labels in order with the phase colour; "applies to new orders" stated; the defaults form with `version`; desktop and phone layouts.
- **Tests:** Vitest (two workflows; a viewer can't open it; version conflict); phone layout test.
- **Reviewers:** `web-reviewer`.
- **Depends on:** 4.16.

### 4.18 Web Clientes

- **Goal:** the customer list and form.
- **Ticks:** part of "Hoje …, Pedidos …, Clientes, Financeiro".
- **Model:** sonnet.
- **Files:** `web/apps/admin/src/features/clientes/`.
- **Work:** list with search by name or phone, cards on the phone; form with addresses; the customer's orders (from `GET /orders?customerId=`); edits only with `customer:write`.
- **Tests:** Vitest (phone taken, read-only for staff and viewer); phone layout.
- **Reviewers:** `web-reviewer`.
- **Depends on:** 4.16.

### 4.19 Web Pedidos: list and Novo pedido

- **Goal:** orders by status, and creating an order.
- **Ticks:** part of "Pedidos (list by status, …)".
- **Model:** sonnet.
- **Files:** `web/apps/admin/src/features/pedidos/` (list, `NewOrder`).
- **Work:** tabs or a filter per status of the store's workflow (labels from the API, colour from the phase), alert badges on top; Novo pedido: pick or create the customer, add kits (with swaps and options) and items, delivery or pickup, per-order days; totals, deposit and date from `POST /orders/quote` only; after saving, show the tracking link with "Enviar pelo WhatsApp".
- **Tests:** Vitest with two workflows; quote errors; `item_unavailable`; idempotent retry; phone layout.
- **Reviewers:** `web-reviewer`.
- **Depends on:** 4.18 (the customer picker).

### 4.20 Web order detail

- **Goal:** Pedido P-0231 as drawn: the item × deadline timeline and every action.
- **Ticks:** completes "Pedidos (list by status, detail with item × deadline timeline)".
- **Model:** sonnet.
- **Files:** `web/apps/admin/src/features/pedidos/` (detail, components).
- **Work:** timeline with today, "Precisa chegar até", {Montagem} and the delivery date; per-item status with manual moves and "Tudo certo / Com defeito"; delay banner; payments (record, void for the owner), release (owner); advance to the next statuses the API allows, with guard messages; change date → open the `wa.me` link; "Enviar link de acompanhamento"; cancel with a reason; history; cost and margin only when sent.
- **Tests:** Vitest per action and guard; owner vs admin; phone layout.
- **Reviewers:** `web-reviewer`.
- **Depends on:** 4.19.

### 4.21 Web Hoje

- **Goal:** the Hoje screen with real data.
- **Ticks:** part of "Hoje (summary cards, orders by delivery date, "Precisa de você")".
- **Model:** sonnet.
- **Files:** `web/apps/admin/src/features/hoje/`.
- **Work:** summary cards, orders by promise date with items ready/total, "Precisa de você" linking to the order; today's purchases are M5 and not shown.
- **Tests:** Vitest; phone layout.
- **Reviewers:** `web-reviewer`.
- **Depends on:** 4.16 (4.20 for the links' target).

### 4.22 Web Financeiro

- **Goal:** the owner's money view.
- **Ticks:** completes "Hoje …, Pedidos …, Clientes, Financeiro".
- **Model:** sonnet.
- **Files:** `web/apps/admin/src/features/financeiro/`; menu entry for `finance:read`.
- **Work:** period picker; received by kind and method; balances to receive; margins per handed-over order ("—" without a cost); no donut charts.
- **Tests:** Vitest; non-owners can't open it; phone layout.
- **Reviewers:** `web-reviewer`.
- **Depends on:** 4.16.

### 4.23 Real stack, phone and shell; close the milestone

- **Goal:** prove the milestone end to end and close the docs.
- **Ticks:** "Hoje, Pedidos, Clientes, Financeiro and the workflow setting work at phone size"; "Changing a delivery date returns a `wa.me` link and nothing is sent automatically"; any box left partial above.
- **Model:** sonnet.
- **Files:** `web/e2e/{pedidos,phone-pedidos}.spec.ts`; `docs/build-plan.md`; `docs/architecture.md` (4.4 and 7 as built: idempotency in Postgres, the new tables); `CLAUDE.md` glossary (order number, kit line, promise, void, `customer:write`); this plan's Result section.
- **Work:** Playwright against `make stack-up`: owner chooses a workflow → creates a customer and an order from a kit → records the deposit → moves items → advances to handover; an admin sees no cost; change the date and open the `wa.me` link. The same at 390 × 844. Walk the main screens in the Android shell on a device or emulator (closes 3.5's walkthrough too, if KVM is back).
- **Tests:** the flows above; `make lint test`, `make test-integration`; coverage gate.
- **Reviewers:** `web-reviewer`, `tenancy-security-reviewer` (final pass over the client tier).
- **Depends on:** all.

## Build-plan boxes → steps

| Box | Steps |
| --- | --- |
| `domain/workflow` … each with its own test | 4.2 |
| Migration `0003_workflows` with the preset seed | 4.8 (seed shipped in M2, ADR 0006; the validation test lands here) |
| `domain/customer`; customers migration | 4.4, 4.7 |
| `domain/order` … | 4.5, 4.6 |
| `domain/schedule` … | 4.3 |
| Orders migration, with RLS and the client read policy | 4.7 |
| `app/workflow`, `app/customer`, `app/order` …, `app/schedule` | 4.8, 4.9, 4.10, 4.11 |
| Outbox in the same transaction as order writes | 4.10, 4.12 |
| Tracking tokens; `whatsapp_link.go` | 4.13 |
| Idempotency-Key storage | 4.13 |
| HTTP handlers and OpenAPI | 4.1, 4.14 |
| New orders keep their workflow; client visibility on tracking | 4.10 |
| Web: Hoje, Pedidos, Clientes, Financeiro | 4.18–4.22 |
| Web: workflow choice in Settings | 4.17 |
| Phone: every M4 screen at phone size | 4.17–4.22, ticked in 4.23 |
| Done when: every preset's moves and guards tested | 4.2, 4.6 |
| Done when: date change returns `wa.me`, nothing sent | 4.10, 4.14, 4.23 |
