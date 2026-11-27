// Package schedule holds the pure date arithmetic of Encomendo's deadlines.
//
// Rule it enforces: delivery = the latest item ready date + the assembly
// days + the safety margin ("Margem de segurança"); the arrival limit
// ("Precisa chegar até") = delivery - assembly days. Days counts the days,
// ExpectedDate, Delivery and ArrivalLimit work on shared.CivilDate days in
// the store's time zone (ADR 0030, Dates), and Alerts derives "Pode atrasar"
// (at_risk) and "Atrasada" (late) on top of an order's status without ever
// being one.
//
// Everything here is a pure function of its arguments: no clock, no I/O.
// Today comes in as a shared.CivilDate from shared.DateOf.
//
// It imports only the standard library and other domain packages.
package schedule
