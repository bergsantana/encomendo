# 0027 — Catalog use case choices (step 3.6)

- Status: accepted
- Date: 2026-11-27

## Context

Building `internal/app/catalog` (plan step 3.6) left a few choices that neither `docs/architecture.md`, ADR 0024 nor ADR 0025 makes. Each keeps the dependency rule, tenant isolation and testability intact. The reviews of step 3.6 (`go-reviewer`, `tenancy-security-reviewer`) added D1 to D4.

## Decision

- **An alternative counts as "in the kit".** `item_in_kit` refuses to archive an item that a kit which is not archived holds as a member *or* lists as an alternative. A kit can neither be saved nor restored with an archived alternative (`catalog.NewKit`, `Kit.Restore`), so allowing the archive would leave a live kit that cannot be saved again.
- **Refusals name rows through one app error.** `appcatalog.RefusalError{Err, References}` wraps `catalog.ErrItemInKit` or `catalog.ErrItemArchived` and carries `[]Reference{ID, Name}`; `respond/problem.go` maps `Err` to the code and `References` to the problem's references.
- **Cost fields arrive tri-state as `appcatalog.CostField`,** built only by `KeepCost()` (absent), `ClearCost()` (`null`) and `SetCost(cents)` (a value, in the store's currency); its fields are unexported, and `Sent()` reports whether the key came. Any sent field from a role without `identity.SeesCosts` is `ErrCostForbidden` (matches `shared.ErrForbidden`) before any transaction.
- **The owner's part of a view is one pointer.** `ItemView.Costs *ItemCosts` and `KitView/ItemSummaryView/KitSummaryView.Costs *Costs` are nil for every role but the owner, so "absent" (nil pointer) and "null" (nil field inside) never mix. The supplier items' costs live in `ItemCosts.MainSupplierCost` / `BackupSupplierCost`, not in `SupplierItemView`.
- **Ids sent by a client are kept only when they are the row's own.** An option, choice or field value keeps its id only when the item (or field) already has it, and only the first time it appears in the request; any other id gets a new one. A client can never take over another item's option or another field's value.
- **Items in a scope with no user.** `Items.ByID` and `Items.List` rebuild whole items (supplier links and costs), so they find nothing without a user, and `Items.Insert` and `Items.Update` are refused (an error that matches no sentinel): RLS would hide the supplier rows, and a replace could never drop them. The store page reads `Items.ByIDs` (facts, no cost) and `Items.Published`, which read only `items` and never a staff-only table. The adapter decides this from `Scope.UserID` in one helper, before any SQL.
- **D1, unpublished items on the store page.** An alternative is offered on the store page only when it is published and not archived; a kit item left with no alternative to offer is not swappable. An unpublished *member* of a published kit still shows inside that kit: the kit's contents are what the owner published. `appcatalog.ItemFacts` carries `Published` for this.
- **D2, publishing an archived row is `400 invalid`.** `catalog.ErrPublishArchived` matches `shared.ErrInvalid`, as the contract says for `setItemPublished` and `setKitPublished`.
- **D3, names order by `lower(name)`, then id,** in every list (categories by position first). The cursor holds only the id of the last row of the page and resolves even when that row has left the filter since.
- **D4, store page staleness.** The store page catalog is cached for 5 minutes and dropped by catalog writes only. Kit days there may lag a change of the store's default assembly days or safety margin by up to those 5 minutes; tenancy does not drop the catalog cache in milestone 3.
- **The store page types are lean until milestone 7.** `StoreKit` carries the kit's total days only, not the assembly and safety-margin split, which is the store's own buffer. Store page types carry no photos until milestone 7's public photo path (ADR 0024); admin photo URLs are signed per caller and never cached. `PublicCatalog` and `PublicKit` check `Scope.Validate` before the cache, and a staff scope may call them (a store preview) and gets the same public data.
- **Field errors the domain cannot see** are reported with the domain's in one `ValidationError`: a sourcing mode the store turned off ("Escolha a origem do item."), a supplier the store does not have ("Escolha o fornecedor principal." / "Escolha o fornecedor reserva."), an unknown category ("Escolha uma das categorias da loja.") or field value ("Escolha valores dos campos da loja."). The last two are new wording for the user to review in the design document's "Catálogo" section.
- **`tenancy.Service.StoreDefaults`** is a query outside `UseCases`, like `CheckStore`: it needs only a tenant in the scope, because the store page prices kits with the store's default days. The store page may compute with its safety margin but never shows it.
- **No outbox events in milestone 3.** Nothing consumes a catalog change yet; milestone 5 (purchasing) or 7 (store page) adds events if it needs them.

## Consequences

- Step 3.7's repositories follow the port contracts in `test/contract/catalogports`, including the no-user cases, the `lower(name)` order, and the foreign keys to the store's own categories, values, suppliers and items.
- Step 3.10 maps `CostField` from an `Optional[T]` DTO through its constructors, `RefusalError` to `references`, and `catalog.ErrPublishArchived` (which matches `shared.ErrInvalid`) to `400 invalid` with no special case.
