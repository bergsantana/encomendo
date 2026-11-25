# 0016 — Android logout clears the device first

- Status: accepted
- Date: 2026-11-25

## Context

`POST /auth/logout` revokes the login's refresh family, puts the access token on the denylist when one is sent, and always answers 204 (ADR 0010). Android names its family with `refreshToken` in the body (`LogoutRequest`, Android only); the web's travels in its cookie.

The web app keeps its refresh token in an httpOnly cookie that only the server can clear. So when the server cannot be reached, the web stays signed in and says so (`web/apps/admin/src/features/auth/store.ts`).

Android holds its tokens itself, in `TokenStore` (ADR 0015). It is used in workshops with weak signal (ADR 0012), and phones are handed from one person to another.

## Decision

- Sair first clears `TokenStore`, so the app shows the login at once, with or without signal.
  - The clear is a compare-and-swap, so the tokens sent to the server are exactly the ones cleared, even when a refresh lands in between.
  - It runs to the end even when the screen that asked is cancelled. The root clears the signed-in screens as soon as it sees the session end, which can be before the clear returns.
- Then, in a scope that outlives the screen, the app calls `/auth/logout`:
  - `{"refreshToken": …}` as the body, and the access token as the bearer, both from the session just cleared;
  - API failures (offline, refused, a server error) are ignored.
- That call never uses the client's stored token or its refresh. A revoke still on the wire when the next person logs in must not carry, or retry with, their token.
- If the device cannot clear its session (a full disk), the logout fails and tells the user, and the server is not called, so the session on screen still works.

## Consequences

- Offline, logging out still works on the device. The family then lives until its refresh token expires (30 days) or the next password reset, but no device holds that token any more.
  - The revoke is not queued for later with WorkManager. That would keep the refresh token outside `TokenStore`, which `mobile/CLAUDE.md` forbids.
- Online, the family is revoked and the access token stops at once.
- A refresh in flight may already have rotated the token that logout presents. So the backend must revoke the family from any token of it, used or not, and not answer such a logout as reuse or as a retry (ADR 0013). This is noted for `m2-domain` and `m2-integrate`, which build logout.
- This differs from the web, which cannot clear its own cookie. The wording on screen is the same ("Sair", "Não foi possível sair.").
