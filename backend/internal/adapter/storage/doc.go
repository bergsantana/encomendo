// Package storage keeps item and kit photos in an S3-compatible bucket:
// MinIO in development, CI and tests, Cloudflare R2 in production, through
// one client, github.com/minio/minio-go/v7 (ADR 0024).
//
// What it guarantees:
//   - Apps upload straight to the bucket with a URL the API signed, never
//     through the API. The signature binds the method, the object, the
//     Content-Type and the expiry, and names the public endpoint, the host
//     browsers and the Android emulator reach (localhost:9000 locally,
//     through adb reverse; ADR 0025). Reads, copies and deletes use the
//     internal endpoint.
//   - Uploads land under UploadPrefix, which the bucket's lifecycle rule
//     empties after a day, so an upload never confirmed goes away on its
//     own (ADR 0028). A confirmed upload is copied, only while it is still
//     the version the API checked, to the photo's own key, which no signed
//     URL can write: a URL reused within its 5 minutes cannot replace a
//     photo after its check.
//   - Every failure of the store matches shared.ErrUnavailable, so the API
//     answers 503 and never mistakes an outage for a refusal; a missing or
//     replaced object is appcatalog.ErrObjectMissing or ErrObjectChanged.
//   - It never logs the secret key or a signed URL; logs name the key,
//     which holds only ids.
//
// Keys are chosen by internal/domain/catalog (PhotoKey); this package
// stores whatever key it is given and does not check it.
package storage
