// Package fake holds hand-written in-memory fakes of the ports of
// internal/app/catalog, for use case tests:
//   - Store implements the repositories (Categories, Attributes, Items,
//     Kits, Photos) over one set of maps, so a field value removed from an
//     attribute leaves the items and kits that picked it, as the foreign
//     keys of 0008_catalog do, and the photos Photos records come back with
//     their item or kit. It emulates RLS from the scope in the context: rows
//     are visible only in their store's scope, and the staff-only tables
//     (supplier_items, item_costs) only with a user as well. It passes the
//     contract suite its real adapter must pass (test/contract/catalogports).
//   - Suppliers and Defaults fake the supplier directory and the store's
//     defaults.
//   - Storage fakes the photo object store (PhotoStore and its URL signer),
//     with a staging area for uploads, and logs every call so a test can
//     prove a refused key never reached storage.
package fake
