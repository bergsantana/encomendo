# 0028 — Photo uploads are staged, then promoted

- Status: accepted
- Date: 2026-11-27

## Context

ADR 0024 puts photos in an S3-compatible bucket: a signed `PUT` from the app, then a confirm step that checks the object. It also asks for a bucket lifecycle rule that removes uploads never confirmed after a day. ADR 0025 fixes the key, `t/{tenant}/{items|kits}/{owner}/{photo id}`, and `0008_catalog` holds every photo row to it.

Building the storage adapter (plan step 3.9) showed two gaps:

- **Lifecycle.** Cloudflare R2 lifecycle rules match by key prefix only, not by tag. With a single key per photo, an upload never confirmed and a confirmed photo share the prefix `t/`, so no rule can expire one and keep the other.
- **Reuse of the signed URL.** A signed `PUT` works until it expires (5 minutes). An object that passed the confirm check could be replaced through the same URL afterwards, by a file of any size or content, and the photo row would then point at bytes nobody checked.

## Decision

### Two places per key

- The app uploads to `uploads/{key}`, the **staging place**. The key the API returns and the app sends back to confirm stays `{key}`, exactly as ADR 0025 and the contract say; only the adapter knows the staging prefix.
- The bucket has one lifecycle rule: delete objects under `uploads/` one day after upload. Nothing else expires.
- `ConfirmPhoto` reads the staging place: `Stat` returns the size and the ETag, and `Head` reads the first 512 bytes only while the ETag still matches (`If-Match`).
- After the row is recorded, in the same transaction, the adapter **promotes** the upload: a server-side copy from `uploads/{key}` to `{key}`, only while the upload is still the checked version (`x-amz-copy-source-if-match`), with the sniffed content type (`x-amz-metadata-directive: REPLACE`). A failed copy rolls back the row. An upload replaced between the check and the copy is `photo_not_uploaded` ("A foto não foi enviada. Tente de novo.").
- The photo's own key is never signed for writing. Reusing the upload URL only changes the staged copy, which nothing reads after the confirm and which expires.
- The staged copy stays after a promotion until the lifecycle rule removes it, so a confirm retried after a failed commit can copy it again.

### Deletes

- `DeletePhoto` removes the row, then the object at `{key}`. When storage fails after the row is gone, the use case still succeeds and the adapter logs the key at error level. Such an object is outside `uploads/`, so no lifecycle rule removes it: an operator does, from the log. A sweep of objects under `t/` with no row can come with the worker if the logs show the need.
- A refused upload (over 10 MB, or not a JPEG, PNG or WebP by its bytes) is deleted from the staging place at once; if that delete fails, the lifecycle rule removes it within a day, and the refusal is still `photo_invalid`.

### CORS and endpoints

- **Production (R2):** CORS on the bucket allows `PUT` and `GET` from the admin origin only, with the `Content-Type` header. It is set once in the Cloudflare dashboard (`deploy/prod.env.example`). Android needs no CORS. The API and the apps use the same R2 endpoint.
- **Development and CI (Silo, the MinIO fork; ADR 0024 as amended):** the compose service keeps the name `minio`, and a one-shot `minio-init` job on the same pinned image creates the bucket and the lifecycle rule with the bundled `mcli`. The server has no per-bucket CORS, so its `MINIO_API_CORS_ALLOW_ORIGIN` lists the admin origins of the stack and of the Vite dev server. testcontainers runs the same image. The API reaches it at `minio:9000`, and signs URLs for `localhost:9000` (`STORAGE_PUBLIC_ENDPOINT`), which the browser and, through `adb reverse`, the emulator reach (ADR 0025). Signing binds the host, so the two endpoints use two clients, and the signing one never connects.

### Configuration

`STORAGE_ENDPOINT`, `STORAGE_PUBLIC_ENDPOINT` (defaults to the endpoint), `STORAGE_BUCKET`, `STORAGE_REGION` (`us-east-1` by default, `auto` for R2), `STORAGE_ACCESS_KEY`, `STORAGE_SECRET_KEY` and `STORAGE_USE_SSL` (on by default, and refused off outside development and test). The secret is a `config.Secret`, which every format verb and `slog` print as `[redacted]`.

## Consequences

- `appcatalog.PhotoStore` has `PresignPut`, `Stat`, `Head`, `DeleteUpload`, `Promote`, `Delete` and `SignGet`, instead of the five methods the plan lists: the staging place needs its own delete and the promotion.
- A confirmed photo costs one server-side copy, and its staged copy is billed for up to a day. Photos are at most 10 MB and 8 per item or kit, so this is small.
- The R2 lifecycle rule and CORS are manual, one-time settings of the bucket; the runbook lists them in `deploy/prod.env.example`.
