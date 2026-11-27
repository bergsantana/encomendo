# 0030 — Milestone 4 order model: entry, dates, items before M5 and M6, payments, links

- Status: accepted (the questions marked "(user)" were answered on 2026-11-27; the rest was settled while planning milestone 4)
- Date: 2026-11-27

## Context

Milestone 4 builds orders, customers, status workflows and deadlines (`docs/plans/m4-customers-orders-deadlines.md`). `docs/architecture.md` 4.2 sets the phases, guards, item lifecycles and the deadline formula, but planning hit gaps it does not close:

- The store page, where clients place orders, is milestone 7. Nothing in M4 creates an order.
- Item statuses are driven by purchasing and receiving (M5) and by stock and production (M6). Without them, no order with a bought item could ever reach Montagem.
- The deadline needs "today", which depends on the store's time zone. Tenant settings have none.
- An order item snapshots its cost for margins, but `order_items` gets a client read policy (architecture 5.5, item 7), so a cost column there would reach the client area.
- Tracking tokens are stored only as hashes (`backend/CLAUDE.md`), so an old link can never be shown again.
- Architecture 4.4 puts `Idempotency-Key` storage in Redis (`redis/idempotency.go`), while section 7 plans an `idempotency_keys` table.
- The delivery date is both computed and changed by the owner; nothing says which one alerts compare against.

## Decision

### Order entry

- **Staff create orders in the admin** ("Novo pedido": a customer, then kits and items with their options). It is the same `app/order` creation and quote code that the M7 store page calls for Monte o seu (`Preview`) and checkout (`PlaceOrder`), so the price, deposit and date have one implementation.
- A kit in an order is one **kit line** priced by `catalog.Kit.PriceWithSwaps`, holding one order item per kit item, each with its own status and ready date. Loose items are lines of one item.
- Each order has a per-store number, shown as `P-0001` (four digits at least), from a per-store counter row taken in the order's insert transaction (`order_counters`, `UPDATE … RETURNING`).
- **Extras on an order** (user): a **Frete** (delivery fee) on delivery orders, a **Desconto** typed in by the owner or an admin, and a **Sinal** percent other than the store's default. Total = lines + frete − desconto; the discount never takes the total below the frete. The deposit due is the total × the order's deposit percent.

### Changing an order

- **Before any payment**, in phase `draft` or `awaiting_deposit`, anyone with `order:write` changes lines, options, frete, handover and address; the discount, the deposit percent and the per-order assembly and margin days need `order:manage`.
- **After a payment, or past `awaiting_deposit`**, items and values can still change when the customer and the seller agree (user):
  - only with `order:manage` (owner, admin), and only **before the order enters `assembling`** (user), or `ready` in a workflow without assembling; later, `order_not_editable`;
  - the customer's approval is **confirmed by the seller** (user): the request carries `customerApproved: true`, the history records who confirmed it, and the answer carries a `wa.me` link summarizing the change for the customer;
  - the order is repriced and redated. When the new forecast is **later than the promise, the promise moves to it** (user) and the summary says so; an earlier forecast leaves the promise alone;
  - when the new total is **below what was paid**, the order shows **"Valor a devolver"** (user) and the balance due is zero; Financeiro lists it, and the refund happens outside Encomendo;
  - a deposit already confirmed stays confirmed: a higher total adds to the balance, not to the deposit.
- Every change reprices and redates the order.

### Snapshots and what the client can read

