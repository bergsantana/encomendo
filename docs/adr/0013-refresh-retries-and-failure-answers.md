# 0013 — Refresh retries, server failures and rate-limit answers

- Status: accepted; amends ADR 0010 ("One refresh at a time")
- Date: 2026-11-25

## Context

ADR 0010 rotates the refresh token on every use and treats any second use as theft: the whole family is revoked. It has no grace window, and clients serialize their refreshes so that two of them never race.

ADR 0012 (Android) ends a session only when `/auth/refresh` answers 400, 401 or 403. Offline, 429 and 5xx keep the user signed in, because the app is used in workshops with weak signal. Its request timeout is 30 seconds.

The mobile and web sessions pointed out what serializing does not cover, and what the server must do for ADR 0012 to work:

- **A lost answer.** The server rotates the token, the answer is dropped on a weak connection, and the client's next attempt presents the old token. Under ADR 0010 that is reuse: the family is revoked and the user is sent to login. On web the same happens when a page reloads while a refresh is in flight and the browser drops the `Set-Cookie`.
- **A server failure answered as 401** would log out every Android user during a Postgres or Redis outage.
- **The cross-origin check on `/auth/*`** must let native clients through: OkHttp sends neither `Origin` nor `Sec-Fetch-Site`, and a 403 on refresh ends the session.
- **Android shows the wait** from `Retry-After`, so every 429 must carry it.

## Decision

### A short retry window

- Presenting a refresh token again is a **retry**, not reuse, when both hold:
  - it was first used at most **2 minutes** ago;
  - the token it was rotated to has been neither used nor superseded.
- A retry supersedes that unused successor and does what the call asks again, from the presented token: refresh rotates it within its family for the same store; switch rotates it for the requested store, after the same membership check. Each family therefore keeps exactly one live token.
- The 2 minutes count from the first use, so retries never extend the window. Outside it, or once the successor has been used, presenting the token is reuse, as in ADR 0010: the family is revoked and the answer is `401 unauthenticated`. The API still never says which case it was.
- **Why 2 minutes:** a 30-second timeout plus one or two retries fit inside it, and it is short next to the 15 minutes between an active client's refreshes.
- **Why theft detection still works:** a thief who presents a stolen token inside the window gets the family's only live token. The owner's next refresh then presents a superseded token, which is reuse and revokes the family, the thief's token included. That next refresh can be days away when the owner's app stays closed, and until then the thief keeps the session. A thief who uses a stolen token before its owner does keeps it just as long, so this is not new. The window only adds 2 minutes, after each rotation, in which a stolen old token is still worth trying.
- **Clients still serialize refreshes** (the Web Lock and the Android mutex of ADR 0010). The window covers lost answers, not parallel refreshes: two refreshes in flight at once leave one client holding a revoked token.
- The access token issued with a superseded successor stays valid until it expires, at most 15 minutes. In the lost-answer case nobody received it.

### One family, one lock

The first design marked each token row revoked. The security review of `m2-db` showed the race in that: a revoke that runs while a rotation is inserting a successor does not see the new row, so the successor stays live. A thief holding a stolen session could keep a rotation in flight and survive the owner's password reset.

