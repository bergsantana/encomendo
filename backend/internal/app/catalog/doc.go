// Package catalog holds the Catálogo use cases: categories, item fields
// ("Serve em"), items with their sourcing, and kits, for the management apps;
// plus the cached store page reads (PublicCatalog, PublicKit) that milestone
// 7 serves.
//
// Rules it enforces:
//   - Every admin use case authorizes first with authz: reads need
//     catalog:read (every role), writes catalog:write (owner and admin; ADR
//     0025). Each runs in one transaction scoped to the caller's store, so
//     RLS is the third line of defense after the token and the permission.
//   - Costs and margins are the owner's only (identity.SeesCosts, ADR 0025).
//     One builder makes every admin view, and for any other role the
//     owner-only part (ItemCosts, KitCosts) is nil: no cost value is copied
//     into a view a non-owner receives. A write that sends any cost field,
//     even a null one, from another role is shared.ErrForbidden before any
//     transaction starts; a cost field left out keeps the stored cost, and
//     the domain keeps each cost with its supplier (catalog.ReplaceSourcing).
//   - An item's suppliers must be the store's (SupplierDirectory), its
//     category and field values must exist, and its sourcing mode must be
//     turned on in the store's settings (StoreDefaults); each is a field
//     error, reported with the domain's in one ValidationError.
//   - An item that a kit which is not archived holds, as a member or as an
//     alternative, cannot be archived (catalog.ErrItemInKit), and a kit
//     cannot hold, nor be restored with, an archived item
//     (catalog.ErrItemArchived). Both come back as a *RefusalError naming
//     the rows, so the apps can say which kit or item is in the way.
//   - Aggregates change only at the version the client read: a stale
//     version is shared.ErrVersionConflict, checked before the new values.
//     Photos are their own resource and never change an item's or kit's
//     version (plan 3.6), so a form opened before an upload still saves.
//   - After every committed write the store's catalog cache
//     (t:{tenant}:catalog…) is dropped. A cache that fails is a miss on
//     reads and is ignored on writes, since entries expire in 5 minutes: the
//     cache never fails a request.
//   - The store page reads (PublicCatalog, PublicKit) need only a tenant in
//     the scope, list only published rows that are not archived, and return
//     their own types (StoreItem, StoreKit…), which have no supplier, cost,
//     margin or lead-time-by-supplier field at all
//     (docs/product/design-document.md, System rules). They read items
//     through methods that never touch staff-only tables, so a store page
//     scope (a tenant and no user) reads them under RLS.
//   - M3 publishes no outbox events: nothing consumes a catalog change yet.
package catalog
