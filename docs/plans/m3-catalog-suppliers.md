# Milestone 3 — Catalog and suppliers

Written 2026-11-26 on Opus with `/plan-milestone 3`, against `main` at `c3777ce`. Build each step with `/build-step 3.<n>`.

Goal (build plan): the owner can describe what they sell, where it comes from and what it costs, and the store page can read it fast.

## Blocked

Nothing. None of the questions in `docs/architecture.md` section 12 touches milestone 3. ADR 0023 and ADR 0024 answered the milestone's own questions, and the user answered the rest on 2026-11-26 (below).

## The user's answers (2026-11-26)

| # | Question | Answer |
| --- | --- | --- |
| Q1 | Who sees costs and margins? | The owner only. Admins edit the rest of Catálogo; staff and viewers read it without costs |
| Q2 | Where does an item's cost live? | On each supplier item (main and backup). `item_costs` keeps only the cost of in-house items |
| Q3 | What can a swappable kit item become? | Items the owner lists for it; the editor suggests the same category |
| Q4 | What is a "Serve em" field? | A name and a list of values; items and kits pick zero or more |
| Q5 | Where are suppliers created in M3? | Inline in the item form, on web and Android. The Fornecedores tab stays in M5 |
| Q6 | How does an item or kit leave the catalog? | Archived, never deleted, and it can be restored |
| Q7 | Who writes the Catálogo wording? | Claude drafts it in `docs/product/design-document.md`; the user reviews it |
| Q8 | May Claude merge its own M3 PRs? | Yes, backend, web and Android, once CI is green and the reviewers find nothing blocking. CI workflow and scanner config changes still need the user's yes |

## Decisions in this plan

ADR 0025 records them. The short form:

- **Costs and margins are the owner's.** The policy is `identity.SeesCosts(role)`. A cost sent by any other role is refused, and a cost left out of a write is kept.
- **New permission `catalog:read`** for every role. `catalog:write` covers suppliers too.
- **Catalog writes item sourcing.** `catalog.Item` holds its main and backup `supplier.SupplierItem`. `app/supplier` owns suppliers only. Catalog reads supplier names through an `inproc` port, so the dependency runs one way, catalog → supplier, with no cycle in `main.go`.
- **`items.ready_days`** keeps the lead days (main supplier, or production days for in-house items). The milestone 7 store page then computes dates without reading a staff-only table.
- **`schedule.Days`** (slowest + assembly + margin) lands now for the kit's "Prazo calculado", with the CB 500X test. Milestone 4 builds dates on it.
- **No store page route in M3.** Visibility is proven by the RLS sixth case, by the public view types in `app/catalog`, and by HTTP tests that leave costs out for non-owners. The store-route visibility test lands in M7.
- **One default niche word set** on each client ("Montagem" for now), replaced by templates in M7.
- **No `Idempotency-Key` in M3.** It lands with `idempotency_keys` in M4.

## Ownership

This session is the only milestone 3 session. It owns `backend/cmd/api/main.go`, `backend/internal/adapter/http/router.go`, `docs/api/openapi.yaml` and the migrations folder. If parallel sessions start:

- 3.1 owns `openapi.yaml`;
- 3.4 owns the migrations;
- 3.10 owns `router.go` and `main.go`.

## Steps

| Step | Goal | Model |
| --- | --- | --- |
| 3.1 | Contract for catalog, suppliers and photos; `catalog:read` and `SeesCosts` | opus |
| 3.2 | `domain/supplier` and `schedule.Days` | sonnet |
| 3.3 | `domain/catalog`: items, categories, attributes, options, kits, kit swap | sonnet |
| 3.4 | Migrations with staff-only RLS, and the RLS sixth case | opus |
| 3.5 | `app/supplier` | sonnet |
| 3.6 | `app/catalog`: owner-only views, ports, cached public queries | opus |
| 3.7 | `adapter/postgres` catalog and supplier repositories | sonnet |
| 3.8 | `adapter/redis` cache | sonnet |
| 3.9 | `adapter/storage` (S3, R2 and MinIO) and the photo use cases | opus |
| 3.10 | HTTP handlers, routes, wiring and the visibility tests | sonnet |
| 3.11 | Web `packages/api`: `catalog.ts`, `suppliers.ts` | sonnet |
| 3.12 | Web Catálogo: Itens | sonnet |
| 3.13 | Web Catálogo: Kits | sonnet |
| 3.14 | Android data layer: catalog, suppliers, photos, Room cache | sonnet |
| 3.15 | Android Catálogo: Itens | sonnet |
| 3.16 | Android Catálogo: Kits | sonnet |
| 3.17 | Real stack on web and Android, and close the milestone | sonnet |