- A family is a row of `refresh_families`: one login, its owner and its audience. Every rotation, retry and switch starts by locking that row (`SELECT … FOR UPDATE`) and refuses a revoked family. So everything that happens to one login happens one step at a time, a retry included.
- **Decide only after the lock.** Finding the family takes a first read of the token by hash, which can be stale by the time the lock is granted: two presentations of one unused token would both see it unused. So, after the lock, a new statement reads the token and its successor again, and only that read decides between rotation, retry and reuse. The changes are conditional too: `SET used_at = now(), replaced_by = $2 WHERE id = $1 AND used_at IS NULL AND superseded_at IS NULL`, and superseding a successor `WHERE … AND used_at IS NULL AND superseded_at IS NULL`. No row changed means another call got there first, and the call takes the retry or reuse path instead. One `JOIN … FOR UPDATE OF` the family is not enough: after waiting, Postgres re-reads only the locked row, not the token's columns.
- Revoking a family, on reuse or logout, sets `refresh_families.revoked_at`. A token is live only when it is unused (or a valid retry), not superseded, not expired, and its family is live. A revoke that meets a rotation in flight waits for it, then kills the family with the token it created.
- Revoking every family of an account, on password reset, first updates the `users` row (the new password hash), then the families.
- Login and invitation acceptance check the password like this:
  1. Verify argon2id against the hash read without a lock, so the slow check never holds a lock or a transaction.
  2. In the transaction that creates the family, read the hash again with `SELECT password_hash FROM users WHERE id = $1 FOR SHARE`.
  3. Refuse with `invalid_credentials` if it differs from the hash just verified.

  A login whose lock comes first makes the reset wait and is then revoked with the rest; one that comes after the reset sees a new hash and fails. For acceptance this is its second, user-scoped transaction (ADR 0009, ADR 0011), the one that builds the session.
- A token's own `superseded_at` means only that a retry replaced it before anyone used it; presenting it later is reuse. A dead family is never recorded on the token rows.
- A successor is always in its token's family, and a token's audience is its family's (composite keys), so lookups by hash ask for staff tokens only and a client token never carries a store.
- Revoking a family also puts the access tokens issued with it in the last 15 minutes and 30 seconds (their lifetime plus the verifier's leeway) on the Redis denylist, from `refresh_tokens.access_jti`, so the thief's access token stops too. The revoke in Postgres commits even when the Redis write fails; that failure is logged, not returned.

### Failures are never 401

- `401 unauthenticated` means the server checked the token and refuses it. When Postgres or Redis fails while reading a refresh token, a membership or the access-token denylist, the answer is `500 internal` or `503 not_ready`, never 401.
- Repositories return `shared.ErrNotFound` only for a missing row and pass every other error through. `app/identity` turns only a missing or refused token into `shared.ErrUnauthenticated`.
- The authn middleware fails closed: if the denylist cannot be read, the request answers `503 not_ready`, not 401 and not a pass.

### Validate before rotating

`/auth/switch-tenant` checks the body and the membership before it rotates anything. Its `400` and `403` leave the refresh token and the session as they were, so a client must not end the session on them: only the refresh call's own refusal ends it (ADR 0012).

### Native clients and `Retry-After`

- ADR 0010 already lets a request with neither `Origin` nor `Sec-Fetch-Site` through the cross-origin check. `m2-integrate` adds an HTTP test for it: neither header, `Content-Type: application/json; charset=utf-8` as Ktor sends it, `refreshTransport: body`, answered 200.
- Every 429 carries `Retry-After` in whole seconds, at least 1. The contract marks the header required, and `m2-integrate` tests it on every limited route.

### One rule for both apps

Both apps end the session only when `/auth/refresh` answers 400, 401 or 403 (ADR 0012). The contract states this, so the web's refresh-once-on-401 follows the same rule. A 401 from `/auth/login` or `/auth/invitations/accept` is `invalid_credentials`, a form error, and never triggers a refresh.

## Consequences

- `m2-db` adds `refresh_families`, and `replaced_by`, `superseded_at` and `access_jti` on `refresh_tokens`.
- `m2-domain` and `m2-auth` test the window's edges: a retry inside it, a retry after the successor was used, a retry after 2 minutes, repeated retries, two presentations at once, and a failing repository that must not become `ErrUnauthenticated`.
- The repositories of `m2-integrate` test the locks against Postgres:
  - presenting a successor superseded by a retry revokes the family, the retry's token included;
  - two concurrent presentations of one unused token give one rotation and one retry, never two fresh successors;
  - a revoke during a rotation in flight leaves no live token;
  - a password reset during a login or an invitation acceptance leaves no live family.
- `m2-integrate` adds the HTTP tests above, plus a 5xx (not 401) on `/auth/refresh` when the store behind it fails.
- `docs/architecture.md` 5.1, 7, 8.1 and 8.2 point here and at ADR 0012.
