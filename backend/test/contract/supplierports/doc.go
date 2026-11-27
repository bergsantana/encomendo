// Package supplierports holds the contract suite for the driven port of
// internal/app/supplier. RunRepositoryContract takes a constructor, so the
// same cases run against the fake (fakes_test.go, in the unit tests) and
// against the Postgres repository in milestone 3's integration step, where
// the staff_only RLS policy does what the fake emulates. A fake that fails
// its contract is a bug (backend/CLAUDE.md, Testing).
package supplierports