**Order.** 3.2 to 3.10 are the backend, in dependency order. Once 3.1 is merged:

- 3.11 and 3.14 can start against the contract, using MSW and `MockEngine`;
- 3.12, 3.13, 3.15 and 3.16 follow their data steps;
- 3.17 needs everything.

**Models.** Steps marked opus need `/model opus`, or `claude --model opus`: 3.1, 3.4, 3.6 and 3.9.

---

### 3.1 Contract for catalog, suppliers and photos; `catalog:read` and `SeesCosts`

- **Goal:** the M3 API in `docs/api/openapi.yaml`, so backend, web and Android build against one contract. The permission and cost policy it names exist in `domain/identity`.
- **Ticks:** nothing alone. It is part of the HTTP box, which ticks in 3.10.
- **Model:** opus. The contract decides which fields each role receives. A cost field in the wrong schema leaks.
- **Skill:** none. `/new-endpoint` runs per endpoint from 3.10.
- **Files:** `docs/api/openapi.yaml` (owned); `backend/internal/domain/identity/permission.go` (+ test); `make api-types` output in `web/packages/api/types/`.
- **Work:**
  - **Permissions.** Add `catalog:read` to the `Permission` enum and grant it to every role. Add `identity.SeesCosts(role)`, which is true for owner only, with a table test over the four roles and an unknown one.
  - **Paths, all under `/api/v1/admin`, all tenant-scoped (`tenant_suspended` applies):**
    - Categories: `GET` and `POST /catalog/categories`; `PATCH` and `DELETE /catalog/categories/{id}`. Delete is refused with `409 category_in_use` while an item uses the category.
    - Attributes: `GET` and `POST /catalog/attributes`; `PUT` and `DELETE /catalog/attributes/{id}`. A `PUT` replaces the name and the ordered values; values keep their ids.
    - Items:
      - `GET /catalog/items`, with cursor, `limit`, `q`, `categoryId` and `archived`;
      - `POST /catalog/items`;
      - `GET` and `PUT /catalog/items/{id}`; `PUT` carries `version`;
      - `PUT /catalog/items/{id}/published`, which carries `version`;
      - `POST /catalog/items/{id}/archive` and `/restore`.
    - Kits: the same shape under `/catalog/kits`.
    - Suppliers: `GET` and `POST /suppliers`; `GET` and `PUT /suppliers/{id}`.
    - Photos:
      - `POST /catalog/photo-uploads` takes `{ownerType: item|kit, ownerId, contentType}` and returns `{key, uploadUrl, method, headers, expiresAt}`;
      - `POST /catalog/{items|kits}/{id}/photos` takes `{key}` and returns the photo;
      - `DELETE /catalog/{items|kits}/{id}/photos/{photoId}`;
      - `PUT /catalog/{items|kits}/{id}/photos/order` takes `{photoIds}`.
  - **Schemas:**
    - `Money` is `{cents, currency}`.
    - `SourcingMode`.
    - `SupplierItem` is `{supplierId, supplierName, leadDays, cost?}`.
    - `Item` is:
      - `{id, name, description, categoryId, sourcingMode, price, productionDays, readyDays, mainSupplier, backupSupplier, attributeValueIds, options, photos, published, archived, version}`;
      - plus `cost` and `margin` `{cents, basisPoints}`, which are **not required** and documented as "owner only, absent for other roles".
    - `Kit` is `{…, price, singleItemsTotal, discount, swapDiscount: keep|lose, assemblyDays, safetyMarginDays, readyDays, items[{itemId, quantity, swappable, alternativeItemIds}]}`. It also has owner-only `cost` and `margin`.
    - `CustomizationOption` is `{id, name, required, choices[{id, label, extraPrice, extraDays}]}` (ADR 0024).
    - `Photo` is `{id, url, expiresAt, position}`.
    - Write bodies leave out every computed field (`readyDays`, `margin`, `singleItemsTotal`, `discount`).
    - A `cost` sent by a non-owner is `403 forbidden`.
  - **New problem codes:** `category_in_use`, `item_in_kit` (archive refused), `item_archived` (an archived item put in a kit), `photo_limit` (more than 8), `photo_invalid` (size or type at confirm), `photo_not_uploaded` (no object at the key). `version_conflict` and `forbidden` are reused.
