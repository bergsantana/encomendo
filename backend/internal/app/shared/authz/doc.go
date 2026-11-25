// Package authz is the second of the three tenant isolation checks
// (docs/architecture.md section 2, rule 3): it carries the request's
// Principal in the context and checks it at the start of every use case,
// before any repository runs. The first check is the token itself; the third
// is Postgres RLS, fed by the Scope that WithPrincipal derives from the same
// principal.
//
// Clients hide UI by permission, but this package is the authority.
package authz
