# Milestone 5 — Purchasing and receiving

Written 2026-11-28 on Opus with `/plan-milestone 5`, against `main` at `0fda109`. Build each step with `/build-step 5.<n>`.

Goal (build plan): the business buys only what was sold, tells the supplier without leaking the client, and checks what arrives.

## Blocked

Nothing. Milestone 5 depends on none of the questions in `docs/architecture.md` section 12. Payments by hand (ADR 0023) don't touch purchasing. The one conflict, offline receiving in the "Done when" against ADR 0029, was answered by the user (Q1).

## The user's answers (2026-11-28)

| # | Question | Answer |
| --- | --- | --- |
| Q1 | The "Done when" says items are received "with no signal → synced later", but ADR 0029 dropped offline. Which? | Online only. Reword the box; a retried request uses the same `Idempotency-Key`, so nothing is lost or doubled |
| Q2 | Do the M4 manual moves of bought items stay on the order screen? | No, Compras drives them. Sending marks items "Enviada ao fornecedor" and Receber itens marks them "Recebida". The order screen keeps only "Marcar como instalada" |
| Q3 | What does the supplier message list? | The same item from several orders summed into one line, under a store purchase number "pedido de compra C-0012"; never the client's data |
| Q4 | What does "Com defeito" do? | The item goes back to "A comprar" and a replacement line for the same supplier appears in Compras. Returns are handled outside Encomendo |
| Q5 | Which photos and notes are required? | Photos are all optional; "Com defeito" needs a note |
| Q6 | Partial quantities? | All or nothing per line |
| Q7 | Extras beyond the boxes? | All four: Hoje entries, "Falar com o fornecedor" on the delay banner, "Avisar o fornecedor" for sent lines of cancelled or changed orders, and the price actually paid (owner only; Financeiro margins use it) |
| Q8 | How does the session run? | Like M4: one session on Opus, Sonnet subagents in worktrees, grouped PRs. Claude merges its own PRs once CI is green and the reviewers find nothing blocking. The wording is drafted in this plan's PR. CI workflow edits wait for the user. The shell walkthrough on a phone stays pending |

ADR 0031 records these and the decisions below. The code-level design is written in step 5.1 (`docs/plans/m5-code-design.md`) before any code, in the same shape as M4's. A later step that finds it wrong stops and says so rather than redesigning.

## How the session runs

The whole milestone runs in **one session, without stopping to ask**. Every product question is answered above or settled in ADR 0031. If something comes up that neither covers, the session takes the option that keeps the dependency rule, tenant isolation and testability, writes it down as an amendment to ADR 0031, and carries on. It stops only for a decision that can't be undone and could reasonably go either way.

- **Models.** The main session runs on Opus (`claude --model opus`) and builds the opus steps itself. It gives each sonnet step to a subagent with `model: sonnet`, `isolation: worktree`, that step's section of this plan and of the code design, and the `/build-step` instructions. The main session reviews what comes back before it goes into the PR, and pushes the agent branches.
- **PRs.** Eight, in order. Each PR is green on its own: `make lint test`, plus `make test-integration` when adapters change. Each runs every reviewer its steps list and ticks its boxes in `docs/build-plan.md`. Claude merges it once CI is green and the reviewers find nothing blocking (Q8). The next PR branches from the updated `main`.

| PR | Steps | Branch | Runs on |
| --- | --- | --- | --- |
| A | 5.1 code design and contract | `feat/m5-contract` | Opus |
| B | 5.2–5.4 domain | `feat/m5-domain` | 5.2, 5.3 Sonnet subagents in parallel; 5.4 Opus |
| C | 5.5 migration and RLS | `feat/m5-migrations` | Opus |
| D | 5.6–5.8 app layer | `feat/m5-app` | 5.6 Opus; 5.7, 5.8 Sonnet |
| E | 5.9–5.10 adapters, HTTP, wiring | `feat/m5-adapters-http` | 5.9, 5.10 Sonnet, with Opus review of the photo keys and wiring |
| F | 5.12–5.13 web API, Compras, Fornecedores | `feat/m5-web-compras` | Sonnet subagent, started once A is merged, alongside B–E |
| G | 5.14–5.15 web Receber itens, order screen, Hoje | `feat/m5-web-receber` | Sonnet subagent, after F |
| H | 5.16 real stack, phone, close | `feat/m5-close` | Opus, with a Sonnet subagent for the Playwright specs |

