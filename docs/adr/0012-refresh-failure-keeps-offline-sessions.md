# 0012 — A failed refresh ends the session only when the server refuses it

- Status: accepted
- Date: 2026-11-24

## Context

`docs/architecture.md` 8.2 says the bearer plugin "refreshes once on 401 and logs out if refresh fails". The management app is used in workshops with weak signal, and it caches orders, the receiving checklist and the production board for offline use. If every failed refresh logged the user out, losing signal for a minute would end the session and leave the offline queue without a user.

Refresh tokens also rotate, and reusing one revokes its whole family (`docs/architecture.md` 5.1). That makes two races dangerous: two requests refreshing with the same token, and a refresh whose answer is dropped after the server has already rotated the token.

## Decision

- After a 401, the client refreshes once and retries once.
- The session ends (tokens cleared, back to login) only when the refresh call itself is refused with **400, 401 or 403**.
- Any other refresh failure keeps the tokens and fails the original request with the refresh's own error: offline or timeout (`network`), 429 or 5xx. The UI shows "no connection" rather than "session expired", and queued writes stay retryable.
- Refreshes are serialized. A request rejected with an access token that has since been replaced retries with the stored tokens and never refreshes again.
- A refresh's result is stored only if the session it started from is still there, so a logout, login or tenant switch made during the refresh wins.
- The refresh runs to completion even if the screen that triggered it goes away, so a token the server has rotated is never lost.

## Consequences

- Web follows the same status rule in its `packages/api` refresh-once-on-401, for parity.
- The 401 → refresh path in `core:network` has tests for every rule above (`BearerAuthTest`, `BearerRefreshTest`).
- A request queued under one tenant can be retried with another tenant's token after a switch. The tenant-switch and offline-queue work must cancel or tag the old tenant's requests.
