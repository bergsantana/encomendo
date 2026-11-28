# 0031 — Milestone 5 purchasing model: who drives bought items, when lines appear, supplier orders, receiving

- Status: accepted (the questions marked "(user)" were answered on 2026-11-28; the rest was settled while planning milestone 5)
- Date: 2026-11-28

## Context

Milestone 5 builds purchasing and receiving (`docs/plans/m5-purchasing-receiving.md`). `docs/architecture.md` 4.2 says purchase lines come after the deposit unless `purchase_before_deposit` is set, that each line points to one order item, that lines are grouped by supplier, that the supplier message never carries the client's name or phone, and that a receipt records `ok` or `defective` per line with item and invoice photos. Milestone 4 left these open (ADR 0030; M4 code design section 8):

- Bought items are moved by hand on the order screen. Nothing says whether that stays once Compras exists.
- `order.deposit_confirmed` and the other order events go to the outbox, but nothing reads the outbox until the worker lands in milestone 6, and `backend/CLAUDE.md` forbids two contexts with ports to each other.
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
- An item bought at the counter is received from Receber itens without a supplier order: an item still pending in "A comprar" can be received directly, as the domain already allows (`to_buy → received`).

### Pending purchases are read, not stored

`backend/CLAUDE.md` forbids two contexts with ports to each other. Purchasing must move order items, so the order context must not call purchasing, not even through an event handler that runs in its transaction. **Purchasing depends on orders, and orders know nothing of purchasing.**

- **A pending purchase ("A comprar") is not a row.** It is a bought item (`to_order`, or `stock` until milestone 6) in status `to_buy`, of an order that is purchasable. Purchasing asks `app/order` for those items through its `OrderItems` port (`adapter/inproc/purchasing_orders.go`).
- **A purchasable order** is not cancelled and not handed over, and either has a release date (the deposit was confirmed, or the owner released it) or, with "Liberar compras antes do sinal" on, has left `draft`. Drafts never count. The store's setting is read when Compras is read.
- **A `purchase_lines` row is written when a line is sent**, or received at the counter without being sent. From then on purchasing owns it.
- So, with no handler and no event:
  - the deposit makes lines appear;
  - a deposit voided and paid again makes nothing new;
  - an item added to a released order appears;
  - a pending item removed, or of a cancelled order, disappears;
  - a backup switch moves a pending item to the other supplier, because the supplier is read from the order's `order_item_sourcing` when Compras is read.
- **Sent lines that are no longer needed are found when read**, and flagged as notices:
  - its order was cancelled;
  - its order item was removed: `purchase_lines.order_item_id` is set to null by `ON DELETE SET NULL (order_item_id)`, and the line keeps its `order_id` and its own snapshot;
  - its order item's quantity or choices changed after it was sent.
- **Purchasing moves the order through `app/order`:** sending, in transit, undo, receiving with a check, promised dates and paid prices are use cases of `app/order` that check their own permission (`purchase:write`, `receiving:write`, the owner for prices). Purchasing calls them in its own transaction, which they join through `WithinTx`'s savepoint, so a send or a receipt commits the lines and the order moves together.
- **What the order screen and Hoje show of purchasing comes from purchasing's own endpoints:**
  - the order's lines, with their C-numbers, promised dates and the "Falar com o fornecedor" link;
  - Hoje's purchasing entries.
  The admin calls them beside the order's and Hoje's own, so `app/order` never reads purchasing.
- **A line takes its supplier, quantity, name, choices and order-time cost from the order item** and its `order_item_sourcing` snapshot when it is sent, never from the catalog as it is now. So the date promised and the line agree.
- **Two people sending the same item at once:** a partial unique index keeps one open line per order item (`sent` or `in_transit`), and the order item's own version is checked, so the second send is a conflict.
- Considered and set aside:
  - an in-transaction event dispatcher that runs purchasing's handlers on order events: it is the cycle `backend/CLAUDE.md` forbids, wired with a registration after construction;
  - moving the outbox relay forward from milestone 6: lines would appear seconds after the deposit, and the worker would need to read across stores. Neither is needed once pending purchases are read.

### Supplier orders and the message (user)