- `order_items` keeps what the client may see: name, unit price, quantity, sourcing mode, chosen options (label, extra price, extra days), ready days, status and expected date.
- What only staff may see lives in a **staff-only** table, `order_item_sourcing`: main supplier id and name, backup supplier id and lead days, and the cost at order time. It gets the `staff_only` policy (ADR 0024), no client policy and no audit trigger (costs are the owner's; ADR 0025).
- Owner-only costs and margins on the order screen go through `identity.SeesCosts`, as in the catalog.
- `order_messages` is staff-only too: its texts carry live tracking links, which a store page scope must never read.

### Dates

- **Days are calendar days**, as the CB 500X example counts them. Dates are civil dates (year, month, day) in the **store's time zone**, a new tenant setting (user). It is one of four zones, one per Brazilian offset: `America/Noronha`, `America/Sao_Paulo` (the default), `America/Manaus`, `America/Rio_Branco`. The API binary embeds `time/tzdata`, since the distroless image has no zone files.
- **The promise and the forecast are different things.**
  - `orders.delivery_date` is the promise to the client. `schedule` sets it when the order is created (a forecast counted from today), and sets it again when the deposit is confirmed or the owner releases the order, counted from that day. After that, only `ChangeDeliveryDate` moves it: the change is recorded in the history and returns a `wa.me` link to warn the client. Nothing is sent automatically (architecture 4.2).
  - Each item has an **expected date** (when it should be ready), recomputed on every change to the order or the item.
  - "Precisa chegar até" = promise − assembly days. `at_risk` when an item's expected date is after it; `late` when the expected date is before today and the item has not arrived.
- **Alerts are computed on read** by the pure `schedule` functions, with today in the store's time zone, for the order screens and Hoje. Hoje reads active orders only (not `handed_over`, not `cancelled`). The daily check of `cmd/worker` (M6) adds what needs a schedule, such as notifying the owner.

### Items before milestones 5 and 6

- **Staff move items by hand** on the order screen, along each sourcing mode's lifecycle (user): `to_buy → sent_to_supplier → in_transit → received → installed`, with the received check `ok` or `defective` (a defective item goes back to `to_buy`), and `planned → in_production → ready → installed` for in-house items. Milestone 5 drives the same domain methods from Compras and receiving; it decides then whether the manual moves stay for bought items.
- **"Usar o fornecedor reserva"**, the first way to fix a delay on the order screen (design document, Pedido P-0231), lands in M4: an owner or admin (`order:manage`) switches a bought item that is still `to_buy` to its backup supplier from the order's own snapshot. The item's lead days, supplier and cost become the backup's, and the expected date is recomputed. Milestone 5 makes the purchase line follow.
- **A stock item counts its supplier's lead time** until Estoque exists (user). It takes the `to_order` path, with no reservation and no "sem estoque" warning, because nothing knows what is on hand. Milestone 6 adds the reservation and "ready today if on hand".
- **An in-house item counts its production days**, with no wait for capacity until milestone 6.

### Payments

- Recorded by hand, with no provider (ADR 0023), by anyone with `order:write`, so staff can take the Saldo at pickup (user). A payment has a kind (`deposit` or `balance`), an amount, a method (`pix`, `dinheiro`, `cartao_credito`, `cartao_debito`, `transferencia`, `outro`), who recorded it and when. An order may have several.
- The deposit due is the total × the deposit percent, rounded half up to the cent. The deposit is paid when the deposit payments reach it. The balance due is the total minus every payment, never below zero. A payment above what is still due is refused (`payment_exceeds_due`).
- A wrong payment is **voided** by the owner, with a reason, never deleted. The void is in the history, and the guards count only payments that are not void.
- **Releasing an order without the deposit is the owner's**, as architecture 4.2 says "the owner released the order".

### Who does what

Two new permissions join `order:read` and `order:write`, so the apps can hide the buttons as they do for every other right:

| Permission | Roles | Covers |
| --- | --- | --- |
| `order:write` | owner, admin, staff | create, change before any payment, advance, move items, record payments, send the tracking link |
| `order:manage` | owner, admin | cancel, change the promised date (user), change after a payment, discount, deposit percent, per-order days |
| `order:override` | owner | release without the deposit, void a payment |
| `customer:write` | owner, admin | edit customers in Clientes |
- Cancelling keeps the payments as recorded. Refunds are handled outside Encomendo in v1.

### Cancelling

- `cancelled` is reachable before `handed_over`, with a reason. It publishes `order.cancelled` in the same transaction. Milestone 5 cancels the purchase lines not yet sent and flags the sent ones, and milestone 6 releases reservations, each through its own port called from `Cancel` when it lands.

### Tracking links

- The token is created with the order and stored only as SHA-256. Its plain form exists only in the answer to the call that made it.
- "Enviar link de acompanhamento" **issues a new token every time** and returns a `wa.me` link carrying the tracking URL; the previous link stops working (user). The message is recorded in `order_messages`.
- The URL is built from a new setting, `STORE_ORIGIN_TEMPLATE` (`https://{slug}.encomendo.com.br` in production, `http://{slug}.localhost:{port}` locally), plus `/p/{token}`. The template must hold `{slug}`, and it must be `https` outside development, as `ADMIN_ORIGIN` is.
- The tracking route and page are milestone 7. Milestone 4 builds `TrackingView`, the client view type, and its visibility test.

### Idempotency

- `Idempotency-Key` is stored in **Postgres**, in `idempotency_keys` (tenant, key, a SHA-256 of the method, path and body, the id of the resource the first call created, and an expiry of 24 hours), written **in the same transaction** as the write it guards. A retry with the same key and body gets that resource as it is now, with the first call's status; the same key with another body is refused (`idempotency_key_reused`, 422). An id, not a stored body, so a replay never serves a body older than the resource. This replaces `redis/idempotency.go` in architecture 4.4: only a row in the write's own transaction makes a retry exactly-once, and Redis is allowed to fail open (ADR 0021), which a key store must not.
- In M4 it guards `POST /orders` and `POST /orders/{id}/payments`. Later milestones add their creating endpoints.

### Customers

- A customer has a name and a WhatsApp phone (55 + DDD + number, unique per store), and optionally an email, notes and addresses (label, street, number, complement, district, city, state, CEP). A delivery order points at one of the customer's addresses.
- Reading customers is `order:read`. Creating one inside "Novo pedido" is `order:write`. Editing them in Clientes is `customer:write` (owner, admin), matching the parity map (architecture 8.4). Customers are never deleted in M4, since orders point at them.

### Status workflows and settings

- The owner picks the store's preset (`settings:write`, `PUT /api/v1/admin/settings/status-workflow`). New orders take the current one and keep their version (ADR 0004).
- Presets are cached under `g:workflow:` for 10 minutes; they never change once published.
- The settings screen (Configurações) holds the workflow choice with a preview of its steps, and the defaults the API already serves (deposit, assembly, margin, purchases before the deposit, Estoque and Produção, WhatsApp number) plus the new time zone.

### Financeiro

- Owner only (`finance:read`): money received in a period, by kind and method; balances still to receive on active orders; "Valor a devolver" on orders that have it; and the margin of each handed-over order, from the costs snapshotted at order time, where a cost was known.

## Consequences

- New tables beyond architecture section 7: `order_item_sourcing` (staff-only), `order_counters`, `order_lines`; `orders` gains `number`, `release_date`, `released_by`, `freight`, `discount`, `deposit_percent`, `cancel_reason`; `payments` gains `voided_at`, `voided_by`, `void_reason`; `tenant_settings` gains `time_zone`.
- Four new permissions in the role table test: `order:manage`, `order:override`, `customer:write`, and their rows for each role.
- `docs/architecture.md` 4.4 and 7 change at the milestone close: idempotency moves from the Redis adapter to Postgres.
- Milestone 5 owes a decision on manual moves of bought items, and calls from `Cancel` into purchasing; milestone 6 owes the stock and capacity paths in `schedule`, the reservation calls, and the daily check.
- Until milestone 6, a shop that sells from stock sees later dates than it will once Estoque exists. That is the conservative side.
