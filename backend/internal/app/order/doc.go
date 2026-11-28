// Package order holds the order use cases: making, quoting and changing an
// order, its payments, moves and dates, and the views staff and clients get
// of it (docs/architecture.md section 4.3; ADR 0030).
//
// Rules it adds to the domain's:
//
//   - Authorization comes first in every use case. order:write runs an
//     order day to day; order:manage cancels, moves the promised date,
//     changes a paid order and sets terms other than the defaults (or than
//     the order's own, on a change); order:override, the owner's, releases
//     without the deposit and voids payments (ADR 0030).
//   - Every staff view is built by toView, which leaves costs and margins
//     out for every role but the owner (identity.SeesCosts). The client's
//     tracking view is its own type, TrackingView.
//   - Creating calls take an Idempotency-Key, checked after the permission
//     and remembered in the write's own transaction.
//   - Messages are wa.me links the user sends; nothing leaves the server.
//     A stored message never holds a tracking link (ADR 0030, Amendments).
//   - Lines a change sends back unchanged keep their price and snapshot, so
//     later catalog edits never reprice a past order.
//
// Hand-offs: milestone 5 cancels purchases from order.cancelled, and must
// generate purchases idempotently per order item, since
// order.deposit_confirmed fires again after a voided deposit is paid again.
// Milestone 6 releases stock reservations. Milestone 7's store page reuses
// Create and Quote; it runs with no staff user, so order_item_sourcing
// (staff-only) cannot be written there, and a customer matched only by phone
// must answer no customer facts and use no stored address
// (docs/plans/m4-code-design.md section 8).
package order
