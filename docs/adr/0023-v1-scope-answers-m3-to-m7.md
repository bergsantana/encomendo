# 0023 — v1 scope answers for milestones 3 to 7

- Status: accepted
- Date: 2026-11-26

## Context

`docs/architecture.md` section 12 left four questions open, and they blocked milestones 4 and 7. The design document also listed four assumptions to confirm. Milestones 3 and 7 needed a few infrastructure choices too: photo storage, DNS for the wildcard certificate, and Google sign-in. On 2026-11-26 the user answered all of them in advance, before milestone 3 was planned.

## Decision

### Open questions (architecture section 12)

- **Payments (M4):** the owner records the deposit and the balance by hand. No payment provider and no PIX code in v1. PIX integration is a later decision.
- **Status workflows (M4):** presets only. Each store picks one of the four launch presets and cannot rename statuses or build its own list. The data model still allows both later (ADR 0004).
- **Store page SEO (M7):** the store page stays a Vite SPA. The server serving its `index.html` fills in each store's `<title>`, description and Open Graph tags (`og:title`, `og:description`, `og:image` from the cover photo or logo), so links shared on WhatsApp and Instagram show the store. No server rendering. The tags carry only public store page data (rule 7).
- **Custom domains (M7):** none in v1. A store page is only `{slug}.encomendo.com.br`.

### Product assumptions (design document)

- **Deposit:** 50% by default; each store can change it.
- **Assembly and safety margin:** 3 days and 2 days by default, editable per kit and per order.
- **Produção and Estoque:** hidden until the store turns them on in Settings.
- **Kit swap:** the owner chooses per kit. A kit either keeps its discount on a swap (kit price − removed item + new item, both at single-item price) or loses it (the swapped kit costs the sum of its items' single-item prices). In both cases the date is recalculated. A new kit keeps the discount by default, as the design document's original rule did.

### Infrastructure

- **Photos (M3):** Cloudflare R2 in production, through the S3-compatible adapter (`adapter/storage/s3.go`). Development and tests use local storage.
- **DNS and TLS (M7):** Cloudflare hosts the `encomendo.com.br` DNS. Caddy gets the `*.encomendo.com.br` certificate by DNS-01 with the Cloudflare module (ADR 0007).
- **Client sign-in (M7):** email code and Google both ship in milestone 7. Production needs a Google OAuth client ID; dev and tests verify against a fake JWKS.

## Consequences

- Milestones 4 and 7 are no longer blocked on section 12.
- The four presets ship with wording that still has to be checked with real shops before launch. That check is not a build step.
- The kit swap rule gains a per-kit setting (`catalog.Kit`), and both modes need tests. The CB 500X reference case covers the default mode.
- The Caddy image must be built with the Cloudflare DNS module, and production needs a Cloudflare API token limited to DNS edits for the zone.
- Because there are no custom domains, the storefront middleware resolves a tenant from one subdomain label under the configured base domain only.
- Payments recorded by hand mean no webhook endpoint and no provider secrets in v1.
