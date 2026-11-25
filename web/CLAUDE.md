# Web (Vue 3 + TypeScript)

pnpm workspace with two apps that share `packages/api` and `packages/ui`. The full layout is in `docs/architecture.md` section 8.1.

- `apps/admin`: the management app for owners, admins and staff. Same features as the Android app.
- `apps/store`: each tenant's store page at `{slug}.encomendo.com.br` (Kits prontos, Monte o seu, checkout, tracking), plus the client area (Minhas encomendas, also at `conta.encomendo.com.br`). Login is optional and never blocks ordering or tracking.

## Commands

```bash
pnpm install --frozen-lockfile
pnpm lint                    # eslint, whole workspace
pnpm -r typecheck            # vue-tsc --noEmit
pnpm -r test                 # vitest
pnpm --filter admin dev      # one app
pnpm api:types               # regenerate packages/api/types from docs/api/openapi.yaml
pnpm e2e                     # playwright against docker compose
```

The skeleton (milestone 1) defines these scripts in the root `package.json`. Keep these names so every session runs the same commands.

## Layers inside a feature

`features/<name>/views/*.vue` → `features/<name>/store.ts` (Pinia) → `packages/api/<context>.ts` → `packages/api/http.ts`

- **Views** compose components and read store state. No business logic and no API calls.
- **Components** take props and emit events. They never call `fetch` or the API package.
- **Stores** hold feature state and call the API package. Totals, deposits, ready-by dates and alerts come from the API; the client only displays them, so web and Android can never disagree.
- **`packages/api`** is the only code that talks HTTP. `http.ts` handles the base URL, JSON, problem+json → `ApiError`, and one refresh on a 401.
- DTO types come only from `packages/api/types/`, which is generated from the OpenAPI spec. Never edit it by hand; change the spec and regenerate.
- Feature folders follow the menu (`hoje`, `pedidos`, `compras`, `producao`, `estoque`, `catalogo`, `clientes`, `financeiro` …). Code inside them is English.

## TypeScript and Vue style

- `<script setup lang="ts">` and the Composition API only. No Options API, no mixins.
- `strict: true`. No `any`, no `@ts-ignore`. Use `unknown` and narrow it.
- `defineProps<{…}>()` and `defineEmits<{…}>()` with types. Give props defaults with `withDefaults`.
- Pinia setup stores (`defineStore('pedidos', () => { … })`). One store per feature; shared session state lives in `stores/session.ts`.
- Composables (`useX`) for reusable behavior: `usePermission`, `usePagination`, `useForm`. A composable returns refs and functions, never DOM.
- Names: components `PascalCase.vue`, composables `useCamelCase.ts`, stores `camelCase.ts`. Base UI components start with `Base` (`BaseButton`).
- Doc comments (`/** … */`) on exported functions, composables, stores and non-obvious props.

## Auth and permissions

- The access token lives in memory only (session store). The refresh token is an httpOnly cookie that JavaScript never reads. On load, call `/auth/refresh`.
- `usePermission('order:write')` and route `meta.permission` only hide UI. The backend is the authority, so never skip an API call because the UI decided.
- The store app talks to `/api/v1/store/*` (tenant from the Host) and, for logged-in clients, `/api/v1/me/*`. It never calls admin routes.

## Product and design rules

From `docs/product/` and the Repair Shop design system:

- All UI text is pt-BR, with the exact wording in the product docs. Niche words (Peças/Produtos, Moto/Ambiente, Montagem/Instalação) come from the tenant's template config, never from string literals in components.
- **Status labels come from the order's status workflow**: each store picks its own list. Show the label the API sends; on the store page, show the client label. Choose the badge color from the status's **phase**, never from its label or key, and never branch on a label.
- The store page and client area never show supplier, cost, margin or "Pode atrasar". Use the store types from the API; never reuse admin types there.
- Status is always **icon + word**, never color alone. Use the status tokens; never invent colors. Alerts ("Pode atrasar", "Atrasada") sit on top of the status with their own icon and word.
- Graphite does actions; color only means status. One primary button per screen.
- Safety yellow `#FFC800` only for the logo mark, selection and focus. Never for text, never for status.
- Money and quantities: ink color, bold, tabular figures. Format money from integer cents with `Intl.NumberFormat('pt-BR', { style: 'currency', currency })`. Never parse money into floats for arithmetic.
- Type: IBM Plex Sans (Condensed for dense tables), IBM Plex Mono for codes like P-0231. Icons: Tabler outline.
- Sizes: admin 14 px body, 13 px tables, 32 px controls. Store page: mobile-first, 16–17 px body, 48 px controls.
- The store page has one fixed layout. Tenants change only logo, brand color, cover photo and which sections show. Never render tenant-supplied HTML or scripts: the client login cookie is shared across `*.encomendo.com.br`.
- WhatsApp is always a `wa.me` link the API returns. Open it; don't build it in the client.
- Avoid gradients, glass effects, emoji, stock photos and donut charts.

## Accessibility

- Every interactive element is reachable by keyboard and has a visible focus ring (yellow).
- Form fields have labels. Errors are announced next to the field, in pt-BR.
- Text contrast meets WCAG AA. The status tokens were chosen for it, so don't override them.

## Testing

- Vitest + Vue Test Utils for components, stores and composables. MSW mocks the API at the network level; never mock `packages/api` functions.
- Test what the user sees: query by role and label text, not by CSS class.
- Test status badges with at least two different workflows, so nothing depends on one preset's labels.
- Playwright for the main flows: admin login, order through the workflow to handover, receiving; store page Monte o seu, checkout without login, tracking by link.
- Every bug fix starts with a failing test.
