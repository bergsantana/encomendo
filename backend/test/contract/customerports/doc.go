// Package customerports holds the contract suite for the driven port of
// internal/app/customer. RunRepositoryContract takes a constructor, so the
// same cases run against the fake (fakes_test.go, in the unit tests) and
// against the Postgres repository (test/integration/postgres), where the
// tenant_isolation policy and the unique number of 0012_customers do what
// the fake emulates. A fake that fails its contract is a bug
// (backend/CLAUDE.md, Testing).
package customerports
