// Package identity owns who the people are: staff users and their
// passwords, global client accounts, emails, and what each staff role may
// do.
//
// Rules it enforces:
//   - The role → permission map lives here, in code, and is tested role by
//     role (docs/architecture.md section 5.4). Roles themselves are
//     shared.Role, because the request principal carries them.
//   - Client accounts are global, not tenant-scoped, and never have a role.
//
// It imports only the standard library and other domain packages.
package identity
