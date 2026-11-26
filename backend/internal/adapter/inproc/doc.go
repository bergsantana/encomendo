// Package inproc connects bounded contexts that live in one process: each
// adapter implements a port of one context by calling the service of
// another, so no context imports another's app package (backend/CLAUDE.md,
// Layers). They translate types and nothing more; the rules stay in the
// context that owns them.
package inproc
