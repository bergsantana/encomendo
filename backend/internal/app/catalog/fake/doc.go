// Package fake holds hand-written in-memory fakes of the ports of
// internal/app/catalog, for use case tests:
//   - Store implements the four repositories (Categories, Attributes, Items,
//     Kits) over one set of maps, so a field value removed from an attribute
//     leaves the items and kits that picked it, as the foreign keys of
//     0008_catalog do. It emulates RLS from the scope in the context: rows
//     are visible only in their store's scope, and the staff-only tables
//     (supplier_items, item_costs) only with a user as well. It passes the
//     contract suite its real adapter must pass (test/contract/catalogports).
//   - Suppliers, Defaults and Photos fake the supplier directory, the
//     store's defaults and the photo URL signer.
package fake
