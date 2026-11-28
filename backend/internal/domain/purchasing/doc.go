// Package purchasing is what the store does after it sells something it must
// buy: supplier orders ("pedidos de compra", C-0012), their purchase lines,
// the receipt of what arrives and the notices for lines no longer needed.
//
// The rules it enforces (docs/architecture.md section 4.2; ADR 0031):
//
//   - A pending purchase ("A comprar") is not stored here. A purchase line
//     is written when an item is sent to a supplier, or when an item is
//     received at the counter without being sent. From then on this package
//     owns it.
//   - A supplier order holds the lines of one supplier, never an empty pick
//     and never one order item twice. Each line copies what it buys from the
//     order item when it is sent, so later catalog or order edits do not
//     change it.
//   - Lines move sent -> in_transit -> received or defective, or are
//     cancelled (undone with the supplier order, or a resolved notice). A
//     line is received once and whole: partial quantities are not tracked.
//     A supplier order can be undone only while no line is received or
//     defective.
//   - A defect needs a note of 1 to 500 characters. A receipt holds 1 to 200
//     entries, one per line, up to 4 invoice photos and up to 2 photos per
//     entry.
//   - A notice is an open line whose order was cancelled, whose item was
//     removed, or whose quantity or choices changed. It is read, never
//     stored.
//   - The supplier messages carry the store's and the supplier's names, the
//     supplier order number, quantities, item names and the store's own
//     option choices. They never carry the client's name, phone, address or
//     order number (docs/product/design-document.md, System rules).
//   - Costs and prices paid are integer cents of one currency, and are the
//     owner's only; the caller decides who sees them.
//
// The package imports no other context except catalog (photo content
// types) and shared. Who may call what (purchase:write, receiving:write) is
// checked in the app layer; the domain records who did it.
package purchasing
