// Package auth is the adapter for credentials: access tokens (JWT signed
// with Ed25519, docs/architecture.md 5.1 and ADR 0003), password hashes
// (argon2id) and the random secrets behind refresh tokens, invitation links
// and reset codes. ADR 0019 records its formats and costs.
//
// It holds no business rules: who may get a token, and when a refresh token
// rotates or revokes its family, is app/identity's (ADR 0013). What it
// guarantees is that every access token it accepts was signed by one of its
// keys, for the audience the caller asked for, and has not expired; that no
// error it returns carries a password, a hash or a key, and none carries a
// token beyond a capped reason that may quote a character of a bad one; and
// that no key prints.
package auth
