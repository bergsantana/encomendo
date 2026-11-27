# 0021 — Client IP and rate limits

- Status: accepted
- Date: 2026-11-26

## Context

The contract limits nine staff calls (`docs/api/openapi.yaml`): sign-up, login, refresh, both password-reset calls, the invitation preview and acceptance, creating a store, and inviting a member. ADRs 0010 and 0011 say what each limit is keyed on, per IP, per email, per user or per store, and ADR 0013 says every 429 carries `Retry-After`. Only the daily caps on stores and invitations have numbers. Nothing says:

- how the API learns the client's IP behind its proxies;
- the numbers for the other limits;
- where each kind of limit runs, and what a limit does when Redis fails;
- how the keys look, and whether an email may sit in Redis.

Trusting the wrong `X-Forwarded-For` value would let anyone choose their own IP and skip every per-IP limit.

## Decision

### The client IP

The API is reached only through proxies:

- in production, the admin host goes Caddy → nginx → api, and the API host goes Caddy → api;
- in the CI and local stack, the admin host goes nginx → api.

The production API publishes no port. Caddy (2.5 and later) replaces an incoming `X-Forwarded-For` with the client's address when the peer is not one of its own trusted proxies. nginx appends its peer with `$proxy_add_x_forwarded_for`.

The rule:

1. Take the peer from `RemoteAddr`. If it is not a trusted proxy, it is the client, and `X-Forwarded-For` is ignored.
2. Otherwise, walk `X-Forwarded-For` from the right, every header line in order, skipping trusted proxies. The first address that is not trusted is the client.
3. If the walk finds no untrusted address, or meets an entry that is not an IP, the client is the last trusted hop it passed. A proxy never writes a malformed entry, so a malformed entry comes from outside, and stopping there keeps it from choosing the key.
4. The leftmost value is never taken on its own: the client writes it.

The trusted proxies are a list of CIDRs in the configuration (step 2.6). The rule is sound only when every trusted address really is a proxy: a trusted address that is not one, such as Docker's NAT gateway in front of a published port, passes on whatever the client wrote.

