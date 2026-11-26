// Package middleware holds the HTTP middleware the router composes around
// requests: request IDs, panic recovery, access logs and security headers
// on every request; and, on the routes that need them, authentication by
// access token (authn.go), the cross-origin refusal and the deadline of
// /auth/* (crossorigin.go, deadline.go). Rate limiting joins later.
//
// Authentication fails closed: a token that cannot be checked, because the
// denylist cannot be read, is a 503 and never a pass, and never a 401
// either, which would end sessions during an outage (ADR 0013).
package middleware
