// Package purchasing holds the use cases of Compras and Receber itens
// (docs/architecture.md section 4.2; ADR 0031): "A comprar" by supplier,
// pedidos de compra sent through wa.me links, the supplier's promised
// dates, receiving with checks and photos, and the notices of sent lines no
// longer needed.
//
// Purchasing depends on orders and orders never know purchasing
// (backend/CLAUDE.md: no two contexts with ports to each other). "A comprar"
// is read from the order context through OrderItems, and a purchase line is
// written only when it is sent or received at the counter. Every move of a
// bought item goes back to the order context through the same port, in the
// caller's transaction, so the lines and the order commit together.
//
// Rules this package enforces:
//   - a purchase goes out only after the deposit, unless the store buys
//     before it (the order context decides which items are to buy);
//   - the supplier texts never carry the client's name, phone, address or
//     order number (domain/purchasing builds them from MessageFacts, which
//     has no field for any of those);
//   - costs and prices paid reach the owner only (identity.SeesCosts).
package purchasing
