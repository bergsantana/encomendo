// Package workflow owns the shape of an order's life: the fixed phases, the
// status workflow a store picks, its validation and the rules for moving an
// order from one status to another (ADR 0004).
//
// Rules it enforces:
//   - The phases are fixed in code: draft, awaiting_deposit, confirmed,
//     sourcing, assembling, ready, handed_over, plus cancelled. Every rule
//     about orders hangs on a Phase, never on a status key or label.
//   - A workflow is an ordered list of statuses, each mapped to one phase.
//     Phases never go back along the list, the list starts at draft and ends
//     at handed_over, and it holds awaiting_deposit and ready too.
//   - Cancelled is implicit in every workflow. It is never a row and its key
//     is reserved.
//   - A status moves forward to any later status, or back to an earlier
//     status of the same phase. A cancelled or handed-over order never moves
//     again, and any earlier order may be cancelled.
//   - Moving only says which statuses are allowed. Whether an order may
//     enter a phase (deposit paid, items checked) is a guard of the order
//     aggregate, which walks PhasesEntered.
//   - Published workflows are global data, and an order keeps the version it
//     was created with, so a Workflow never changes after New.
//
// Labels are pt-BR and come from the workflow's data. The staff label and
// the client label are separate: the client label never names suppliers or
// purchases (docs/product/design-document.md, System rules).
//
// It imports only the standard library and internal/domain/shared.
package workflow
