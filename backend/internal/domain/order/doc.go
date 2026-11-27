// Package order is the order aggregate: what a client bought from one store,
// how it is paid, where each item stands and when it will be ready.
//
// The rules it enforces (docs/architecture.md section 4.2; ADR 0030):
//
//   - An order keeps the status workflow version it was made with, and every
//     rule hangs on the order's workflow.Phase, never on a status key or
//     label (ADR 0004).
//   - Advance checks the guard of every phase a move enters: leaving
//     awaiting_deposit needs the deposit paid or the owner's release;
//     entering assembling (or ready, in a workflow without assembling)
//     needs every item arrived and checked; entering handed_over needs the
//     balance paid and how the order was handed over.
//   - Each item keeps a snapshot of what was sold (name, price, sourcing
//     mode, chosen options, lead days), so later catalog edits never change
//     a past order. What the client may see (ItemSnapshot) and what only
//     staff may see (Sourcing: supplier and cost) are separate types, so no
//     code path can put a cost in a client view by accident.
//   - The delivery date is the promise to the client. Item expected dates
//     are the forecast. Alerts compare the forecast with the arrival limit
//     ("Precisa chegar até") and with today; they sit on top of the status
//     and are never statuses.
//   - Money is integer cents of one currency. The deposit due is the total
//     times the deposit percent, rounded half up to the cent.
//   - Events and history entries carry ids, statuses and amounts of the
//     order, never a customer's name or phone.
//
// Who may call what (order:write, order:manage, order:override) is checked
// in the app layer; the domain records who did it.
package order
