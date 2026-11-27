# 0025 — Catalog owns item sourcing; owner-only costs; milestone 3 shapes

- Status: accepted
- Date: 2026-11-26
- Amends: ADR 0024 (where item costs live)

## Context

Planning milestone 3 raised questions that neither `docs/architecture.md` nor ADR 0024 answers. On 2026-11-26 the user answered eight of them; their answers are marked "(user)" below. Planning settled the others, keeping to the dependency rule and tenant isolation.

- **Who sees costs.** The design document shows cost and margin only to the owner on the order screen. But architecture 5.4 lets admins run the catalog and suppliers, and the Catálogo screen shows cost and margin.
- **Where costs live.** The architecture puts a cost on each supplier item. ADR 0024 moves the item's cost into `item_costs`. Having both would give two sources of truth.
- **Who writes the link between an item and its suppliers.** If `app/supplier` wrote supplier items and also had to update catalog items, while `app/catalog` read supplier names for its views, the two contexts would need ports to each other. That is a cycle in `main.go` (`backend/CLAUDE.md`).
- **The store page needs lead times.** Under ADR 0024, a store page scope reads zero rows from supplier tables. Yet the milestone 7 store page shows a ready date for each item and kit, and that date depends on the supplier's lead time.
- **Smaller shapes:** what a swappable kit item can become, what a "Serve em" field holds, how items leave the catalog, where M3 creates suppliers, and which part of the visibility test M3 can run before any store page route exists.

## Decision

### Costs and margins are the owner's only (user)

- `identity.SeesCosts(role)` is the domain policy. It returns true for `owner` only.
- Every admin view type that can carry a cost or a margin leaves those fields out for any other role. They are left out, not set to zero or null.
- A write that sends a cost from any role other than the owner is refused with `forbidden`. A write that leaves a cost out keeps the cost already stored. So an admin can edit an item without seeing or wiping its cost.
- The apps show the cost and margin columns only when the API sends them.

### `catalog:read` for every role

- A new permission, `catalog:read`, is granted to owner, admin, staff and viewer. It covers the catalog and suppliers, without costs.
- `catalog:write` (owner, admin) covers writing both. There is no `supplier:write`: suppliers in milestone 3 exist only to source catalog items. Milestone 5 may split purchasing writes out under `purchase:write`.

### Cost per supplier; `item_costs` only for items with no supplier (user)

- Each supplier item (the main supplier and an optional backup) has its own lead time and its own cost. The cost is optional, since an admin can create the link without it.
- The item's margin is its price minus the main supplier's cost.
- `item_costs` keeps only the own cost of in-house items (`in_house`), which have no supplier. It is a staff-only table, as in ADR 0024.
- Margins are computed in the app layer and never stored.

### Catalog writes item sourcing; `supplier` owns suppliers

- `domain/supplier` holds `Supplier` (name, WhatsApp phone) and the value `SupplierItem` (supplier, role `main` or `backup`, lead days, optional cost), with their validation.
- `catalog.Item` holds its main and backup `supplier.SupplierItem`. Domain packages may import each other.
- `app/catalog` saves items together with their supplier items in one transaction.
- `app/supplier` manages suppliers only, and depends on no other context.
- `app/catalog` reads supplier names through a port (`SupplierDirectory`) implemented in `adapter/inproc` on top of `app/supplier`. That makes the dependency one way: catalog → supplier.
- The `supplier_items` table keeps its name, is staff-only (ADR 0024), and is written by the catalog repository.

### Items carry their ready days

- `items.ready_days` is the number of days the item needs before it is ready, kept by the catalog repository in the same transaction as the item:
  - `to_order` and `stock`: the main supplier's lead time;
  - `in_house`: the production days.
- It holds no supplier and no cost, so the store page (milestone 7) can compute dates from `items` alone, while supplier tables stay invisible to it.
- `stock` items count their supplier lead time until milestone 6 adds stock on hand.

### The kit's days use `domain/schedule`