- **Tests:** `make api-types`; the web typecheck; `identity` permission tests; the OpenAPI lint the repository already runs.
- **Reviewers:** `tenancy-security-reviewer` (which fields are owner-only, the key rules), `go-reviewer` (identity).
- **Depends on:** this plan's PR.

### 3.2 `domain/supplier` and `schedule.Days`

- **Goal:** the supplier aggregate, the `SupplierItem` value and the days formula, all pure and tested.
- **Ticks:** "`domain/supplier`: supplier, supplier item (lead time, cost, backup supplier)".
- **Model:** sonnet. ADR 0025 holds the design.
- **Skill:** `/new-bounded-context`, domain part only.
- **Files:** `backend/internal/domain/supplier/{doc,supplier,supplier_item,phone,errors}.go`, `backend/internal/domain/schedule/{doc,days}.go`, tests; typed IDs `SupplierID` in `domain/shared/id.go`.
- **Work:**
  - `supplier.New(tenant, id, name, whatsapp)` and `Rename` / `ChangePhone`, with a version.
  - The WhatsApp phone is validated as 55 + DDD + 8 or 9 digits, digits only (the contract's `SupplierPhone`). Everything else is refused, so `wa.me` links (M5) always work.
  - `SupplierItem{supplier, role main|backup, leadDays 0..365, cost *Money}` with its validation.
  - `schedule.Days(ready []int, assembly, margin int)`.
- **Tests:** table tests; fuzz the phone parser; `schedule` has `TestDays_KitScramblerCB500X` (15, 3, 2 → 20) and an empty kit.
- **Reviewers:** `go-reviewer`.
- **Depends on:** 3.1 (the IDs and codes it names).

### 3.3 `domain/catalog`

- **Goal:** the catalog rules in code: items and sourcing, categories, attributes, customization options, kits, kit swap in both modes, archive.
- **Ticks:**
  - "`domain/catalog`: item, sourcing mode, category, attribute ("Serve em"), customization option, kit, kit swap";
  - "Kit swap price and the Kit Scrambler CB 500X reference case are covered by tests" (with 3.2).
- **Model:** sonnet. ADR 0023, ADR 0024 and ADR 0025 hold every rule.
- **Skill:** `/new-bounded-context`, domain part only.
- **Files:** `backend/internal/domain/catalog/{doc,item,sourcing_mode,category,attribute,customization_option,kit,kit_swap,photo,errors}.go` + tests; `backend/CLAUDE.md` (the kit swap line).
- **Work:**
  - **`Item`:**
    - name and price are required, and the price is greater than 0 in the store currency;
    - `to_order` and `stock` need a main supplier item; `in_house` needs production days (1..365) and no supplier;
    - a backup supplier must differ from the main one;
    - `ReadyDays()` per ADR 0025;
    - `Margin(cost)` returns cents and basis points, with integer rounding toward zero;
    - `Archive()` unpublishes;
    - up to 8 photos, in order.
  - **`CustomizationOption`** (ADR 0024):
    - an option has at least one choice;
    - choice labels are unique within the option;
    - extra price ≥ 0 and extra days ≥ 0.
  - **`Attribute`:** a name and unique, ordered values.
  - **`Kit`:**
    - price > 0 and at least one item; quantity 1..99;
    - a swappable item has at least one alternative, and an alternative is never the item itself or archived;
    - the swap mode defaults to `keep`;
    - assembly and margin overrides are optional, 0..60.
  - **`kit_swap.go`:** `Kit.PriceWithSwaps(prices map[ItemID]Money, swaps []Swap)` for both modes. It refuses a swap to an item that is not listed, or of an item that is not swappable.
  - **Kit totals:** `SingleItemsTotal()` and `Discount()`.
  - In `backend/CLAUDE.md`, replace "The kit discount stays." with the per-kit rule.
- **Tests:**
  - table tests for every invariant;
  - kit swap: keep, lose, two swaps, a quantity of 2, an unknown alternative, a non-swappable item;
  - the CB 500X kit: a leather seat at 15 days, the slowest, gives `schedule.Days` = 20;
  - fuzz the option choices.
- **Reviewers:** `go-reviewer`.
- **Depends on:** 3.2.

### 3.4 Migrations with staff-only RLS, and the RLS sixth case

- **Goal:** the catalog and supplier tables, with ADR 0024's staff-only policy on internal tables, proven by the RLS suite.
- **Ticks:**
  - "Before the first internal table … decide, in an ADR, an RLS tier …";
  - "Migrations for catalog and suppliers, with RLS policies and tests".
- **Model:** opus. Schema and RLS.
- **Skill:** `/new-migration`.
- **Files:**
  - the migrations folder (owned): `0008_catalog.sql` and `0009_suppliers.sql`, or the next free numbers on `main`;
  - `backend/test/integration/rls/`;
  - `.claude/skills/new-migration/SKILL.md`, which gains the staff-only form (ADR 0024 asks for it).
- **Work:**
  - **`0008_catalog`.** Every table is tenant-scoped, with `UNIQUE (tenant_id, id)` and composite foreign keys.
    - `categories`: name, position, unique `(tenant_id, lower(name))`.
    - `attributes` and `attribute_values`: label, position.
    - `items`:
      - name, description, `category_id` (null for none), `sourcing_mode` (check), `price_cents` and `currency`;
      - `production_days`, `ready_days`, `published`, `archived_at`, `version`.
    - `item_attribute_values` and `kit_attribute_values`.
    - `customization_options`: item, name, required, position, `choices jsonb` with a `CHECK (jsonb_typeof(choices) = 'array')`.
    - `kits`: price, `swap_discount` (`keep` / `lose`), `assembly_days`, `safety_margin_days`, `published`, `archived_at`, `version`.
    - `kit_items`: kit, item, quantity, swappable, position.
    - `kit_item_alternatives`.
    - `photos`: `item_id` or `kit_id`, exactly one (`CHECK`); `object_key` unique; `content_type`, `size_bytes`, `position`. The architecture's planned `media` is named `photos` here, because only photos exist.
    - `item_costs`: item PK, `cost_cents`, `currency`. **Staff-only.**
  - **`0009_suppliers`:**
    - `suppliers`: name, `whatsapp_phone`, `version`. **Staff-only.**
    - `supplier_items`: item, supplier, `role` (`main` / `backup`), `lead_days`, `cost_cents` (nullable), `currency`, unique `(tenant_id, item_id, role)`. **Staff-only.**
  - **Policies:**
    - Ordinary tenant tables use `tenant_isolation`.
    - Staff-only tables use `staff_only`: `tenant_id = current_tenant_id() AND current_user_id() IS NOT NULL`, for `USING` and `WITH CHECK`.
    - No client policy on any of these tables.
    - Grants to `encomendo_app` only.
  - **Audit triggers** on the aggregate tables, as M2's tables have them, if `0005`'s trigger applies.
- **Tests:**
  - every new table enters `tenantTables`, which runs the five cases;
  - a new list, `staffOnlyTables`, runs the sixth case: with the tenant set and no user, zero rows on select, and insert, update and delete refused;
  - a test that every table in `staffOnlyTables` has no other policy;
  - `goose up → down → up`;
  - `TestMigrationVersions_AreContiguous`.
- **Reviewers:** `tenancy-security-reviewer`, `go-reviewer`.
- **Depends on:** 3.3, so the columns follow the domain.

### 3.5 `app/supplier`

- **Goal:** supplier use cases with fakes: create, rename and change phone, get, list. Plus the query `Names(ids)` that catalog's port uses.
- **Ticks:** part of "`app/catalog` and `app/supplier` services, cached queries, fakes" (with 3.6).
- **Model:** sonnet.
- **Skill:** `/new-bounded-context`, app part.
- **Files:** `backend/internal/app/supplier/{doc,ports,inputs,service}.go`, `fake/`, `test/contract/supplierports/`.
- **Work:**
  - `catalog:read` for reads and `catalog:write` for writes.
  - Optimistic locking.
  - Supplier names and phones are visible to every role. There are no costs at this level.
  - No cache: supplier data is staff-only, never on the store page.
  - No outbox events in M3; purchasing (M5) adds them if it needs them.
- **Tests:** use cases covering a denied role, the happy path, a version conflict and not found; the repository contract suite against the fake.
- **Reviewers:** `go-reviewer`.
- **Depends on:** 3.2.

### 3.6 `app/catalog`: owner-only views, ports, cached public queries

- **Goal:** catalog use cases with the owner-only cost views, the supplier port, cache invalidation, and the public queries the store page will use.
- **Ticks:** "`app/catalog` and `app/supplier` services, cached queries, fakes".
- **Model:** opus. This is where costs reach or miss a role, and where the cache keys and public view types are set.
- **Skill:** `/new-bounded-context`, app part.
- **Files:**
  - `backend/internal/app/catalog/{doc,ports,inputs,items,kits,categories,attributes,views,public}.go`, `fake/`;
  - `test/contract/catalogports/`;
  - `backend/internal/adapter/inproc/supplier_directory.go`.
- **Work:**
  - **Ports:**
    - `Repository` (items with their supplier items and cost, kits, categories, attributes, photos);
    - `SupplierDirectory` (does the supplier exist, and its name, in this tenant);
    - `StoreSettings` (currency, assembly and margin defaults, and whether stock or production is on: a mode that is off is refused), implemented in `inproc` over `app/tenancy`;
    - `PhotoStore`, defined here and implemented in 3.9;
    - `appshared.Cache`, `TxManager`, `Clock` and `IDGenerator`.
  - **Views:**
    - `ItemView` and `KitView` are built through one function that takes `identity.SeesCosts(principal.Role)`;
    - for other roles the cost and margin fields are absent: pointer fields left nil, and the DTO uses `omitempty`;
    - a cost in the input from a non-owner returns `shared.ErrForbidden`;
    - a cost left out keeps the stored cost.
  - **Writes:** each write runs authz, validate, tx (the domain, then the repository), commit, then `cache.DeleteByPrefix(t:{tid}:catalog:)`.
  - **Public queries:**
    - `PublicCatalog(ctx)` and `PublicKit(ctx, id)` return `StoreItem` and `StoreKit`, types with no internal field;
    - they read only published, non-archived rows;
    - they are cached under `t:{tid}:catalog:store:…` for 5 minutes;
    - a cache failure reads Postgres;
    - nothing calls them over HTTP until M7.
  - `ArchiveItem` returns `catalog.ErrItemInKit` while an active kit holds the item.
  - Photo calls (confirm, order, delete) never change the item's or kit's `version`, because photos are their own resource. Otherwise a form opened before a photo upload would hit a `version_conflict` on save. The web form (3.12) relies on this.
- **Tests:**
  - every use case: a denied role, the happy path, owner versus admin versus viewer views, a cost refused for admin, a cost kept when it is left out, the cache invalidated after each write, the public query cached, a cache error falling back;
  - a JSON test of `StoreItem` and `StoreKit` that walks every key and refuses `cost`, `margin`, `supplier`, `supplierName`, `leadDays` and `atRisk`.
- **Reviewers:** `tenancy-security-reviewer`, `go-reviewer`.
- **Depends on:** 3.3, 3.5.

### 3.7 `adapter/postgres` catalog and supplier repositories

- **Goal:** GORM repositories for both contexts, passing their port contract suites against Postgres.
- **Ticks:** part of "`adapter/postgres` repositories; `adapter/redis` cache …" (with 3.8).
- **Model:** sonnet. The `backend/CLAUDE.md` GORM rules hold the design.
- **Skill:** none.
- **Files:** `backend/internal/adapter/postgres/{catalog_repo,supplier_repo}.go`, `models/{catalog,supplier}.go`, `test/integration/postgres/`.
- **Work:**
  - Save an item, its supplier items, `item_costs` and `ready_days` in one write.
  - Optimistic locking.
  - Keyset pagination by `(name, id)`, with the search under `ILIKE` and bound parameters.
  - `23505` maps to a conflict and `23503` to an invalid reference.
  - Updates use maps or a named `Select`, never `Select("*")`.
- **Tests:**
  - the contract suites (`catalogports`, `supplierports`) run against Postgres, as `encomendo_app`;
  - constraint errors map to domain errors;
  - a staff scope reads costs, and a tenant-only scope gets the item without its supplier items. The repository must survive the zero rows RLS returns.
- **Reviewers:** `tenancy-security-reviewer`, `go-reviewer`.
- **Depends on:** 3.4, 3.6.

### 3.8 `adapter/redis` cache

- **Goal:** `appshared.Cache` on Redis, with the key-prefix check, TTL, prefix delete and the fallback when Redis is down.
- **Ticks:** "`adapter/postgres` repositories; `adapter/redis` cache (tenant-prefixed keys, `g:` for globals, Redis-down fallback) with integration tests".
- **Model:** sonnet. The rules are written in the architecture section 4 and in `appshared.Key`.
- **Skill:** none.
- **Files:** `backend/internal/adapter/redis/cache.go`, `test/contract/sharedports` (the Redis run), `test/integration/redis/cache_test.go`.
- **Work:**
  - JSON values.
  - `DeleteByPrefix` uses `SCAN` + `UNLINK`, never `KEYS`.
  - The adapter checks the key prefix again.
  - Errors are wrapped as `shared.ErrUnavailable`, so the app layer can fall back.
- **Tests:** the shared contract suite against Redis; TTL; prefix isolation between two tenants; Redis stopped mid-test, so the use case reads from the fake repository and logs.
- **Reviewers:** `tenancy-security-reviewer`, `go-reviewer`.
- **Depends on:** 3.6.

### 3.9 `adapter/storage` and the photo use cases

- **Goal:** signed upload and confirm for item and kit photos, on any S3-compatible store: MinIO in development and tests, R2 in production. Amended 2026-11-27: the server is Silo, the maintained MinIO fork (`pgsty/silo@sha256:c20d112390b9a9ce8c11b8af95ad35bcaa62f4759663c09c936cd8d2b511f329`, `RELEASE.2026-11-12T00-00-00Z`), since MinIO's images left Docker Hub in September 2026 (the user's decision; ADR 0024). Uploads are staged under `uploads/` and promoted on confirm (ADR 0028).
- **Ticks:** "`adapter/storage`: local and S3-compatible photo storage …". The box's "local" storage is dropped by ADR 0024, and the box is reworded.
- **Model:** opus. The keys, signed URLs and content checks decide whether one tenant can write or read another's objects.
- **Skill:** none.
- **Files:**
  - `backend/internal/adapter/storage/{doc,s3}.go` and tests;
  - `backend/internal/app/catalog/photos.go`;
  - `backend/internal/platform/config`;
  - `deploy/docker-compose*.yml`, for MinIO (Silo), a bucket-init job (`mcli`, same image) with CORS for the admin origin, and a lifecycle rule;
  - `deploy/prod.env.example`;
  - `test/integration/storage/`.
