// Package fake holds hand-written in-memory fakes of the ports in
// internal/app/identity, for use case tests: Store (users, refresh tokens,
// reset codes), Hasher, Tokens, Denylist, Mailer, and the two ports into
// tenancy, Memberships and Invitations, which emulate RLS from the scope in
// the context. Every fake passes the contract suite its real adapter must
// pass (test/contract/identityports).
package fake