F and G build against MSW from the contract, so they don't wait for the backend; H checks them against the real stack.

- **CI workflow edits** (`.github/workflows/`) still need the user's yes. M5 should need none. If a step finds it does, it writes the change in its PR description and carries on.
- **Android shell walkthrough.** KVM is still blocked (KB5127648). H proves phone size at 390 × 844 in Playwright, with a file input standing in for the camera, and records the device walkthrough as pending with M3.5's and M4's. The phone box stays partial for the camera-through-the-shell half only.

## Decisions in this plan

ADR 0031 records them. The short form:

- **Online only.** No offline queue; retries are safe through `Idempotency-Key`.
- **Compras is the only path** for bought items from `to_buy` to `received`. The order's move endpoint refuses those moves with `409 item_moved_by_purchasing` and keeps `received → installed`. In-house items keep manual moves until M6.
- **An in-transaction event dispatcher** (`adapter/inproc`) wraps the outbox publisher and runs purchasing's handlers in the order's transaction. `app/order` never names purchasing. Purchasing reaches orders through an `OrderItems` port.
- **Lines appear** on `deposit_confirmed` and `released`, or when the order leaves `draft` with "Liberar compras antes do sinal" on. They are idempotent per order item (one open line per item), and follow changes, cancels and backup switches. Sent lines that are no longer needed are **flagged** ("Avisar o fornecedor").
- **Supplier orders** are numbered per store, `C-0001`. Sending creates one from the picked pending lines and answers the `wa.me` link. The message sums the same item and choices, and never names the client. "Desfazer envio" works while nothing is received.
- **Promised date** per supplier order, with a per-line override; it redates the items (forecast), never the promise.
- **Price paid** per line, owner only; Financeiro margins prefer it.
- **Receipts:** all or nothing per line; photos optional; "Com defeito" needs a note and makes a replacement line.
- **No new permission:** `purchase:write` for Compras, `receiving:write` for Receber itens.
- **Staff-only RLS** on every purchasing table.
- **Time zone changes** call `schedule.Recalculate`, which now redates.

## Ownership

This session is the only milestone 5 session. It owns `backend/cmd/api/main.go`, `backend/internal/adapter/http/router.go`, `docs/api/openapi.yaml` and the migrations folder. If parallel sessions start:

- 5.1 owns `openapi.yaml` (later changes go through 5.10);
- 5.5 owns the migrations;
- 5.10 owns `router.go` and `main.go`.

## Steps

| Step | Goal | Model |
| --- | --- | --- |
| 5.1 | Code design and contract: purchasing, receipts, supplier orders, order and Hoje changes, problem codes | opus |
| 5.2 | `domain/purchasing` I: purchase line, supplier order, numbers, supplier message | sonnet |
| 5.3 | `domain/purchasing` II: receipt, checks, replacement lines, notices | sonnet |
| 5.4 | `domain/order` changes: purchasing-only moves, promised-date redate, paid cost, new events | opus |
| 5.5 | Migration `0016_purchasing` with staff-only RLS, and `paid_cost_cents` on sourcing | opus |
| 5.6 | In-transaction event dispatcher, purchasing's event handlers, `OrderItems` port | opus |
| 5.7 | `app/purchasing` commands and queries: send, undo, promised date, in transit, prices, receive, notices | sonnet |
| 5.8 | `app/order` and `app/schedule` changes: refuse bought moves, `Recalculate` redates, Hoje entries, Financeiro paid cost, supplier chat link | sonnet |
| 5.9 | `adapter/postgres` purchasing repository and receipt photo storage, with the contract suite | sonnet |
| 5.10 | `wa.me` supplier links, `inproc` adapters, HTTP handlers, routes, wiring, visibility tests | sonnet |
| 5.11 | pt-BR wording for Compras, Receber itens, Fornecedores, order and Hoje changes | done in the plan PR |
| 5.12 | Web `packages/api`: `purchasing.ts`, `receipts.ts`, order and Hoje changes | sonnet |
| 5.13 | Web Compras: A comprar, Enviadas, pedido de compra, Fornecedores tab, menu | sonnet |
| 5.14 | Web Receber itens: checklist, photos, Tudo certo / Com defeito, workshop sizes | sonnet |
| 5.15 | Web order screen and Hoje changes | sonnet |
| 5.16 | Real stack on desktop and phone; update the M4 specs; close the milestone | sonnet |