- **Work:**
  - **New dependency:** `github.com/minio/minio-go/v7`. It is a single S3-compatible client that works with R2 and MinIO, smaller than `aws-sdk-go-v2`. Justify it in the PR. MinIO (Silo, pinned by digest) joins testcontainers.
  - **`PhotoStore` port:**
    - `PresignPut(key, contentType, ttl)`;
    - `Stat(key)`;
    - `Head512(key)`, which reads the first 512 bytes to sniff the type;
    - `Delete(key)`;
    - `PresignGet(key, ttl)`.
  - **`RequestPhotoUpload`:**
    - checks `catalog:write`, that the owner exists in the tenant, and the count (< 8);
    - builds the key `t/{tid}/{items|kits}/{ownerID}/{photoID}`;
    - signs for 5 minutes, with the content type bound.
  - **`ConfirmPhoto`**, in this order (tenancy review of 3.1):
    1. **Parse the key** exactly as `t/{principal's tenant}/{items|kits}/{path owner id}/{photoID}`, with nothing after it. On failure, return `photo_not_uploaded` and never touch storage. The answer is the same as for a missing object, so it is no existence oracle.
    2. **A row already exists for `photoID`:**
       - on the same owner, return it with `200`;
       - otherwise, `photo_not_uploaded`.
       - Never read or delete in either case.
       - This check runs before the 8-photo limit, so a retried confirm of the 8th photo answers `200`, not `photo_limit`.
    3. **`Stat`.** A missing object is `photo_not_uploaded`.
    4. **Check the object:** size ≤ 10 MB and a sniffed type of JPEG, PNG or WebP. On failure, delete the object and return `photo_invalid`.
    5. **Record the row** with `photoID` as its primary key. A unique-violation race returns the existing photo; it is not a delete.
  - Views sign `GET` URLs for 10 minutes.
  - Config holds the endpoint, the public endpoint, the bucket, the region, and the access key and secret. Secrets are never logged.
