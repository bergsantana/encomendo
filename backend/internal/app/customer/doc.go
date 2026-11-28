// Package customer holds the customer use cases: list, get, find
// by WhatsApp number, create and update.
//
// Rules it enforces:
//   - Every use case authorizes first with authz: reads need order:read,
//     Create needs order:write because Novo pedido creates customers, and
//     Update needs customer:write (ADR 0030). Viewers read customers; only
//     roles that write orders create them.
//   - Every use case then runs in one transaction scoped to the caller's
//     store, so RLS (tenant_isolation on customers and customer_addresses)
//     is the third line of defense after the token and the permission check.
//   - A WhatsApp number belongs to one customer of the store: a second
//     customer with it is ErrPhoneTaken (409), on create and on update.
//   - A customer changes only with the version the client read: a stale
//     version is shared.ErrVersionConflict, checked before the new values are
//     validated.
//   - A customer's addresses are replaced as a set. Create gives every
//     address a new id; Update keeps the id of an address the customer
//     already has and gives every other one a new id, so an id sent by a
//     client can never reach another customer's address.
//   - Customer data is never cached: it is personal data of the store's
//     clients and only staff read it (docs/product/design-document.md,
//     System rules).
//   - M4 publishes no outbox events: nothing consumes a customer change yet.
package customer
