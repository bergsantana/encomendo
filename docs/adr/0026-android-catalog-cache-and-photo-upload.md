# 0026 — Android catalog cache and photo upload

- Status: accepted
- Date: 2026-11-27

## Context

Step 3.14 of the milestone 3 plan gives the Android app its catalog and supplier data layer. `docs/architecture.md` says recent catalog reads are cached for offline use and that costs show only when the API sends them (ADR 0025), but not how the cache keeps costs from the wrong role, nor how a photo reaches storage without the session's token.

## Decision

### The cache keeps what the API sent, per store and per costs view

- Room holds two tables, `catalog_pages` (a list page, by list and cursor) and `catalog_details` (an item, kit or supplier). Each row keeps the API's own JSON in a `payload` column and when it was saved. The categories, item fields, items, kits and suppliers lists are cached the same way.
- Keeping the JSON means an owner-only field the API left out stays left out, and one it sent as `null` stays `null`. The app never builds a cost, so it can never fill one for a role that does not see costs.
- Rows are keyed by store and by a costs view (the owner's, or every other role's). Every cache access first deletes the store's rows of the other view, so an owner who is demoted never reads the costs kept for them, and an admin's rows never hold a cost.
- Rows are keyed by store instead of being cleared on store switch or logout, because the existing cache clears nothing yet. The sync state table is not used: each row carries its own save time.
- A read asks the server first and falls back to the cache only on `Failure.Offline`, saying when the copy was saved (`Fetched`). Any other failure (a 403, a suspended store) is shown as it is. Catalog writes are never queued offline (ADR 0025): an offline write is a failure. The detail a write returns is saved, so it opens offline as saved.
- A page that cannot be read or decoded is a miss, and one that cannot be written is skipped, so a broken cache never fails a read. A first page replaces the other pages of its list, and each store keeps the 10 most recent lists and 100 most recent details of each kind.
- The schema moves from version 1 to 2 with an auto-migration, tested by opening a version 1 file with the current database.

### Writes leave a cost out unless the user set it

A cost field has three states: absent, `null` and a value (`OwnerField` in `core:domain`, `OwnerFieldDto` in `core:network`). A write body leaves the field out unless the owner set it, and sends fields the contract requires as explicit `null`.

### Photo bytes go through a client with no Auth plugin

The upload is a `PUT` to the signed URL with exactly the returned headers. It uses its own HTTP client, with no bearer token and no default URL, because the storage host is not the API. A refused upload is `photo_not_uploaded`. The confirm that follows counts `200` and `201` as success.

### Host tests run on the JVM build of the bundled SQLite driver

The Android build of `sqlite-bundled` only carries Android native libraries, so Room tests on the host JVM use `sqlite-bundled-jvm` in the `androidHostTest` source set only. The app itself is unchanged.

### The photo picker is attached by the activity

`PhotoPicker` is a small interface in the new `core:platform` module. Its Android implementation launches the system photo picker and the camera, and needs the activity to register its result launchers before it starts, so `MainActivity` attaches itself in `onCreate`. Without an activity it answers like a user who backed out.

## Consequences

- Step 3.15 and 3.16 screens show an offline banner when `Fetched.fromCache` is true, and show a cost column only when the field is `OwnerField.Shown`.
- If a later step makes logout clear device data, it clears these tables too.