- **Tests:**
  - unit: key building and parsing, including another tenant's key, another owner's key, a trailing segment, `..` and a URL-encoded slash. None of them reaches storage, checked with a fake that fails on any call;
  - a repeat confirm returns the existing photo and deletes nothing;
  - use cases with a fake store;
  - integration with MinIO (Silo):
    - presign, then a `PUT` with `net/http`, then confirm;
    - an 11 MB object refused and deleted;
    - a `.jpg` name holding text refused;
    - an expired URL.
- **Reviewers:** `tenancy-security-reviewer`, `go-reviewer`.
- **Depends on:** 3.6, 3.7.

### 3.10 HTTP handlers, routes, wiring and the visibility tests

- **Goal:** every 3.1 endpoint served, wired in `main.go`, with tests validated against the contract.
- **Ticks:** "HTTP handlers, OpenAPI entries, and the "clients never see costs or suppliers" visibility test". The M3 part of the test is defined in ADR 0025; the box notes that the store-route part lands in M7.
- **Model:** sonnet. `/new-endpoint` holds the layout; authorization lives in the app layer.
- **Skill:** `/new-endpoint`, per endpoint group.
- **Files:**
  - `backend/internal/adapter/http/handler/{catalog,supplier}.go` and `dto/`;
  - `router.go` (owned);
  - `cmd/api/main.go` (owned);
  - `respond/problem.go`, for the new codes;
  - `docs/api/openapi.yaml` (owned): remove every `x-unserved: step 3.10` mark. The e2e suite fails on a marked operation the server answers, and runs the full 401 and suspension checks on each one once its mark is gone.
