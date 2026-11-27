// Package schedule holds the pure date arithmetic of Encomendo's deadlines.
//
// Rule it enforces: the days an order or kit needs are the slowest item's
// ready days, plus the assembly days, plus the safety margin ("Margem de
// segurança"); see Days. Dated deadlines, the arrival limit and the alerts
// are built on it in milestone 4.
//
// Everything here is a pure function of its arguments: no clock, no I/O.
//
// It imports only the standard library and other domain packages.
package schedule
