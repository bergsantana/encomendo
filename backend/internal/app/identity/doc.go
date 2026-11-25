// Package identity holds the staff session use cases: sign-up, login,
// refresh, store switch and logout; password reset; and joining a store by
// invitation. It also exports Directory, the read-only user queries the
// tenancy context reaches through adapter/inproc.
//
// Rules it enforces:
//   - Every session-starting call returns the same AuthResult, with every
//     membership read in a scope of the user alone (ADR 0010).
//   - Refresh tokens rotate under their family's lock, with the 2-minute
//     retry window and reuse detection of ADR 0013. Only a refused token is
//     shared.ErrUnauthenticated; a store or dependency failure is returned
//     as it is, so the API answers 5xx and the apps stay signed in.
//   - argon2id runs outside every transaction. The transaction that creates
//     a session re-reads the password hash FOR SHARE and refuses when it
//     changed, so a password reset never misses a login in flight.
//   - Password reset never tells whether an account exists, counts wrong
//     codes atomically, and revokes every family of the account (ADR 0011).
//   - Accepting an invitation needs the account's password, or creates the
//     account; the link alone never logs anyone in.
package identity