- **Tests:**
  - `httptest` with fake use cases: status codes, problem codes, DTO validation with unknown fields refused, `version_conflict`;
  - responses validated against `openapi.yaml`;
  - **cost fields are tri-state in requests:** absent keeps the cost, `null` clears it, and a value sets it. Use a small `Optional[T]` DTO type whose `UnmarshalJSON` runs only when the key is present. Use flat structs for `Replace…Request`, never an embedded input, so a promoted `UnmarshalJSON` cannot swallow `version`. Test all three states, plus a non-owner sending the key, even as `null`, which gets 403;
  - **cost fields in responses:** present as `null` for an owner with no cost, absent for others. Plain `*Money` with `omitempty` cannot do this, so use separate owner and non-owner view structs, or a `Nullable[T]`;
  - an unknown field in every request body gives `400 invalid`. The schemas under `allOf` cannot say so, so only `DisallowUnknownFields` and this test enforce it;
  - **visibility:** for viewer, staff and admin principals, every catalog and supplier response JSON has no `cost`, `productionCost` or `margin` key, at any depth; the owner's response has them;
  - the M2 route test that lists every route, updated;
  - an admin `PUT` that leaves costs out keeps each supplier's own cost:
    - when the main supplier is replaced, the new one starts with none;
    - when main and backup swap, each keeps its own;
    - when a supplier is unlinked, its cost goes;
    - when the item leaves `in_house`, the production cost goes (tenancy review of 3.1).
