// Package orderports holds the contract suite for the driven port of
// internal/app/order, apporder.Repository. RunRepositoryContract takes a
// constructor, so the same cases run against the fake (fakes_test.go, in the
// unit tests) and against the Postgres repository (test/integration/postgres),
// where the keyset queries, the version check and the number counter are SQL
// that the fake emulates. A fake that fails its contract is a bug
// (backend/CLAUDE.md, Testing).
//
// The suite builds orders with the domain only (internal/testkit is for
// tests, and this package is imported by them). It puts an order in a state
// the way the repository sees it: through order.Record and order.Restore.
package orderports
