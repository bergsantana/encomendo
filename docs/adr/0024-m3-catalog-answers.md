# 0024 — Milestone 3 answers: internal tables, categories, options and photo upload

- Status: accepted; amended by ADR 0025 (costs live on each supplier item, and `item_costs` keeps only in-house costs), by ADR 0028 (uploads are staged under `uploads/`), and on 2026-11-27 (the server below is Silo)
- Date: 2026-11-26

## Context

Before planning milestone 3, four questions were still open: how the database keeps supplier and cost data away from store page requests (the security review of `m2-db` left this as a box in `docs/build-plan.md`), where categories and "Serve em" fields come from before niche templates exist (milestone 7), what a customization option is (`customization_options` has "`jsonb` rules" that no document defines), and how photos reach Cloudflare R2 (ADR 0023). On 2026-11-26 the user answered all four.

## Decision

### Internal tables are staff-only in RLS

- Tables with internal data (suppliers, supplier items, item costs, and later purchasing tables) get a stricter policy than other tenant tables: a row is visible only when the tenant **and** a staff user are set, `tenant_id = current_tenant_id() AND current_user_id() IS NOT NULL`, for `USING` and `WITH CHECK`. A store page scope sets a tenant but no user, so it reads zero rows from these tables, whatever the query.
- The item's cost leaves the `items` table and lives in its own staff-only table (`item_costs`: item, cost in cents, currency), because the store page reads `items`. Margins are computed from it in the app layer and never stored.
- The RLS suite adds a sixth case for each internal table: a scope with the tenant but no user reads zero rows and cannot write.
- App-layer authorization and the client-visibility tests still apply; this is the third layer of rule 3, not a replacement.

### Stores create their own categories and fields

- In milestone 3 the owner creates categories and item fields (such as "Serve em") in Catálogo. They are ordinary tenant data.
- Milestone 7 templates prefill the same tables when a store applies a template. No template table or seed in milestone 3.

### A customization option is a list of choices

- An option has a name (pt-BR, shown to the client), a required flag, and an ordered list of choices. The client picks exactly one choice when the option is required, and at most one otherwise.
- Each choice has a label, an extra price (integer cents, zero or more, the item's currency) and extra days (zero or more), which the deadline adds to the item's ready date.
- No free-text fields and no rules between options in v1. `domain/catalog` validates the shape; the `jsonb` column holds only this shape.

### Photos upload straight to R2 by a signed URL

- The API checks the permission (`catalog:write`), then returns a short-lived presigned `PUT` URL (a few minutes) for an object key it chooses, always under the tenant: `t/{tenantID}/…`. Apps never choose keys.
- The app uploads to that URL, then calls a confirm endpoint. The API checks the stored object (size at most 10 MB; JPEG, PNG or WebP by content, not by name), records the photo row, and deletes the object if it fails. Presigned `PUT` cannot cap the size by itself on R2, so the confirm step is the gate.
- Objects never confirmed are removed by a bucket lifecycle rule (one day).
- The R2 bucket allows `PUT` from the admin web origin only (CORS); Android needs no CORS.
- Development, CI and tests use MinIO, an S3-compatible server, in docker compose and through testcontainers, so the same adapter (`adapter/storage/s3.go`) runs everywhere. `adapter/storage/local.go` is no longer needed for uploads.
  - Amended 2026-11-27, the user's decision: Silo, the maintained MinIO fork, since MinIO's images left Docker Hub in September 2026. The image is `pgsty/silo@sha256:c20d112390b9a9ce8c11b8af95ad35bcaa62f4759663c09c936cd8d2b511f329` (`RELEASE.2026-11-12T00-00-00Z`), pinned by digest; it bundles the client as `mcli`. It speaks the same S3 API and reads the same `MINIO_*` settings, so the adapter is unchanged.

## Consequences

- Milestone 3 has no open questions left. The RLS box in `docs/build-plan.md` is decided here and is built with the catalog and supplier migrations.
- `0007_rls_policies`-style policies need a second form for internal tables; the `/new-migration` skill should name it.
- Store page reads of photos (milestone 7) need a public, read-only path for published items; that is planned in milestone 7.
- New dependencies to justify in their PRs: an S3 client for Go, and MinIO for compose and testcontainers.
- The deadline (milestone 4) adds the extra days of the chosen choices to the item's ready date.
