# Web (Vue 3 + TypeScript)

pnpm workspace with two apps that share `packages/api` and `packages/ui`. The full layout is in `docs/architecture.md` section 8.1.

- `apps/admin`: the management app for owners, admins and staff, on desktop and on phones. Every screen works at phone sizes too, because the Android app opens it (ADR 0029).
- `apps/android`: the Android shell, a Capacitor project that opens the hosted admin. It holds no UI of its own.
- `apps/store`: each tenant's store page at `{slug}.encomendo.com.br` (Kits prontos, Monte o seu, checkout, tracking), plus the client area (Minhas encomendas, also at `conta.encomendo.com.br`). Login is optional and never blocks ordering or tracking.

## Commands

```bash
pnpm install --frozen-lockfile
pnpm lint                    # eslint, whole workspace
pnpm -r typecheck            # vue-tsc --noEmit
pnpm -r test                 # vitest
pnpm --filter admin dev      # one app
pnpm api:types               # regenerate packages/api/src/types from docs/api/openapi.yaml
pnpm e2e                     # playwright against docker compose
```

The skeleton (milestone 1) defines these scripts in the root `package.json`. Keep these names so every session runs the same commands.

## Layers inside a feature

`features/<name>/views/*.vue` → `features/<name>/store.ts` (Pinia) → `packages/api/<context>.ts` → `packages/api/http.ts`

- **Views** compose components and read store state. No business logic and no API calls.
- **Components** take props and emit events. They never call `fetch` or the API package.
- **Stores** hold feature state and call the API package. Totals, deposits, ready-by dates and alerts come from the API; the client only displays them, so web and Android can never disagree.
- **`packages/api`** is the only code that talks HTTP. `http.ts` handles the base URL, JSON, problem+json → `ApiError`, and one refresh on a 401.
- DTO types come only from `packages/api/src/types/`, which is generated from the OpenAPI spec. Never edit it by hand; change the spec and regenerate.
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
- Sizes: admin 14 px body, 13 px tables, 32 px controls. Store page: mobile-first, 16–17 px body, 48 px controls. Workshop sizes (Receber itens): 18 px text, 56 px targets, a 64 px main action, through the `enc-workshop` class (`tokens.css`).
- The store page has one fixed layout. Tenants change only logo, brand color, cover photo and which sections show. Never render tenant-supplied HTML or scripts: the client login cookie is shared across `*.encomendo.com.br`.
- WhatsApp is always a `wa.me` link the API returns. Open it; don't build it in the client.
- Avoid gradients, glass effects, emoji, stock photos and donut charts.

## Phone layouts (admin)

Two layouts, one breakpoint: desktop, and phone at 640 px wide or less (design size 390 × 844). Decide layout in CSS; use script only for behaviour.

- Write `@media (max-width: 640px)` in the component, after the desktop rules. The number is `phoneMaxWidth` in `packages/ui/src/responsive.ts`; a CSS custom property cannot sit in a media query, so it is repeated.
- `tokens.css` raises `--enc-control-height` to 48 px at phone width, so `BaseButton`, `BaseInput` and `BaseSelect` follow. Links and menu entries set `min-height: var(--enc-control-height)`.
- Tables become cards with the `enc-cards` class on the `<table>`: a `data-label` on each `<td>`, and `enc-cards__actions` on the actions cell. The header row stays for screen readers. Every button keeps its accessible name.
- `BaseDialog` (dialog and drawer) is a full-screen sheet at phone width. A phone has no Escape key and no backdrop to tap, so every dialog's slot carries its own "Fechar" or "Cancelar".
- `enc-sheet-actions` (`tokens.css`) puts a sheet's "Salvar", "Cancelar" or "Fechar" in a sticky footer at phone width, so a long form never hides them. Put it on a block that is a direct child of the dialog's slot (or of the form that fills it), never nested in another scroller or wrapper with its own padding: it cancels the sheet's 16 px padding and sticks to the sheet body, whose `scroll-padding-bottom` keeps a focused field clear of it.
- `enc-workshop` (`tokens.css`) is the third size tier, for the screen used with dirty hands: put it on the screen's root and 18 px text, `--enc-control-height: 56px` and a 64 px primary `BaseButton` follow, at every width, phone included. Only Receber itens uses it; do not add it to other screens without a product reason.
- `enc-check` (`tokens.css`) is a checkbox or radio with its words as one `<label>`. Its base style (flex row, 8 px gap) applies at every width; at phone width the label is also at least 48 px tall and the box 24 px. Put the `<input>` inside the label.
- `useIsPhone()` and `useMediaQuery()` follow a media query as a ref; `trapTab()` keeps Tab inside a container. Both are for focus behaviour (the menu drawer, `BaseDialog`), never for choosing what to render.
- Never set `inert` to `false`; set it or remove it (an older WebView reads the bare attribute as inert).
- Playwright: the `phone` project runs the `e2e/phone*.spec.ts` files at 390 × 844 and checks `scrollWidth <= innerWidth`. `openMenu(page)` in `e2e/support.ts` opens the drawer when there is one.

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
