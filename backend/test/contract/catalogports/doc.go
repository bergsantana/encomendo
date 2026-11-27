// Package catalogports holds the contract suites for the repository ports
// of internal/app/catalog: Categories, Attributes, Items and Kits.
// RunRepositoryContract takes a constructor, so the same cases run against
// the fakes (fakes_test.go, in the unit tests) and against the Postgres
// repositories of step 3.7, where RLS (tenant_isolation, and staff_only on
// supplier_items and item_costs) does what the fakes emulate. A fake that
// fails its contract is a bug (backend/CLAUDE.md, Testing).
//
// A statement that fails in Postgres aborts its transaction, so each case
// that expects a SQL error (a unique or foreign key violation, a row RLS
// refuses) runs that call in a transaction of its own.
package catalogports
