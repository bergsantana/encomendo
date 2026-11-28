# 0031 — Milestone 5 purchasing model: who drives bought items, when lines appear, supplier orders, receiving

- Status: accepted (the questions marked "(user)" were answered on 2026-11-28; the rest was settled while planning milestone 5)
- Date: 2026-11-28

## Context

Milestone 5 builds purchasing and receiving (`docs/plans/m5-purchasing-receiving.md`). `docs/architecture.md` 4.2 says purchase lines come after the deposit unless `purchase_before_deposit` is set, that each line points to one order item, that lines are grouped by supplier, that the supplier message never carries the client's name or phone, and that a receipt records `ok` or `defective` per line with item and invoice photos. Milestone 4 left these open (ADR 0030; M4 code design section 8):

- Bought items are moved by hand on the order screen. Nothing says whether that stays once Compras exists.
- `order.deposit_confirmed` and the other order events go to the outbox, but nothing reads the outbox until the worker lands in milestone 6.
- `order.Cancel` must cancel unsent purchase lines and flag sent ones; an order change can add or remove bought items.
- `deposit_confirmed` fires again when a voided deposit is paid again.
- `schedule.Timing.Promised` waits for the supplier's promised date, and `schedule.Recalculate` cannot redate yet.
- The build plan's "Done when" asks for items "received on a phone with no signal → synced later", while ADR 0029 dropped offline use for v1.

## Decision

### Online only (user)

Receiving needs a connection, like every admin screen (ADR 0029). A tap or an upload that fails is retried with the same `Idempotency-Key`, so nothing is lost or recorded twice. The "Done when" box is reworded to say so. Offline use stays an open question after v1.

### Compras drives bought items (user)

- For a `to_order` item (and a `stock` item until milestone 6), **purchasing is the only path** from `to_buy` to `received`: sending a supplier order moves its items to `sent_to_supplier`, "Marcar como em trânsito" to `in_transit`, a receipt to `received` with its check, and "Desfazer envio" back to `to_buy`.
- The order's move endpoint refuses those moves for bought items (`409 item_moved_by_purchasing`) and keeps only `received → installed`. In-house items keep their manual moves until milestone 6's production board.
- The order screen shows each bought item's status with its supplier order ("C-0012") and links to Compras or Receber itens.
- An item bought at the counter is received from Receber itens without a supplier order: a line still `pending` can be received directly, as the domain already allows (`to_buy → received`).

### Purchases start inside the order's transaction

- An **in-transaction event dispatcher** (`adapter/inproc/event_dispatcher.go`) wraps the outbox publisher. It stores the event exactly as today, then calls the handlers registered for its name, synchronously, in the same transaction and scope. `cmd/api/main.go` registers purchasing's handlers. `app/order` publishes events as it does now and never names purchasing.
- A handler that fails fails the order's write, so an order is never confirmed without its lines. A handler that is not registered changes nothing.
- Handlers are internal entry points: no HTTP route reaches them, they check no permission (the order write that published the event was already authorized), and they act in the caller's store only. Each has a test that it runs in the publishing transaction and rolls back with it.
- Considered and set aside:
  - a direct `Purchasing` port on `app/order`, because the two services would need each other at construction;
  - moving the outbox relay forward from milestone 6, because lines would appear seconds after the deposit, and the relay, its deploy service and its retry policy would all land here. The relay still lands in milestone 6, for consumers that may lag.
- Purchasing reads and moves order items through an `OrderItems` port (`adapter/inproc/purchasing_orders.go`, over `app/order`). Order writes made from purchasing join the caller's transaction through `WithinTx`'s savepoint.

### When lines appear

- **After the deposit:** on `order.deposit_confirmed` and `order.released`, one `pending` line for each bought item of the order.
- **With "Liberar compras antes do sinal" on:** when the order leaves `draft` (`order.advanced` out of the draft phase). Drafts never make lines.
- **Idempotent per order item:** a partial unique index keeps at most one open line (`pending` or `sent`) per order item, so a deposit paid again after a void makes nothing new.
- **Order changes** (`order.changed`): added bought items get lines if the order is already past its trigger; removed items cancel their `pending` lines and **flag** their `sent` lines for the owner.
- **Cancelled orders** (`order.cancelled`): the same as removing every item.
- **Backup supplier** (`order.supplier_switched`, a new event): a `pending` line follows the item to the backup supplier. A sent line can't switch.
- In-house items never make lines. Restock lines with no order item are milestone 6's.
- A line takes its supplier, quantity, name, options and order-time cost from the order item and its `order_item_sourcing` snapshot, never from the catalog as it is now, so the date promised and the line agree.

