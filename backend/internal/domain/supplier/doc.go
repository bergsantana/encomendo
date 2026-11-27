// Package supplier owns the store's suppliers and the value that links one to
// a catalog item.
//
// Rules it enforces:
//   - A supplier has a name of 1 to 120 characters and a WhatsApp number
//     that wa.me links accept: digits only, 55 + DDD + 8 or 9 digits
//     (shared.ValidWhatsAppPhone).
//   - A supplier changes only with the version it was read at: the use case
//     checks it with CheckVersion, so two people editing the same supplier
//     never overwrite each other.
//   - A SupplierItem is a supplier's role for one item (main or backup),
//     its lead time in days (0 to 365) and an optional cost. The cost
//     belongs to the pair of item and supplier, never to the slot (ADR
//     0025); the value only holds it, and the catalog applies the swap
//     rules.
//
// Suppliers, their lead times and their costs are internal data: the store
// page, tracking and the client area never carry them
// (docs/product/design-document.md, System rules). Only the owner sees a
// cost (identity.SeesCosts).
//
// It imports only the standard library and other domain packages.
package supplier
