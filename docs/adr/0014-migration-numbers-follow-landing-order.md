# 0014 — Migration numbers follow the order they land

- Status: accepted
- Date: 2026-11-25

## Context

`docs/architecture.md` section 7 gave every migration a fixed number, `0001` to `0015`, in schema order: catalog `0006`, orders `0010`, platform `0014`, RLS policies `0015`. The milestones land them in a different order. Milestone 2 needs `0014_platform` (the outbox and the audit log) and `0015_rls_policies`, and later milestones would then add `0006` to `0013` below a version the database already has.

goose refuses that by default: a file numbered below the database's current version is a "missing migration", and `up` stops. CI would not notice, because it migrates an empty database in file order. The error would first appear on a database that already has `0015`: a developer's volume, staging or production. Allowing out-of-order migrations would hide it instead, and leave production applying files in a different order from CI. The same problem would come back in milestone 6: inventory is `0009` in the list but lands after orders, `0010`.

## Decision

- A migration takes the **next free number on `main`** when it lands, as the `new-migration` skill already says. Section 7 lists the planned tables in the order they are expected, without numbers.
- Milestone 2 lands `0002_identity`, `0003_workflows`, `0004_tenancy`, `0005_rls_helpers`, `0006_platform` and `0007_rls_policies`. `0002` to `0005` keep their planned numbers, so ADR 0006 still holds.
- `0007_rls_policies` covers the tables of `0004` and `0006`. Their policies need the helper functions of `0005`, which come after `0004`. Every later migration enables RLS and writes the policies for its own tables in the same file, as the skill shows, so a table is never granted to `encomendo_app` without RLS.
- A unit test fails when the versions in `migrations/` are not exactly `1, 2, 3 …`. A gap, a duplicate or a number taken out of order breaks `make test` before review. Two pull requests that pick the same number collide in CI once both are merged, and the second one renumbers.
- `0006_platform` holds `audit_log` and `outbox_events`. `idempotency_keys` waits for its first user in milestone 5, because its columns depend on how responses are replayed.

## Consequences

- `docs/architecture.md` section 7, `docs/build-plan.md` and `docs/parallel-sessions.md` use the new names.
- A session that needs a migration rebases on `main` right before it takes a number.
- goose keeps its default: it never runs out-of-order migrations.
