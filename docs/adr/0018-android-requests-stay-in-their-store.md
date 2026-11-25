# 0018 — Android requests stay in the store they were made for

- Status: accepted
- Date: 2026-11-25

## Context

The access token names the store (`tid`, ADR 0003). The server trusts it: a request that carries store B's token runs in store B, whatever screen it came from.

On Android, a store switch (`/auth/switch-tenant`, ADR 0010) stores the new store's session in `TokenStore` (ADR 0015), and the API client reads the token from there for every request. The root then clears the old store's screens (`RootViewModel`), which cancels their work. Two windows remain, in which a request made for store A could run in store B:

1. **The retry.** A request sent with A's token gets a 401 while the switch holds the `SessionTurns` turn. When it gets its turn, the stored token has changed, and the client used to retry with it, that is with B's token.
2. **The send.** Between storing B's session and clearing A's screens, a screen of A can start a request, and it would carry B's token.

The web closes the same gaps with a full page load after every switch, and with a check that a call is retried only for the user and store it was sent for (`web/apps/admin/src/stores/session.ts`).

## Decision

- Every session has a **scope**: the user and the store it acts for (`Session.scope`, `{userId}@{tenantId}`; no store selected is a scope of its own). It is defined once, in `core:domain`. The API client reads it as `BearerCredentials.scope`.
- **Retries keep their scope.** The client remembers the scope of each access token it hands out, keyed by the token's signature so no usable token stays in memory. A 401 is retried only when the stored session has the same scope; otherwise the request fails with its 401. A refresh that comes back without the store (the user was removed from it) still keeps its rotated tokens, but does not retry.
- **A token goes out only to screens of its scope.**
  - The root reports the scope its screens serve (`ScreenHandover.screensReplaced`, into `SessionTurns.screensServe`). It does so each time it replaces them, after clearing the old ones.
  - Loading a token samples that scope, then reads the stored session. If the two differ, a switch is under way, and the request may come from the old store's screens. It waits until the screens change, then reads again.
  - The old screens' waiting requests are cancelled when the root clears them, so they are never sent.
  - While signed out, or before the root first reports, any token may go.
- There is no hold to release. The decision is made against the token actually read, so a stale read or a switch that stores nothing leaves nothing waiting. A request already waiting when the store goes from B back to A sees no change in the screens, so it waits at most a second before it reads again (`SessionTurns.RECHECK_MILLIS`). This matters most in Ktor's refresh path, which waits while holding the client's token lock.
- When the root itself is gone, it reports no scope.

## Consequences

- A request made from a store's screens never runs in another store, whether it is a first send or a retry. This is the client's share of tenant isolation; the server cannot tell such a request apart.
- **Rule:** bearer calls run in ViewModel scopes, which the root cancels before it serves the next scope.
  - Coroutines tied to the Compose tree (`LaunchedEffect`, `rememberCoroutineScope`, image loads) are disposed only after the handover. They must not make bearer calls; image loads go to public URLs.
  - Code under `NonCancellable` that sends a bearer request would also slip through, and needs its own check.
- **Rule:** work that outlives its screen carries the scope it was made in, and refuses to send in another. The offline write queue (ADR 0012, WorkManager) is the first such work. Today its requests would wait for the screens like any other; when it lands it must check its own scope instead.
- For the moment of the handover, the new store's first requests wait, and so does every other bearer request in the process. While the app is in the background the root does not observe the session. A switch made just before that waits until the app is back in front, which is safe: the screens' ViewModels keep running, but their requests wait.
