// Package identity owns who the people are: staff users and their
// passwords, global client accounts and their login identities, emails and
// names, refresh token rules, password reset codes, and what each staff role
// may do.
//
// Rules it enforces:
//   - The role → permission map lives here, in code, and is tested role by
//     role (docs/architecture.md section 5.4). Roles themselves are
//     shared.Role, because the request principal carries them.
//   - Passwords are 8 to 128 characters and are kept only as a hash. Emails
//     compare without regard to case (Email.Key), as the database does.
//   - A refresh token rotates on every use; presenting it again is reuse
//     and revokes its family, except for a retry within RefreshRetryWindow
//     of its first use while its successor is untouched (ADR 0013,
//     RefreshToken.UseAt).
//   - Password reset codes are six digits, stored as an HMAC, valid for 10
//     minutes and 5 wrong tries, with 10 wrong codes per account per day
//     (ADR 0011).
//   - Client accounts are global, not tenant-scoped, and never have a role.
//
// It imports only the standard library and other domain packages.
package identity