### Supplier orders and the message (user)

- In Compras the owner or an admin picks a supplier's `pending` lines (all of them are picked by default) and taps "Enviar pelo WhatsApp". That creates a **supplier order** with a per-store number shown as **C-0001** ("pedido de compra"; the same counter pattern as `P-0001`, ADR 0030), moves its lines to `sent`, and answers the `wa.me` link. The link is recorded with the supplier order, as order messages are.
- **The message sums** the same catalog item with the same option choices across orders into one line, "2× Banco de couro CB 500X (Cor: Preto)", under "Pedido de compra C-0012". It carries the store's name and the supplier's name only. It never carries the client's name, phone or address, nor the client order number. Options are store-defined choices, never free text (ADR 0024), so they can't carry client data either. The test checks the message against every customer field of the orders behind it.
- **Desfazer envio:** while nothing on it is received, the supplier order can be undone. Its lines go back to `pending`, its items to `to_buy`, and its number is never reused.
- **The supplier's promised date** is recorded per supplier order, with a per-line override. It fills `schedule.Timing.Promised` for those items: their expected dates move and alerts follow. The promise to the client (`delivery_date`) still moves only by "Alterar data" (ADR 0030).
- **Price paid (user):** on a sent supplier order the owner can record the unit price actually paid per line. It is stored on the line and copied to `order_item_sourcing.paid_cost_cents` through the `OrderItems` port. Financeiro's margins use the paid price when there is one, and the order-time cost otherwise. Only the owner sees or writes it (`identity.SeesCosts`).

### Receiving (user)

- A **receipt** is one save on Receber itens: lines of one supplier order, or `pending` lines bought at the counter. Each line it holds is checked **Tudo certo** or **Com defeito**.
- **All or nothing per line:** a line is received only when its whole quantity arrived. Partial quantities are not tracked.
- **Photos are optional:** up to 4 invoice photos per receipt and up to 2 photos per line. They are staged and promoted as ADR 0028 says, under `t/{tenant}/receipts/{receipt}/{photo}`. **Com defeito needs a note** (up to 500 characters).
- **Tudo certo** moves the item to `received` with `ok`.
- **Com defeito** records the check and moves the item back to `to_buy` (the domain's `landing`). The line becomes `defective`, keeps its note and photos, and a new `pending` **replacement line** ("Reposição") for the same supplier appears in Compras. The owner can still switch it to the backup. Returns and refunds with the supplier happen outside Encomendo.
- A receipt is a POST with `Idempotency-Key`.

### Flags and alerts (user)

- **Avisar o fornecedor:** a `sent` line whose item was removed or whose order was cancelled is flagged. Compras shows it with a prefilled `wa.me` message asking the supplier to cancel, and "Marcar como resolvido", which cancels the line. If the item arrives anyway it is still received; milestone 6 can turn it into stock.
- **Hoje, for `purchase:write` roles only:**
  - "Compras a enviar", per supplier with `pending` lines;
  - supplier orders sent more than 2 days ago with no promised date;
  - flagged lines.
- **Falar com o fornecedor:** each bought item of the order view carries a text `wa.me` link to its supplier, for `purchase:write` roles only, naming the supplier order and the item and nothing about the client. The delay banner shows it.

### Permissions and visibility

- No new permission. Compras, its tab Fornecedores, supplier orders, promised dates, notices and prices need `purchase:write` (owner, admin). Receber itens needs `receiving:write` (owner, admin, staff). This matches architecture section 8's table.
- Costs and paid prices reach the owner only. Supplier names reach every role that reads the order, as in milestone 4.
- Every purchasing table is in the **staff-only RLS tier** (ADR 0024), with no client policy: a store page scope reads zero rows.

### Dates

- `Order` gains a public redate. Recording a promised date calls it in the same transaction, and `schedule.Recalculate` saves with it.
- A time zone change in Configurações calls `Recalculate` for the store's active orders (M4 hand-off).

## Consequences

- One path moves bought items, so an order and its purchase lines can't disagree, and receiving is where checks and photos live.
- The milestone 4 Playwright specs that received items on the order screen change to receive through Receber itens.
- Synchronous handlers make an order write slower by the purchasing work it causes. That is a few rows per item, in the same transaction.
- The outbox relay of milestone 6 must not run these handlers again: they are in-process and run once, at publish time. Outbox consumers of milestone 6 are separate subscribers.
- `stock` items make purchase lines until milestone 6 adds reservation (ADR 0030, Q2). Milestone 6 decides what happens to their open lines when Estoque is turned on.
- Partial deliveries wait for the whole quantity. Counting what arrived can be added later as a received quantity on the line.
