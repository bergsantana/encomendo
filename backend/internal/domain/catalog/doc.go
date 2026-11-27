// Package catalog owns what the store sells: items and how each is sourced,
// categories, item fields ("Serve em"), the customization options a client
// picks, photos, and kits with their swap rules.
//
// Rules it enforces:
//   - An item has a price above zero in the store's currency and one of
//     three sourcing modes (ADR 0025). to_order and stock need a main
//     supplier.SupplierItem and allow a backup from another supplier;
//     in_house needs production days (1 to 365) and no supplier. The days an
//     item needs before it is ready come from that sourcing (Item.ReadyDays).
//   - A cost belongs to the pair of item and supplier, never to the main or
//     backup slot. Item.ReplaceSourcing keeps the cost of a supplier that
//     stays linked, starts a newly linked supplier with none and drops an
//     unlinked supplier's cost, so a write that leaves costs out can never
//     move one to another supplier. Leaving in_house drops the production
//     cost.
//   - Costs and margins are the owner's only (identity.SeesCosts). This
//     package computes them; the app layer decides who receives them.
//   - A customization option has 1 to 50 choices with unique labels, each
//     with an extra price and extra days (ADR 0024).
//   - Items and kits hold at most 8 ordered photos; the first is the cover.
//   - Items and kits are archived, never deleted. Archiving unpublishes, and
//     an archived row cannot be published. Whether an item may be archived
//     while a kit holds it is decided by the app layer, which knows the kits
//     (ErrItemInKit).
//   - A kit bundles 1 to 50 items, each at most once, with a quantity of 1
//     to 99. A swappable item lists the items allowed in its place. A kit
//     never holds an archived item; because a kit cannot know that alone, the
//     methods that need item data take an ItemLookup.
//   - A client's swap replaces one kit item with one of its listed
//     alternatives, in the same quantity. The kit's swap mode says what it
//     does to the price: keep the kit discount, or charge the single-item
//     sum (ADR 0023, ADR 0025).
//   - Aggregates change only with the version they were read at: the use
//     case checks it with CheckVersion, so two people editing the same row
//     never overwrite each other.
//
// Field errors are shared.ValidationError values with the contract's field
// names and the pt-BR wording of the "Catálogo" section of
// docs/product/design-document.md. Nested fields are written as
// "options[0].choices[1].label".
//
// Suppliers, costs, margins and lead times by supplier are internal data:
// the store page, tracking and the client area never carry them
// (docs/product/design-document.md, System rules).
//
// It imports only the standard library and other domain packages.
package catalog