**Order.**

- 5.1 first.
- 5.2, 5.3 and 5.4 are pure and run in parallel once 5.1 is merged (5.3 builds on 5.2's types, so the same subagent does both if that is simpler).
- 5.5 needs 5.2–5.4, so the columns follow the domain.
- 5.6 needs 5.4 and 5.5. 5.7 needs 5.6. 5.8 needs 5.4 and 5.6.
- 5.9 needs 5.5 and 5.7. 5.10 needs everything above.
- On the web side, 5.11 is done in the plan PR, and 5.12 starts once 5.1 is merged. 5.13 to 5.15 follow 5.12 against MSW; 5.16 checks them against the real stack.

**Models.** Steps marked opus need `/model opus` or `claude --model opus`: 5.1, 5.4, 5.5 and 5.6.

**Phone size.** Every web step builds its screen at desktop and at 390 × 844 (`web/CLAUDE.md`, Phone layouts). Receber itens is a workshop screen: 18 px text, 56 px targets and a 64 px primary action (ADR 0029). The build plan's phone box ticks in 5.16.

---

### 5.1 Code design and contract

- **Goal:** write `docs/plans/m5-code-design.md` (packages, types and signatures, SQL, endpoints, problem codes, web files, test names, as M4's) and the purchasing part of `docs/api/openapi.yaml`. Ticks: part of "HTTP handlers and OpenAPI" (the contract half; the box ticks in 5.10).
- **Model:** opus. It fixes the cross-context design (dispatcher, `OrderItems` port, event payloads), the schema and the client-visibility edges for every later step.
- **Skill:** none. `/new-endpoint`'s contract section is the checklist for each operation.
- **Files and owner:** `docs/plans/m5-code-design.md`, `docs/api/openapi.yaml` (owned here), `web/packages/api/src/types` regenerated (`make api-types`).
- **Contract, at least:**
  - `GET /admin/purchasing/to-buy`: pending lines grouped by supplier, with the order number, the arrival limit, "Reposição" and, for the owner, the cost;
  - `POST /admin/purchasing/supplier-orders/preview`: the message for picked lines;
  - `POST /admin/purchasing/supplier-orders`: send, with `Idempotency-Key`; answers the supplier order and its `wa.me` link;
  - `GET /admin/purchasing/supplier-orders` and `GET /admin/purchasing/supplier-orders/{id}`;
  - on a supplier order: `PUT …/promised-date` (with per-line dates), `POST …/in-transit`, `POST …/undo`, `PUT …/paid-prices` (owner);
  - `GET /admin/purchasing/to-receive`;
  - `POST /admin/purchasing/receipt-photo-uploads` (signed upload, as catalog photos);
  - `POST /admin/purchasing/receipts`, with `Idempotency-Key`;
  - `GET /admin/purchasing/notices` and `POST …/notices/{lineId}/resolve`, with the notice's `wa.me` link;
  - `OrderItem` gains `purchase` (supplier order number, line status, promised date) and, for `purchase:write`, `supplierChatUrl`;
  - Hoje gains the three purchasing entries and the "Compras a enviar" count, for `purchase:write`;
  - the move endpoint documents `409 item_moved_by_purchasing`.
- **Event shapes:** `order.advanced` gains `fromPhase`; `order.supplier_switched` is new; each handler's input is listed.
- **Tests:** none (docs and contract). `make api-types` and the contract lint in `web.yml` pass.
- **Reviewers:** `tenancy-security-reviewer` on the contract (no client-tier field, costs owner-only, no client data in any purchasing schema), `web-reviewer` on the generated types.
- **Depends on:** nothing.

### 5.2 `domain/purchasing` I: purchase line, supplier order, numbers, supplier message

- **Goal:** `purchase_line.go`, `supplier_order.go`, `number.go` (`C-0001`), `supplier_message.go`. Ticks: part of "`domain/purchasing`".
- **Model:** sonnet. The code design and ADR 0031 hold the design.
- **Skill:** none; `/new-bounded-context` only as the layout reference (the context is new, but its scaffold comes with 5.6–5.9).
- **Files:** `backend/internal/domain/purchasing/`.
- **Tests (domain):**
  - line states `pending → sent → received | defective`, `pending → cancelled`, `sent → pending` (undo), and the flag on sent lines;
  - a supplier order holds lines of one supplier only and refuses an empty pick;
  - the promised date per order and per line, refusing a past day;
  - prices are integer cents with a currency;
  - `Number.String()` gives `C-0001` and `C-12345`.
  - **The message:** it sums equal item and choices, keeps order-of-first-appearance, and carries the store's and the supplier's names. A test builds lines from orders whose customers have names, phones and addresses, and asserts that none of them, nor any `P-` number, appears in the text (`TestSupplierMessage_NeverCarriesClientData`, with a fuzz case over names and phones).
- **Reviewers:** `go-reviewer`.
- **Depends on:** 5.1.

### 5.3 `domain/purchasing` II: receipt, checks, replacements, notices

- **Goal:** `receipt.go`: lines checked `ok` or `defective`; all or nothing; a defect note of 1 to 500 characters; photo limits (4 invoice photos, 2 per line); a defective line yields its replacement `pending` line; notices and their resolution. Ticks: the rest of "`domain/purchasing`".
- **Model:** sonnet.
- **Files:** `backend/internal/domain/purchasing/`.
- **Tests (domain):**
  - a receipt refuses no lines, a line twice, a line already received or cancelled, and a defect without a note;
  - receiving a `pending` line (bought at the counter) works;
  - a defective check makes exactly one replacement line, pointing at the old one;
  - resolving a notice cancels the line, and resolving it twice is refused.
- **Reviewers:** `go-reviewer`.
- **Depends on:** 5.2.

### 5.4 `domain/order` changes

- **Goal:** the order side of ADR 0031:
  - a purchasing-only path for bought items (`MarkSentToSupplier`, `MarkInTransit`, `UndoSent`, `ReceiveBought` with its check), while `MoveItem` refuses those moves for bought items (`ErrItemMovedByPurchasing`) and keeps `received → installed` and every in-house move;
  - `RecordPromisedDate(item, date)`, which fills `Timing.Promised` and redates the forecast (the public redate M4 left out);
  - `RecordPaidCost(item, cents)` on the sourcing snapshot;
  - the events `SupplierSwitched` and `Advanced.FromPhase`.
  Ticks: none alone (it serves "`app/purchasing`").
- **Model:** opus. It changes the aggregate's invariants and the event contract other contexts depend on.
- **Files:** `backend/internal/domain/order/`.
- **Tests (domain):**
  - every bought move through `MoveItem` other than `received → installed` is refused;
  - each purchasing method follows `CanMoveItem`, with a defect landing on `to_buy`;
  - a promised date after the arrival limit makes the item `at_risk` and leaves `Promise()` unchanged;
  - `UseBackupSupplier` publishes `SupplierSwitched`;
  - leaving draft publishes `FromPhase: draft`;
  - the existing `TestAdvance_*` tables still pass with items received through the new path.
- **Reviewers:** `go-reviewer`.
- **Depends on:** 5.1.

### 5.5 Migration `0016_purchasing`

- **Goal:** `purchase_counters`, `supplier_orders`, `purchase_lines`, `receipts`, `receipt_lines` and `receipt_photos`, plus `order_item_sourcing.paid_cost_cents`, all with composite tenant foreign keys and the **staff-only** policy. Ticks: "Purchasing migration, with RLS and tests".
- **Model:** opus. Schema and RLS.
- **Skill:** `/new-migration`.
- **Files and owner:** `backend/internal/adapter/postgres/migrations/0016_purchasing.sql` (owned here), the RLS suite.
- **Must hold:**
  - a partial unique index on `purchase_lines (tenant_id, order_item_id) WHERE status IN ('pending','sent')`;
  - `order_item_id` nullable (restock, M6), with a check that only M6 kinds may leave it null (or no null until M6, as the code design decides);
  - photo object keys checked as `t/{tenant}/receipts/{receipt}/{photo}`;
  - no client policy on any of these tables;
  - audit triggers on supplier orders, lines and receipts, but not on cost columns (`audit_log` is readable by every staff role, while costs are the owner's: split `paid_cost_cents` into its own table or leave it out of the audit, as 0013 did for sourcing).
- **Tests (RLS, integration):**
  - the tenant cases for every new table;
  - the staff-only case: a store page scope reads zero rows;
  - the client case: a client account reads zero rows;
  - the unique index refuses a second open line;
  - a cross-tenant foreign key is refused.
- **Reviewers:** `tenancy-security-reviewer`, `go-reviewer`.
- **Depends on:** 5.2–5.4.

### 5.6 Event dispatcher, purchasing's handlers, `OrderItems` port

- **Goal:**
  - `adapter/inproc/event_dispatcher.go` (wraps `EventPublisher`; handlers by name, synchronous, in the publishing transaction);
  - `app/purchasing` handlers for `deposit_confirmed`, `released`, `advanced` (leaving draft with `purchase_before_deposit`), `changed`, `cancelled` and `supplier_switched`;
  - the `OrderItems` port and its fake.
  Ticks: "`app/purchasing`: `GeneratePurchases` (after deposit unless `purchase_before_deposit`) …" (the generation half).
- **Model:** opus. Cross-context design, and the handlers run without a permission check in the caller's scope.
- **Skill:** `/new-bounded-context` for `app/purchasing`'s ports, fakes and contract-suite layout.
- **Files:** `backend/internal/app/purchasing/`, `backend/internal/adapter/inproc/`, `backend/test/contract/`.
- **Tests:**
  - app with fakes: lines appear after the deposit and not before; with the setting on, they appear on leaving draft and never for a draft; a deposit paid again after a void makes nothing new; a change adds and cancels lines and flags sent ones; a cancel does the same for every item; a backup switch moves a pending line and leaves a sent one;
  - the dispatcher: a failing handler fails the publish, and the transaction rolls back the order write (fake tx), and an unregistered event changes nothing;
  - the handlers reject a scope with no tenant.
- **Reviewers:** `go-reviewer`, `tenancy-security-reviewer` (permission-less internal entry points).
- **Depends on:** 5.4, 5.5.

### 5.7 `app/purchasing` commands and queries

- **Goal:**
  - `SendSupplierOrder` (→ `wa.me` link through a `SupplierLinks` port), `PreviewMessage`, `UndoSend`, `RecordPromisedDate` (→ `OrderItems.RecordPromisedDate`), `MarkInTransit`, `RecordPaidPrices` (owner) and `Receive` (receipt, photos confirmed through the storage port, checks to `OrderItems`);
  - `ResolveNotice`;
  - the queries `ToBuy`, `SupplierOrders`, `SupplierOrder`, `ToReceive` and `Notices`.
  Ticks: the rest of "`app/purchasing`".
- **Model:** sonnet. The code design holds the design.
- **Files:** `backend/internal/app/purchasing/`.
- **Tests (app with fakes, fixed clock and IDs):**
  - every command checks its permission (`purchase:write`, `receiving:write`, the owner for prices);
  - costs and paid prices are absent from every view built for a non-owner;
  - send moves the lines and the items and records the link;
  - undo is refused once anything is received;
  - a promised date redates the items and raises `at_risk` when late;
  - receive with a defect makes the replacement and moves the item back;
  - an `Idempotency-Key` replay of send and receive answers the first result;
  - the message test from 5.2 again, through the use case, with real orders from the fake.
- **Reviewers:** `go-reviewer`.
- **Depends on:** 5.6.

### 5.8 `app/order` and `app/schedule` changes

- **Goal:**
  - `MoveItem` maps `ErrItemMovedByPurchasing`;
  - `schedule.Recalculate` redates and saves, and the time zone change in `app/tenancy` settings calls it for active orders;
  - Hoje gains the purchasing entries through a `PurchasingAlerts` port (only for `purchase:write`);
  - Financeiro margins prefer `paid_cost_cents`;
  - the order view's bought items carry `purchase` and, for `purchase:write`, `supplierChatUrl`;
  - the M4 hand-off comment in `Cancel` is removed (the handler now does it) and its issue closed.
  Ticks: none alone; serves "HTTP handlers and OpenAPI" and the web boxes.
- **Model:** sonnet.
- **Files:** `backend/internal/app/order/`, `backend/internal/app/schedule/`, `backend/internal/app/tenancy/`.
- **Tests (app with fakes):**
  - a staff move of a bought item to `received` is refused and an in-house move still works;
  - a time zone change redates;
  - Hoje shows the three entries to the owner and admin and none to staff;
  - a margin uses the paid price when there is one;
  - `supplierChatUrl` is absent for staff and viewers and never names the client.
- **Reviewers:** `go-reviewer`.
- **Depends on:** 5.4, 5.6.

### 5.9 `adapter/postgres` purchasing repository and receipt photos

- **Goal:** `purchasing_repo.go`:
  - lines, supplier orders with their number counter, receipts and notices;
  - `ToBuy` grouped by supplier with `Raw` where clearer;
  - receipt photos staged and promoted through `adapter/storage` (ADR 0028), under the receipts key prefix;
  - the GORM models in `models/`.
  Ticks: "Postgres repository (grouping by supplier with `Raw` where clearer); receipt photo upload through storage".
- **Model:** sonnet.
- **Files:** `backend/internal/adapter/postgres/`, `backend/internal/adapter/storage/` (key helper only), `backend/test/contract/`.
- **Tests:**
  - the contract suite for every purchasing port against the fake (`make test`) and Postgres and Silo (`make test-integration`);
  - the second open line for one item is a conflict;
  - the counter never repeats under two concurrent sends;
  - a receipt photo under another store's key is refused;
  - the outbox and the handlers commit or roll back together with the order write (integration).
- **Reviewers:** `go-reviewer`, `tenancy-security-reviewer`.
- **Depends on:** 5.5, 5.7.

### 5.10 `wa.me` links, `inproc`, HTTP, wiring

- **Goal:**
  - `adapter/notify` supplier links (send, falar, avisar);
  - `adapter/inproc/purchasing_orders.go` (`OrderItems` over `app/order`) and `order_purchasing_alerts.go`;
  - the HTTP handlers and DTOs, routes and problem codes;
  - the dispatcher wired in `main.go` with the handlers registered.
  Ticks: "HTTP handlers and OpenAPI".
- **Model:** sonnet. The main session reviews the wiring and the dispatcher registration on Opus before the PR.
- **Skill:** `/new-endpoint` for each operation's handler and tests.
- **Files and owner:** `backend/internal/adapter/http/` (`router.go` owned here), `backend/cmd/api/main.go` (owned here), `backend/internal/adapter/notify/`, `backend/internal/adapter/inproc/`.
- **Tests:**
  - handler tests per operation (status codes, problem codes, `Idempotency-Key`);
  - the visibility tests:
    - no purchasing DTO field reaches a client-tier type;
    - cost and paid price reach the owner only;
    - the supplier link's text never holds a customer field (`TestSupplierLinks_HaveNoClientData`, through HTTP);
  - an API e2e path in `test/e2e`: deposit → lines → send → promised date → receive with one defect → replacement → receive → assembling allowed.
- **Reviewers:** `go-reviewer`, `tenancy-security-reviewer`.
- **Depends on:** 5.7–5.9.

### 5.11 Wording (done in the plan PR)

The pt-BR texts for every M5 screen are drafted in `docs/product/design-document.md`, "Milestone 5 screens", for the user to review in this plan's PR. Web steps use them as written.

### 5.12 Web `packages/api`

- **Goal:** `purchasing.ts` (to-buy, preview, send, supplier orders, promised dates, in transit, undo, prices, notices) and `receipts.ts` (photo upload reusing the catalog upload flow, receipt with `Idempotency-Key`), plus the order and Hoje type changes; MSW handlers from the contract. Ticks: none alone.
- **Model:** sonnet.
- **Files:** `web/packages/api/src/`.
- **Tests (web unit):** each call's path, body, key header and problem mapping; a retried receipt reuses its key.
- **Reviewers:** `web-reviewer`.
- **Depends on:** 5.1.

### 5.13 Web Compras and Fornecedores

- **Goal:**
  - `admin/features/compras`: the tabs "A comprar", "Enviadas", "Receber itens" (5.14) and "Fornecedores";
  - the pedido de compra screen: promised date, in transit, undo, "Informar preço pago" for the owner, notices with "Avisar pelo WhatsApp" and "Marcar como resolvido";
  - the menu entry "Compras" (`purchase:write`), with "Receber itens" as its own entry for `receiving:write` without `purchase:write`.
  Ticks: "Compras: by supplier, send via `wa.me`, promised dates; Fornecedores tab".
- **Model:** sonnet.
- **Files:** `web/apps/admin/src/features/compras/`, the menu.
- **Tests (web unit, MSW):**
  - pick and send opens exactly the link the API answered;
  - the preview is the API's text;
  - a promised date updates the lines;
  - the owner sees Custo and Preço pago, and an admin sees neither;
  - phone layouts at 390 × 844 (cards, full-screen sheets, 48 px targets).
- **Reviewers:** `web-reviewer`.
- **Depends on:** 5.12.

### 5.14 Web Receber itens

- **Goal:** the receiving checklist by supplier order, plus lines bought at the counter: "Tudo certo" / "Com defeito" with the note, photos from the camera or the gallery through the file input, "Salvar recebimento", completion messages, workshop sizes. Ticks: "Receber itens: checklist, item and invoice photos, Tudo certo / Com defeito".
- **Model:** sonnet.
- **Files:** `web/apps/admin/src/features/compras/receber/`.
- **Tests (web unit, MSW):**
  - a defect without a note is refused before the call;
  - a failed save keeps the checks and photos and retries with the same key;
  - photo upload errors;
  - workshop sizes at 390 × 844 (18 px text, 56 px targets, a 64 px primary action).
- **Reviewers:** `web-reviewer`.
- **Depends on:** 5.12.

### 5.15 Web order screen and Hoje changes

- **Goal:**
  - on the order screen, bought items show their purchase status and supplier order with "Ver em Compras" or "Receber itens", and lose the M4 move buttons except "Marcar como instalada";
  - "Falar com o fornecedor" on the delay banner;
  - Hoje's purchasing entries and the "Compras a enviar" card.
  Ticks: none alone (the order and Hoje boxes of M4 stay ticked); it serves the "Done when".
- **Model:** sonnet.
- **Files:** `web/apps/admin/src/features/pedidos/`, `web/apps/admin/src/features/hoje/`.
- **Tests (web unit, MSW):**
  - no bought-item move button but "Marcar como instalada";
  - the supplier chat link is the API's and appears only when the API sends it;
  - Hoje entries open Compras.
- **Reviewers:** `web-reviewer`.
- **Depends on:** 5.12, 5.13 (links into Compras).

### 5.16 Real stack, phone, close

- **Goal:**
  - `web/e2e/compras.spec.ts` against `make stack-up`: an order with a paid deposit, its lines in A comprar, send (the opened link is exactly the API's `https://wa.me/…`, and its text holds no customer name, phone or `P-` number), a promised date, receive one line "Com defeito" with a note and a photo, then the replacement "Tudo certo", and the order allowed into Montagem;
  - `web/e2e/phone-compras.spec.ts` at 390 × 844 with workshop sizes on Receber itens;
  - update `pedidos.spec.ts` and `phone-pedidos.spec.ts`, which received items on the order screen;
  - tick the boxes, update the summary and the audit line.
  Ticks: "Phone: Compras and receiving work at phone size …" (partial: the camera through the shell waits for the device walkthrough) and "Done when".
- **Model:** sonnet for the specs; the main session closes on Opus.
- **Tests:** the specs above; `make lint test test-integration`; the mock-only Playwright part without a stack.
- **Reviewers:** `web-reviewer`, and `tenancy-security-reviewer` over the whole milestone's diff.
- **Depends on:** everything above.

## Build-plan boxes → steps

| Box | Step |
| --- | --- |
| `domain/purchasing` … | 5.2, 5.3 |
| Purchasing migration, with RLS and tests | 5.5 |
| `app/purchasing` … | 5.6, 5.7 |
| Postgres repository …; receipt photo upload through storage | 5.9 |
| HTTP handlers and OpenAPI | 5.1, 5.10 |
| Compras: by supplier, send via `wa.me`, promised dates; Fornecedores tab | 5.13 |
| Receber itens: checklist, item and invoice photos, Tudo certo / Com defeito | 5.14 |
| Phone: Compras and receiving at phone size … | 5.16 (partial until the device walkthrough) |
| Done when (reworded, Q1) | 5.16 |
