# 0027 — Catalog use case choices (step 3.6)

- Status: accepted
- Date: 2026-11-27

## Context

Building `internal/app/catalog` (plan step 3.6) left a few choices that neither `docs/architecture.md`, ADR 0024 nor ADR 0025 makes. Each keeps the dependency rule, tenant isolation and testability intact.

## Decision

- **An alternative counts as "in the kit".** `item_in_kit` refuses to archive an item that a kit which is not archived holds as a member *or* lists as an alternative. A kit can neither be saved nor restored with an archived alternative (`catalog.NewKit`, `Kit.Restore`), so allowing the archive would leave a live kit that cannot be saved again.
- **Refusals name rows through one app error.** `appcatalog.RefusalError{Err, References}` wraps `catalog.ErrItemInKit` or `catalog.ErrItemArchived` and carries `[]Reference{ID, Name}`; `respond/problem.go` maps `Err` to the code and `References` to the problem's references.
- **Cost fields arrive tri-state as `appcatalog.CostField{Sent, Cents}`.** Absent keeps, `Sent` with nil `Cents` clears, a value sets, in the store's currency. Any `Sent` field from a role without `identity.SeesCosts` is `ErrCostForbidden` (matches `shared.ErrForbidden`) before any transaction.
- **The owner's part of a view is one pointer.** `ItemView.Costs *ItemCosts` and `KitView/ItemSummaryView/KitSummaryView.Costs *Costs` are nil for every role but the owner, so "absent" (nil pointer) and "null" (nil field inside) never mix. The supplier items' costs live in `ItemCosts.MainSupplierCost` / `BackupSupplierCost`, not in `SupplierItemView`.
- **Ids sent by a client are kept only when they are the row's own.** An option, choice or field value keeps its id only when the item (or field) already has it, and only the first time it appears in the request; any other id gets a new one. A client can never take over another item's option or another field's value.
- **Items in a scope with no user.** `Items.ByID` and `Items.List` rebuild whole items (supplier links and costs), so they find nothing without a user; the store page reads `Items.ByIDs` (facts, no cost) and `Items.Published`, which read only `items` and never a staff-only table.
- **The store page types are lean until milestone 7.** `StoreKit` carries the kit's total days only, not the assembly and safety-margin split, which is the store's own buffer. Store page types carry no photos until milestone 7's public photo path (ADR 0024); admin photo URLs are signed per caller and never cached. An archived alternative is left out of a store page kit, and a kit item left with none is not swappable.
- **Field errors the domain cannot see** are reported with the domain's in one `ValidationError`: a sourcing mode the store turned off ("Escolha a origem do item."), a supplier the store does not have ("Escolha o fornecedor principal." / "Escolha o fornecedor reserva."), an unknown category ("Escolha uma das categorias da loja.") or field value ("Escolha valores dos campos da loja."). The last two are new wording for the user to review in the design document's "Catálogo" section.
- **`tenancy.Service.StoreDefaults`** is a query outside `UseCases`, like `CheckStore`: it needs only a tenant in the scope, because the store page prices kits with the store's default days.

## Consequences

- Step 3.7's repositories follow the port contracts in `test/contract/catalogports`, including the no-user cases.
- Step 3.10 maps `CostField` from an `Optional[T]` DTO, `RefusalError` to `references`, and `catalog.ErrPublishArchived` to the contract's `400 invalid` (the domain sentinel matches `shared.ErrConflict`).
