// Package respond reads JSON requests and writes JSON and RFC 9457
// problem+json responses.
//
// problem.go is the only place that maps errors to HTTP status codes. The
// problem "code" values are part of the API contract: add new ones, never
// rename existing ones.
package respond
