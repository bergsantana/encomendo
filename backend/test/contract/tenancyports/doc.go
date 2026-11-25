// Package tenancyports holds the contract suites for the driven ports of
// internal/app/tenancy. Each Run…Contract function takes a constructor, so
// the same cases run against the fakes (fakes_test.go, in the unit tests)
// and against the Postgres repositories in milestone 2's integration step,
// where RLS does what the fake emulates. A fake that fails its contract is
// a bug (backend/CLAUDE.md, Testing).
package tenancyports
