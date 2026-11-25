// Package identityports holds the contract suites for the driven ports of
// internal/app/identity. Each Run…Contract function takes a constructor, so
// the same cases run against the fakes (fakes_test.go, in the unit tests)
// and against the real adapters: the Postgres repositories and the inproc
// ports in milestone 2's integration step, the argon2id hasher in
// adapter/auth. A fake that fails its contract is a bug (backend/CLAUDE.md,
// Testing).
package identityports
