// Package workflow is the application layer of the status workflow
// context: the global presets each store chooses from (ADR 0004, ADR 0006).
//
// Presets are published versions that never change, so they are cached for
// ten minutes under g:workflow:presets and g:workflow:{id}:{version}, and a
// cache that fails is a miss. The Service lists them for staff (Presets,
// settings:read) and serves them to other contexts through Source, which
// does no authorization because the data is global and read-only. Choosing a
// store's workflow is a settings write and lives in app/tenancy.
package workflow