- **Reviewers:** `go-reviewer`, `tenancy-security-reviewer`.
- **Depends on:** 3.5 to 3.9.

### 3.11 Web `packages/api`: `catalog.ts`, `suppliers.ts`

- **Goal:** typed calls for every 3.1 endpoint, plus `uploadPhoto` (request, `PUT`, confirm).
- **Ticks:** "`packages/api`: `catalog.ts`, `suppliers.ts`".
- **Model:** sonnet.
- **Skill:** none.
- **Files:** `web/packages/api/src/{catalog,suppliers}.ts` and tests; `index.ts`.
- **Tests:**
  - MSW for each call and its problem codes;
  - `uploadPhoto` sends the signed headers and no `Authorization` to the storage host;
  - a failed confirm surfaces `photo_invalid`.
- **Reviewers:** `web-reviewer`.
- **Depends on:** 3.1.

### 3.12 Web Catálogo: Itens

- **Goal:** the Itens tab and the item drawer, with the wording in the design document's "Catálogo" section.
- **Ticks:** nothing alone. It is part of the web Catálogo box (with 3.13).
- **Model:** sonnet.
- **Skill:** none.
- **Files:**
  - `web/apps/admin/src/features/catalogo/{views,components,store.ts}`;
  - the router entry, with `meta.permission` set to `catalog:read`;
  - the menu entry;
  - `web/packages/ui`, if a shared component is needed;
  - the default niche words module.
- **Work:**
  - The list has its filters, archived items and "Carregar mais".
  - The drawer:
    - sourcing modes, filtered by the store settings;
    - main and backup supplier, with "Novo fornecedor" inline, lead days and owner-only cost;
    - production days and owner-only production cost;
    - attribute values and options;
    - photos (upload, cover, order, remove);
    - "No portal", archive and restore.
  - The "Categorias" and "Campos" dialogs.
  - Read-only for roles without `catalog:write`.
  - The cost and margin columns appear only when the API sends them.
- **Tests:**
  - Vitest with MSW: an owner sees margin, while an admin and a viewer see no column;
  - the viewer has no edit controls;
  - the inline supplier;
  - field errors from the wording;
  - an archive refused with `item_in_kit`;
  - a version conflict;
  - a photo upload and a refusal.
- **Reviewers:** `web-reviewer`.
- **Depends on:** 3.11; 3.10 for the real stack.

### 3.13 Web Catálogo: Kits