- `schedule.Days(readyDays []int, assembly, margin int)` returns slowest ready days + assembly + safety margin. It is a pure function, and milestone 4 builds dated deadlines on it.
- The kit editor's "Prazo calculado" uses it, with the kit's assembly and margin overrides or the store defaults.
- The Kit Scrambler CB 500X case (15 + 3 + 2 = 20) is its reference test.

### Swaps go to owner-picked items (user)

- Each swappable kit item lists the items allowed in its place (`kit_item_alternatives`), and the editor suggests items from the same category.
- A swap replaces one kit item with one of its listed alternatives, in the same quantity. A kit item cannot be removed without a replacement.
- Pricing follows the kit's swap mode (ADR 0023), using single-item prices times quantity:
  - `keep`: kit price − removed item + new item;
  - `lose`: the sum of the resulting items' single-item prices.

### Item fields are lists of values (user)

- An attribute such as "Serve em" has a name and an ordered list of values, for example CB 500X and XRE 300.
- Items and kits pick zero or more values of each attribute, so the milestone 7 store page can filter by them.
- Deleting a value removes it from the items and kits that use it, after a confirmation.

### Categories

- An item has at most one category ("Sem categoria" otherwise).
- A category that items still use cannot be deleted.

### Archive, never delete (user)

- Items and kits are archived and can be restored. An archived row is hidden from the store page and from the pickers, and kits and history still resolve it.
- An item in a kit that is not archived cannot be archived until it is taken out of that kit.
- Archiving an item also unpublishes it.

### Suppliers are created inside the item form in M3 (user)

- The web and Android item forms create a supplier inline ("Novo fornecedor": name and WhatsApp) and edit the supplier's lead time and cost on the item.
- The Fornecedores tab, inside Compras, ships in milestone 5.

### Photos

- Items and kits each have up to 8 photos. They are ordered, and the first is the cover.
- Upload follows ADR 0024: a signed `PUT`, then a confirm step. Keys are `t/{tenantID}/{items|kits}/{ownerID}/{photoID}`.
- The confirm step accepts only keys under the caller's tenant and the photo's owner.
- Admin views carry short-lived signed `GET` URLs. The store page's public read path is planned in milestone 7.
- In development, the signed URLs point at MinIO on `localhost:9000`. The Android emulator reaches it through `adb reverse tcp:9000 tcp:9000`, so every signed URL keeps one host.

### What milestone 3 proves about client visibility

- Milestone 3 has no `/api/v1/store/*` route: the store page middleware is milestone 7 (ADR 0010, M2 plan).
- Milestone 3 proves three things:
  1. the RLS sixth case, that a store page scope reads zero supplier and cost rows;
  2. the cached public catalog query returns its own view types, whose JSON has no supplier, cost, margin or lead-time-by-supplier field, tested in `app/catalog`;
  3. HTTP tests show that non-owner roles get no cost or margin field from any catalog or supplier endpoint.
- The HTTP-level store page visibility test lands with the first store route in milestone 7.

### Niche words until templates exist

- Milestone 3 screens need one niche word: "Montagem", in the kit's assembly days.
- Until milestone 7 templates exist, each client reads niche words from one default word set, a module on web and one on Android. It holds Montagem → "Montagem".
- Components never write these words as literals. In milestone 7, the store's template replaces the defaults.

### No idempotency keys yet

- `idempotency_keys` lands with orders in milestone 4. Milestone 3 creates are not idempotent, and Android does not queue catalog writes offline. It caches recent reads only (build plan).

## Consequences

- `supplier_items.cost_cents` is nullable. `item_costs` stays, holding in-house costs only.
- The kit swap line in `backend/CLAUDE.md` changes to describe both modes (step 3.3).
- Milestone 5 reads supplier items through `app/catalog` (or a port on it) when it generates purchase lines.
- The milestone 7 storefront reads only `items`, `kits`, categories, attributes, options and photos, never a staff-only table.
- `catalog:read` enters the contract's `Permission` enum. The web and Android apps gate Catálogo with it.
