// Package supplier holds the supplier use cases ("Fornecedores"): create,
// replace (rename and change the WhatsApp number), get, list, and the Names
// read the catalog reaches through adapter/inproc to label an item's
// suppliers.
//
// Rules it enforces:
//   - Every use case authorizes first with authz: reads need catalog:read
//     (every role), writes catalog:write (owner and admin; ADR 0025). Names
//     needs catalog:read too, so a caller that may not read suppliers learns
//     none of their names.
//   - Every use case then runs in one transaction scoped to the caller's
//     store, so RLS (staff-only tables) is the third line of defense after
//     the token and the permission check.
//   - A supplier changes only with the version the client read: a stale
//     version is shared.ErrVersionConflict, checked before the new values
//     are validated.
//   - A supplier carries no cost, and nothing here is cached: suppliers are
//     staff-only data and never reach the store page (ADR 0024,
//     docs/product/design-document.md, System rules).
//   - M3 publishes no outbox events: nothing consumes a supplier change yet.
package supplier
