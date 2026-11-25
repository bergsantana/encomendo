// Package tenancy holds the store use cases: creating a store, its
// settings, its team ("Equipe") and invitations ("Convites"). It also
// exports Lookup, which the identity context reaches through
// adapter/inproc to read a user's stores and to join one by invitation.
//
// Rules it enforces:
//   - Every use case authorizes first with authz, then runs in one
//     transaction scoped to the caller's store; only CreateTenant sets its
//     own scope, the new store's (ADR 0009).
//   - Team management (invite, revoke, change role, remove) re-reads the
//     caller's membership inside its transaction and never trusts the
//     token's role (ADR 0011).
//   - Losing team management, by demotion or removal, revokes the open
//     invitations that member sent; accepting re-checks the sender too.
//   - The owner is never removed and never changes role; nobody becomes
//     owner by role change or invitation.
//   - The invitation link is only in the email; the store keeps the hash
//     of its secret.
package tenancy
