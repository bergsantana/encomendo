// Package sharedports holds the contract suites for the ports in
// internal/app/shared. Each Run…Contract function takes a constructor, so
// the same cases run against the fakes (fakes_test.go, in the unit tests)
// and against the real adapters (in test/integration, as each adapter
// lands). A fake that fails its contract is a bug (backend/CLAUDE.md,
// Testing).
package sharedports