- In Compras the owner or an admin picks a supplier's pending items (all of them are picked by default) and taps "Enviar pelo WhatsApp". That creates a **supplier order** with a per-store number shown as **C-0001** ("pedido de compra"; the same counter pattern as `P-0001`, ADR 0030), writes its lines as `sent`, moves their items to `sent_to_supplier`, and answers the `wa.me` link. The link is recorded with the supplier order, as order messages are.
- **The message sums** the same catalog item with the same option choices across orders into one line, "2× Banco de couro CB 500X (Cor: Preto)", under "Pedido de compra C-0012". It carries the store's name and the supplier's name only. It never carries the client's name, phone or address, nor the client order number. Options are store-defined choices, never free text (ADR 0024), so they can't carry client data either. The test checks the message against every customer field of the orders behind it.
- **Desfazer envio:** while nothing on it is received, the supplier order can be undone. Its lines are cancelled, its items go back to `to_buy` (so they show again in "A comprar"), and its number is never reused.
- **The supplier's promised date** is recorded per supplier order, with a per-line override. It fills `schedule.Timing.Promised` for those items: their expected dates move and alerts follow. The promise to the client (`delivery_date`) still moves only by "Alterar data" (ADR 0030).
- **Price paid (user):** on a sent supplier order the owner can record the unit price actually paid per line. It is stored on the line and copied to `order_item_sourcing.paid_cost_cents` through the `OrderItems` port (an `app/order` use case, owner only). Financeiro's margins use the paid price when there is one, and the order-time cost otherwise. Only the owner sees or writes it (`identity.SeesCosts`).

### Receiving (user)

- A **receipt** is one save on Receber itens: lines of one supplier order, or pending items bought at the counter (a line is written for them as it is received). Each line it holds is checked **Tudo certo** or **Com defeito**.
- **All or nothing per line:** a line is received only when its whole quantity arrived. Partial quantities are not tracked.
- **Photos are optional:** up to 4 invoice photos per receipt and up to 2 photos per line. They are staged and promoted as ADR 0028 says, under `t/{tenant}/receipts/{photo}` (uploaded before the receipt exists, then recorded with it). **Com defeito needs a note** (up to 500 characters).
- **Tudo certo** moves the item to `received` with `ok`.
- **Com defeito** records the check and moves the item back to `to_buy` (the domain's `landing`). The line becomes `defective` and keeps its note and photos. The item, back in `to_buy`, shows again in "A comprar" for the same supplier, marked **Reposição** because it has a defective line. The owner can still switch it to the backup. Returns and refunds with the supplier happen outside Encomendo.
- A receipt is a POST with `Idempotency-Key`.

### Flags and alerts (user)

- **Avisar o fornecedor:** a sent or in-transit line whose order was cancelled, whose item was removed, or whose item's quantity or choices changed is flagged when read. Compras shows it with a prefilled `wa.me` message asking the supplier to cancel, and "Marcar como resolvido", which cancels the line. If the item arrives anyway it is still received; milestone 6 can turn it into stock.
- **Hoje, for `purchase:write` roles only,** from `GET /admin/purchasing/today`:
  - "Compras a enviar", per supplier with pending items;
  - supplier orders sent more than 2 days ago with no promised date;
  - flagged lines.
- **Falar com o fornecedor:** purchasing's view of an order's lines (`GET /admin/purchasing/orders/{orderId}/lines`) carries, for `purchase:write` roles only, a text `wa.me` link to each bought item's supplier, naming the supplier order and the item and nothing about the client. The order screen's delay banner shows it.

### Permissions and visibility

- No new permission. Compras, its tab Fornecedores, supplier orders, promised dates, notices and prices need `purchase:write` (owner, admin). Receber itens needs `receiving:write` (owner, admin, staff). This matches architecture section 8's table.
- Costs and paid prices reach the owner only. Supplier names reach every role that reads the order, as in milestone 4.
- Every purchasing table is in the **staff-only RLS tier** (ADR 0024), with no client policy: a store page scope reads zero rows.

### Dates

- `Order` gains a public redate. Recording a promised date sets the item's `Timing.Promised` and redates it in the same transaction, and `schedule.Recalculate` now saves what it redates.
- **A time zone change still redates nothing** (M4 hand-off, left as it is). Calling `Recalculate` from the settings would make `tenancy` depend on `order`, which already depends on `tenancy`. Dates computed in the old zone differ by at most one day.

## Consequences

- One path moves bought items, so an order and its purchase lines can't disagree, and receiving is where checks and photos live.
- The milestone 4 Playwright specs that received items on the order screen change to receive through Receber itens.
- Orders never wait on purchasing: a deposit, a change or a cancel writes nothing in purchasing. Compras reads the order context each time it is opened, which is cheap at a small shop's volume and is load-tested in milestone 8.
- The M4 hand-offs that assumed handlers (`GeneratePurchases` idempotent per item, `Cancel` calling purchasing) are met by the read instead. `Cancel`'s comment is replaced.
- `stock` items show in "A comprar" until milestone 6 adds reservation (ADR 0030, Q2). Milestone 6 decides what happens to their sent lines when Estoque is turned on.
- A line's `order_item_id` is null once its item was removed from the order; its `order_id` stays. Architecture section 7 reads "`order_item_id` null" as a stock restock, but in M5 a null means "removed". Milestone 6's restock lines get their own column, with a CHECK that a line has an order or a restock.
- Referential actions bypass RLS: the `ON DELETE SET NULL` on a line runs as the table owner. The guard is the order repository's refusal of any write without a staff scope (security review of 5.1).
- Partial deliveries wait for the whole quantity. Counting what arrived can be added later as a received quantity on the line.