- **Goal:** the Kits tab and "Editar kit": items, quantity, swappable items with their alternatives, the swap mode, overrides, totals and "Prazo calculado".
- **Ticks:** "Catálogo screens: items, sourcing mode, supplier, cost, price, margin, "No portal", photos; Kits tab with swappable items".
- **Model:** sonnet.
- **Skill:** none.
- **Files:** `web/apps/admin/src/features/catalogo/`.
- **Tests:**
  - totals and days shown exactly as the API sends them, never computed;
  - same-category suggestions;
  - both swap modes saved;
  - margin shown for the owner only.
- **Reviewers:** `web-reviewer`.
- **Depends on:** 3.12.

### 3.14 Android data layer: catalog, suppliers, photos, Room cache

- **Goal:** repositories, DTOs, Room cache of recent catalog reads, and a photo picker behind `expect`/`actual`.
- **Ticks:** "Fornecedores tab data layer (screens ship with Compras in milestone 5)".
- **Model:** sonnet.
- **Skill:** none.
- **Files:**
  - `mobile/shared/core/{domain,network,database,data,testing}`;
  - a new `core:platform` module for `PhotoPicker`, with an `androidMain` actual over the system photo picker and camera;
  - `core:di`.
- **Work:**
  - Every call goes through `StoreCalls.inStore`.
  - Uploads read the signed URL and `PUT` with a bare Ktor client, never the bearer client, then confirm.
  - Room tables for the item and kit lists and details, with a Room migration.
  - Costs are stored only when the API sent them, and dropped when the role changes.
  - `catalog:read` joins `Permission`.
- **Tests:**
  - `MockEngine` for every call and problem code;
  - the upload uses no bearer;
  - Room in memory, with the migration;
  - offline reads served from the cache;
  - the repositories with fakes.
- **Reviewers:** `mobile-reviewer`.
- **Depends on:** 3.1.

### 3.15 Android Catálogo: Itens

- **Goal:** `feature/catalogo` items: the list and the item form, with the same rules and wording as 3.12, at workshop sizes where the screen is used with gloves.
- **Ticks:** nothing alone. It is part of "`feature/catalogo`: items and kits, with offline cache of recent reads" (with 3.16).
- **Model:** sonnet.
- **Skill:** none.
- **Files:**
  - `mobile/shared/feature/catalogo/`;
  - `core:navigation`, for the menu entry gated by `catalog:read`, in the web's order (ADR 0020);
  - string resources.
- **Tests:**
  - ViewModels with Turbine;
  - Compose UI tests: owner versus admin columns, viewer read-only, inline supplier, an offline banner over cached data.
- **Reviewers:** `mobile-reviewer`.
- **Depends on:** 3.14.

### 3.16 Android Catálogo: Kits

- **Goal:** the kits list and kit editor, as in 3.13.
- **Ticks:** "`feature/catalogo`: items and kits, with offline cache of recent reads".
- **Model:** sonnet.
- **Skill:** none.
- **Files:** `mobile/shared/feature/catalogo/`.
- **Tests:** ViewModel and UI tests for swaps, modes and the owner-only margin.
- **Reviewers:** `mobile-reviewer`.
- **Depends on:** 3.15.

### 3.17 Real stack on web and Android, and close the milestone

- **Goal:** prove the "Done when" against `make stack-up`, and tick the milestone.
- **Ticks:** "A catalog item with a supplier and lead time can be created on web and on Android and appears on the other".
- **Model:** sonnet.
- **Skill:** none.
- **Files:**
  - `docs/build-plan.md`, for the summary, "What exists today", the cross-cutting tracks and the decisions log;
  - `docs/architecture.md`: section 4.4 (`storage/` is S3 only; `photos` replaces `media`) and section 7 (the tables as built);
  - Playwright specs;
  - `scripts/android-emulator-docker.sh` (MinIO port, `adb reverse`);
  - fixes.
- **Work:**
  - **On web:** create an item with a new supplier, lead time and a photo. Check that it shows on Android.
  - **On Android:** create one with a supplier and lead time, and edit the web item's price. Check both on web.
  - **As an admin:** confirm that no cost shows on either app.
  - **Offline on Android:** the list still shows.
  - **Coverage:** check the gate.
  - **Device gaps:** the M2 gaps (keyboard inset, two-store switch) are checked if an on-screen-keyboard AVD runs.
- **Tests:** a Playwright flow for item and kit; the emulator walkthrough with screenshots in the PR; `make lint test test-integration`; `./gradlew check`.
- **Reviewers:** `web-reviewer`, `mobile-reviewer`.
- **Depends on:** all.
