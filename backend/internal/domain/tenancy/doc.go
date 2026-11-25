// Package tenancy owns the stores (tenants), their addresses (slugs), their
// teams (memberships and invitations) and their settings.
//
// Rules it enforces:
//   - A slug is 3 to 40 lower-case letters, digits and single hyphens, and
//     never a reserved label: www, api, admin, app, conta, mail, static and
//     the labels of the configured admin and API hosts (ADR 0011).
//   - A store has exactly one owner. The owner is never removed and never
//     changes role, and no one becomes owner by invitation or role change
//     (ADR 0011).
//   - An invitation offers admin, staff or viewer, lasts 7 days and works
//     once. Its link carries the tenant id and a secret whose hash alone is
//     stored.
//   - Settings change only with the version they were read at, and stay
//     within the ranges of docs/api/openapi.yaml (EditableSettings).
//
// The store's billing plan (docs/architecture.md 4.2 lists plan.go) is not
// modelled: no product document defines plans yet.
//
// It imports only the standard library and other domain packages.
package tenancy