- **Production names its proxies exactly.** The compose network gets a fixed subnet, Caddy and nginx get fixed addresses in it, and the list holds those two addresses. The configuration refuses an empty list outside development (step 2.6).
- **Development and test default to the private and loopback ranges:** `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `127.0.0.0/8`, `fc00::/7` and `::1/128`. The local and CI stacks publish the API's port, and its Docker gateway falls in these ranges, so a client there can pick its own key. That is accepted for those stacks only. End-to-end tests must not rely on per-IP limits keeping clients apart.
- **IPv6 in production.** Behind Docker's userland proxy, IPv6 connections reach Caddy from the bridge gateway, and every IPv6 client would share that one key. A third party could then lock out all of them. So the production network enables IPv6, or Caddy uses the host's network (step 2.6). In production the API logs a warning, once, when the client it finds has a private, loopback or link-local address, which no real client has. So a collapse like this shows in the logs.

IPv4-mapped IPv6 addresses count as IPv4. An IPv6 client is limited by its `/64`, because one subscriber usually holds a whole `/64` and could otherwise pick a new address for every attempt. A holder of a larger block, such as a `/56` or a `/48`, still gets one count per `/64`. That is accepted: the per-email limits stay tight whatever the address.

### The limits

Brazilian mobile carriers put many customers behind one address (CGNAT), so per-IP limits are loose and the per-email limits are the tight ones. Windows slide: "per day" means the last 24 hours.

| Rule | Keyed on | Limit | Window |
| --- | --- | --- | --- |
| `signup` | IP | 10 | 1 hour |
| `signup` | email | 5 | 1 hour |
| `login` | IP | 60 | 10 minutes |
| `login` | email | 10 | 10 minutes |
| `refresh` | IP | 300 | 10 minutes |
| `password-reset` | IP | 10 | 1 hour |
| `password-reset` | email | 5 | 1 hour |
| `password-reset-confirm` | IP | 30 | 10 minutes |
| `password-reset-confirm` | email | 10 | 10 minutes |
| `invitation-preview` | IP | 60 | 10 minutes |
| `invitation-accept` | IP | 20 | 10 minutes |
| `create-tenant` | user | 10 | 24 hours |
| `invite` | store | 20 | 24 hours |
| `invite` | inviting user | 50 | 24 hours |
| `invite` | store and email | 3 | 24 hours |
| `photo-upload` | user | 30 | 1 minute |
| `photo-upload` | store | 60 | 1 minute |

- Sign-up is limited per email as well as per IP, as ADR 0010 says; `openapi.yaml` gets that line in step 2.3.
- A per-email login limit lets someone keep a known address out of login for as long as they keep sending about one attempt a minute, because the window slides. Only new logins are blocked: a signed-in user stays signed in, since refresh is not limited per email. That is accepted. The alternative, per-IP limits alone, lets a CGNAT pool try passwords freely.
- *Amended in step 2.4:*
  - `create-tenant` counts attempts, a taken address included. The handler cannot know an address is taken before trying, and the sliding log cannot give a count back. So the cap is 10 attempts a day, not 5 stores, and a few collisions do not lock a new owner out.
  - The invitation caps count only calls that could send mail: from a caller whose token may manage the team (`members:manage`), with an email and a role the use case accepts. Otherwise a viewer's refused calls, or a typo, would spend the store's budget.
  - Accepted: a member removed or demoted from admin keeps a token that says `admin` for up to 15 minutes (ADR 0011), and in that time could spend the store's 20 invitations. The use case refuses each call, so nothing is sent. Retries of "Reenviar" during an email outage count too.
- *Amended in step 3.10:* `POST /api/v1/admin/catalog/photo-uploads` is limited per user and per store. Each call hands out a signed upload URL that works for 5 minutes and takes up to 5 GB, and the bucket is billed, so unlimited requests from one staff member would inflate the bill. The numbers allow a normal session (an item has at most 8 photos, a bulk import a few dozen) and stop a loop. Like the invitation caps, the handler counts only calls from a caller who may write the catalog (`catalog:write`), so a viewer's refused calls never spend the store's budget; the user's count runs first, so a refused user does not touch the store's. Redis down lets the call through, as for every rule above.
- `switchTenant` and logout have no limit. Switching rotates a refresh token the caller already holds, and logout must always work.

### Where limits run

- **Per-IP limits** run in a middleware on each route, before the body is read.
- **Per-email, per-user and per-store limits** run in the handler, after decoding (email) or after authn (user, store), through a small interface next to the handlers. Steps 2.3 and 2.4 add them.
- **Counting:** a sliding log in a Redis sorted set, one Lua script per attempt (`adapter/redis/ratelimit.lua`). It drops entries older than the window, counts the rest, and adds the attempt only when it is under the limit, so refused attempts never extend a block. When it refuses, it returns the time until the oldest entry leaves the window, which becomes `Retry-After` in whole seconds, at least 1. The time comes from the API's clock, not Redis's, so tests run on a fixed clock. The API instances' clocks differ by milliseconds, and that is accepted.
- One request can meet two limits (per IP, then per email). The first one counts even when the second refuses. That is accepted.

### When Redis fails

A limit that cannot be checked lets the request through and logs an error, as the cache does (`docs/architecture.md` section 4).

- Each count gives up after 250 ms, so a Redis that hangs instead of refusing does not hold limited requests until the `/auth` deadline. The Redis client honours every context's deadline, which go-redis does only when told (`ContextTimeoutEnabled`).
- While Redis is down, password guessing is not limited. argon2id's cap of four hashes at once bounds the server's CPU, not the guesses one account takes (ADR 0019). Password-reset codes keep their own counters in Postgres (ADR 0011). That is accepted for an outage, and the error log is what an alert watches.
- The denylist is the exception: it fails closed (ADR 0013). Its reads are bounded too, so a hung Redis gives quick 503s.

### Keys

Keys are global, because a limit is checked before or without a tenant: `g:rl:{rule}:{kind}:{value}`. For example, `g:rl:login:ip:` followed by the 64 hex digits of the address's HMAC.

- Values are stored as the HMAC-SHA256 of the kind and the value, in hex. So neither an email nor a client's address sits in Redis in clear, and no list of addresses can reverse them without the key.
  - An email uses its domain key, trimmed and lower-cased (`identity.Email.Key`, ADR 0010).
  - An address is the client IP rule's value: the address, or the `/64` for IPv6.
  - A key under 32 bytes is refused, because an empty key would make the HMAC a plain hash.
- The key is derived from the reset-code HMAC key (ADR 0011) as `HMAC-SHA256(reset key, "encomendo rate-limit email v1")`, so it needs no new secret and the two uses stay apart. Step 2.6 does the derivation.
- A store-and-email key joins the store id and the email's HMAC.

## Consequences

- `adapter/redis/ratelimiter.go` implements the count and is tested against a real Redis: the window's edges, `Retry-After`, and Redis stopped.
- `adapter/http/ratelimit` holds the rules above, the keys, the pseudonyms and the client IP rule. `middleware/ratelimit.go` applies a per-IP rule, and on `/auth` routes it runs after the cross-origin check, so a cross-site page cannot spend its visitors' budgets (step 2.3).
- Step 2.6 adds the trusted proxies to the configuration, with exact addresses in production and the private ranges in development, and derives the pseudonym key.
- With exact proxy addresses, the rule holds even if Caddy ever stops replacing `X-Forwarded-For`: the address Caddy appends is the first untrusted one from the right. With the private-range default it holds only for clients outside those ranges, which is why production names its proxies.
