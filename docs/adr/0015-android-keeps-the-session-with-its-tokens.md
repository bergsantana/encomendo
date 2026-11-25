# 0015 — Android keeps the whole session with its tokens

- Status: accepted
- Date: 2026-11-25

## Context

Every login, refresh and store switch returns an `AuthSession`: the tokens plus the user, the selected store, the role, its permissions and every membership (ADR 0010). The web app keeps it in memory and refreshes on load. Android must also work after a cold start without signal (ADR 0012), when it cannot ask the API who the user is, so it has to remember the session, not just the tokens.

The permissions on screen must belong to the tokens in use. If the two were stored apart, a crash between the writes could pair new tokens with old permissions, or show a previous user's store after a new login.

## Decision

- `TokenStore` (`core:security`) seals the tokens and the session in one Keystore-encrypted blob, and every login, refresh or switch replaces both in one write.
- The atomic `replace` compares only the tokens. The refresh token is unique to one session, and a refresh must not lose its rotated tokens because something stored next to them changed.
- The blob's JSON shape (`SessionRecord`) is a storage format, pinned by a test. Fields are added with defaults; renaming one signs every device out.
- `core:security` therefore depends on `core:domain` and maps the domain session to its storage record. This is the one exception to "map at the boundary in `core:data`" (`mobile/CLAUDE.md`): the API's shape is still mapped in `core:data`.
- The user's name and email are sealed like the tokens. `User.toString()` prints only the id, so a logged session carries no personal data.

## Consequences

- After an offline cold start the app opens the right store and shows what the role allows. The cached session is at most as old as the last refresh. When the access token has expired, the first API call refreshes it; that is the same 15-minute bound the server gives role changes (ADR 0010).
- The API client is generic over what is stored with the tokens (`TokenStorage<S>`), so `core:network` knows nothing about sessions.
- Any other call that spends the refresh token (switch-tenant, a refresh on app start) must take the same turn as the client's own refresh. That lands with the first of those calls.
