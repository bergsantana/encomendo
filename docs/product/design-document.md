# Encomendo — design document

Nov 24, 2026 · @Berg

## Overview

Encomendo is a SaaS for small Brazilian businesses that sell made-to-order products: the client builds an order, the business buys or makes only what was sold, and everyone knows when it will be ready.

It started as a spin-off of the Techni Sistemas repair-shop system, designed for a custom motorcycle parts shop, and grew into a multi-niche product.

- **Who uses it:** the owner or attendant, in the web app or the Android app (both management apps with the same features; iOS is postponed), and the end client, who uses the store's page (for example altarpm.encomendo.com.br). Login is optional for clients: an account shows all their orders from every Encomendo store.
- **Problem it solves:** buying parts before the client commits ties up money, and promising a date without knowing supplier lead times leads to late deliveries and constant "when is it ready?" calls.
- **Core promise:** a delivery date calculated from the real lead time of every item, a warning before it slips, and a client page that answers the question on its own.
- **UI language:** Brazilian Portuguese. Same design system as the Repair Shop project.

## How it works

An order moves in six steps. By default nothing is bought before the client pays the deposit (sinal); the owner can change this.

1. **Cliente escolhe:** a ready kit, or builds his own ("Monte o seu") from the catalog.
2. **Pedido e sinal:** the system calculates the delivery date; the client pays the deposit.
3. **Compras geradas:** items to buy are grouped by supplier and sent through a free WhatsApp (wa.me) link.
4. **Itens chegando:** each item has an expected date; an alert fires if it passes the limit.
5. **Montagem:** starts when every item has arrived and been checked.
6. **Entrega ou retirada:** the client pays the balance; the order is delivered or picked up; photos stay in the history.

### Deadline formula

```latex
\text{Entrega} = \text{dia do sinal} + \text{item mais lento} + \text{montagem} + \text{margem de segurança}
```

- **Precisa chegar até** (arrival limit) = delivery date − assembly days. The last day an item can arrive without making the client late.
- **Pode atrasar:** the supplier's expected date is after the arrival limit.
- **Atrasada:** the expected date passed and the item has not arrived.
- Example, Kit Scrambler CB 500X: 15 days (leather seat, the slowest) + 3 assembly + 2 margin = 20 days.

### Life of an item

A comprar → Enviada ao fornecedor → Em produção / em trânsito → Recebida → Instalada. Alerts sit on top of the status, always with an icon and a word.

### System rules

- A purchase only goes out after the deposit is paid (the owner can change this).
- Every purchase line points to one order and one client (or to stock, see Product direction).
- A ready kit has a fixed price. Swapping an item: kit price − removed item + new item, both at single-item price; the kit discount stays and the date is recalculated.
- The client never sees suppliers, costs or "Pode atrasar"; the owner warns him by WhatsApp when the date changes.
- The supplier message never carries the client's name or phone.
- A client account sees its orders from every store, but a store never sees a client's orders at other stores.
- Encomendo does not replace WhatsApp conversations: it opens them with wa.me links (message prefilled) and keeps the links in the order history.

## Screens

Ten screens are drawn on the [design canvas](https://claude.ai/artifact/MvmDDtPSwjrSVhy4nzvzAa) with example data from a motorcycle shop; two more are specified in the GPT prompt but not drawn yet.

| Screen | Device | What it shows |
| --- | --- | --- |
| Fluxo e regras | Canvas board | The 6-step flow, deadline formula, item lifecycle and system rules |
| Hoje (was Painel) | Web | Summary cards, orders sorted by delivery date with items received, "Precisa de você" alerts, today's purchases |
| Pedido P-0231 | Web | Item × deadline timeline (today, arrival limit, assembly, delivery), delay banner, 3 ways to fix the late seat, values with cost and margin visible only to the owner |
| Compras | Web | Items to buy grouped by supplier, each tied to its order and client; WhatsApp message preview; supplier's promised date |
| Catálogo | Web | Items with supplier, lead time, cost, price, margin, "No portal" switch; drawer with client options and a backup supplier |
| Editar kit | Web | Kit items, which ones the client may swap, kit price vs single-item sum, margin, calculated lead time |
| Cliente: escolher | Phone | Client's motorcycle, "Kit pronto" or "Monte o seu", available kits |
| Cliente: Monte o seu | Phone, interactive | Category chips, add/remove items; total, deposit and ready-by date update live |
| Cliente: acompanhar | Phone | Delivery forecast, steps, items in plain words, payment summary, WhatsApp button |
| Dono: receber itens | Android (workshop) | Checklist of what arrived, photos of item and invoice, "Tudo certo / Com defeito", tells when an order is complete |
| Produção | Web, not drawn | Kanban for in-house items with weekly capacity |
| Cliente: minhas encomendas | Phone, not drawn | Login (optional, email code or Google), then every order from every Encomendo store with status and forecast |
| Construtor do portal | Web, not drawn | Template choice, logo, brand color, cover photo, sections on/off, live phone preview |

## Product direction

Encomendo serves many niches: each business builds its client portal from a niche template and can buy to order, keep stock, or make items in-house.

### Sourcing chosen per item

One order can mix all three (a furniture shop makes cabinets, keeps hinges in stock and orders glass), so the sourcing mode belongs to each catalog item, not to the business.

| Mode | When the item is ready | What it adds |
| --- | --- | --- |
| Sob encomenda | Supplier lead time | Purchase line tied to the order |
| Estoque | Immediately, if enough is on hand (reserved for the order); otherwise supplier lead time + "sem estoque" warning | Reservations, minimum-quantity alert, restock purchases tied to "estoque" |
| Produção própria | Production time + wait for free capacity | Production board (kanban) and a weekly capacity number |

The deadline formula stays the same: delivery = latest ready date among the order's items + assembly + safety margin.

### Templates and portal builder

- A template is a niche preset: categories, item fields ("Serve em": motorcycle model, roof type, room), default sourcing per category, sample kits and wording. Configuration, not separate code.
- Each store page lives at its own subdomain (for example altarpm.encomendo.com.br) and is the only thing the client sees.
- The portal builder stays limited: logo, brand color, cover photo, and sections on or off (Kits prontos, Monte o seu, Acompanhar pedido). Status colors, contrast and layout stay fixed so every portal stays readable and there is one front end to support.

## Naming

The product is called **Encomendo** ("I order"): specific to what it does, not tied to a niche, and as a made-up word more likely to be registrable. INPI (classes 9 and 42), the .com.br domain and the Instagram handle still need checking.

**Slogan:** Venda primeiro. Compre o necessário. Entregue com confiança.

Alternatives considered: No Prazo, Pronto Quando, Sob Medida (too common), Monte, Techni Encomendas (family brand, cheaper if starting with automotive niches only).

### Menu

Hoje · Pedidos · Compras · Produção · Estoque · Catálogo · Clientes · Financeiro. Produção and Estoque appear only when the business turns them on.

| Before (canvas) | After | Why |
| --- | --- | --- |
| Painel | Hoje | It is the "what needs me today" screen |
| Fornecedores (menu item) | Tab inside Compras | Both opened the same screen |
| Catálogo + Kits | Catálogo, tabs Itens and Kits | A kit is a bundle of catalog items |
| Chegada limite | Precisa chegar até | Says what it means |
| Em risco | Pode atrasar | Clear next to "Atrasada" |
| Folga | Margem de segurança | "Folga" can read as a day off |
| Marcar como pedido | Marcar como enviada ao fornecedor | "Pedido" meant both client and supplier orders |
| Montar meu kit | Monte o seu | Shorter, works in any niche |

### Words set by the template

- **Peças:** Produtos, Itens, Módulos (furniture), Componentes (PCs)
- **Moto (what it fits):** Ambiente, Telhado, Plataforma
- **Montagem:** Instalação, Produção, Entrega

## Visual identity

Encomendo reuses the Repair Shop [design system](https://claude.ai/artifact/LbV8eABEAJLXfSWEGodUSw): calm, "a well-kept workshop", where graphite does actions and color only means status.

- **Ground and cards:** warm off-white #F5F4F0, white cards with a hairline border; radius 8 px on cards and buttons, 4 px on badges (stamped tags, not pills).
- **Text:** ink #1B1D20, secondary #545860. Money and quantities always in ink, bold, tabular figures.
- **Primary button:** graphite #1B1D20, one per screen. Secondary: white with a #7D796F border.
- **Accent:** safety yellow #FFC800 only for the logo mark, current selection and focus; never text, never a status.
- **Status (always icon + word):**

| Status | Fill | Text |
| --- | --- | --- |
| A comprar / Planejado | #E5E9EE | #36414D |
| Aguardando fornecedor / Em produção | #FDE9D6 | #7A3300 |
| Em montagem | #E1EDF9 | #0A4478 |
| Pronto / Recebida | #DFF1E3 | #145226 |
| Pode atrasar / Atrasada | #FBE3E3 | #981418 |
| Entregue | #E5EBE7 | #36423A |

- **Type:** IBM Plex Sans (Condensed for dense tables), IBM Plex Mono for codes such as P-0231. Icons: Tabler outline.
- **Sizes:** web app 14 px body, 13 px tables, 32 px controls; client portal 16–17 px body, 48 px controls; workshop phone 18 px body, 56 px targets, 64 px primary action.
- **Logo direction:** yellow rounded square with a package-and-check icon, then "Encomendo" in bold ink.
- **Avoid:** gradients, glass effects, purple AI look, emoji, stock photos, donut charts.

## Market

The back-office half already exists in manufacturing ERPs; what was not found anywhere is a client-facing kit builder with a live delivery date, niche templates and WhatsApp supplier orders, at a small-shop price.

### Target niches

- **Strongest fits:** solar installers, custom PC builders, planned furniture (marcenaria), car customization (sound, lighting, wheels), boat outfitting (relevant in Amazonas).
- **Good fits:** opticians (frame + lab lenses), bathroom and kitchen remodels, custom bicycles and e-bikes, uniforms and promotional merchandise.
- **With stock and in-house production on:** bakeries and cake shops, print shops, sign makers, furniture shops with their own workshop, tailors, 3D printing, auto parts stores taking special orders.
- **Also in scope:** motorcycle customization, automotive accessories, jewelry, personalized gifts, party decoration kits, clothing, small manufacturers.
- **Worked backwards:** events and weddings (fixed date → last day to order each item).
- Solar and furniture look like the largest markets in Brazil.

### Competitors

| Product | What it covers | What it lacks for this market |
| --- | --- | --- |
| [Odoo](https://www.odoo.com/documentation/14.0/applications/inventory_and_mrp/inventory/management/products/mto.html) | Purchase per sale (MTO), stock-then-buy routes, customer portal | Large ERP set up per company, usually with an implementation partner |
| [MRPeasy](https://www.mrpeasy.com/resources/user-manual/one/) | Make-to-order and make-to-stock, earliest delivery date, B2B portal with statuses | Built for factories (10–200 staff) selling to businesses, not consumers building a kit |
| [Katana](https://support.katanamrp.com/en/articles/5908448-workflow-basics) | Make-to-order and to-stock, purchases linked to sales | Supplier orders exported as PDF; no consumer configurator |
| Bling, [Omie](https://www.omie.com.br/blog/sistema-para-marcenaria-nao-perca-margem-no-orcamento/) | Stock, NF-e, marketplaces; Omie markets to marcenarias | No client kit builder or supplier-delay alerts found |

### Differentiators

- The end client sees price and ready-by date change while building the kit.
- Niche templates ready on day one instead of configuring an ERP.
- "Pode atrasar" explained in plain words, with ways to fix it.
- Supplier orders by free wa.me link.
- Monthly price a small shop can pay, no implementation project.

**Biggest risk:** an Odoo partner could build a niche version; the defense is being ready to use, in Portuguese and cheap.

This was a quick scan of the main players, not a full market study.

## Open questions and next steps

### Assumptions to confirm

- Deposit is 50% by default.
- Assembly 3 days and safety margin 2 days by default, editable per kit and per order.
- Swapping an item inside a kit keeps the kit discount.
- The client never sees "Pode atrasar"; the owner warns by WhatsApp.

### Next steps

- [ ] Check INPI, .com.br domain and Instagram handle for "Encomendo"
- [ ] Apply the new menu and wording to the canvas
- [ ] Draw Produção, Estoque and the portal builder; add the sourcing field to Catálogo
- [ ] Draw client login and Minhas encomendas
- [ ] Generate screens with the GPT prompt (saved as encomendo-design-prompt.md in the Repair Shop project)
- [ ] Pick the launch niche

### Links

- [Design canvas, 10 screens](https://claude.ai/artifact/MvmDDtPSwjrSVhy4nzvzAa)
- [Repair Shop design system](https://claude.ai/artifact/LbV8eABEAJLXfSWEGodUSw)

### Sources (competitors)

- [Odoo — Replenish on order (MTO)](https://www.odoo.com/documentation/14.0/applications/inventory_and_mrp/inventory/management/products/mto.html)
- [Odoo — MTO and dropshipping in one order](https://www.odoo.com/forum/help-1/using-mto-and-dropshipping-in-a-single-sales-order-in-odoo-276862)
- [MRPeasy — Getting started](https://www.mrpeasy.com/resources/user-manual/one/)
- [The Fabricator — MRPeasy B2B portal](https://www.thefabricator.com/thefabricator/product/cadcamsoftware/customer-portal-integrates-web-store-into-mrp-software)
- [Katana — Workflow basics](https://support.katanamrp.com/en/articles/5908448-workflow-basics)
- [Omie — Sistema para marcenaria](https://www.omie.com.br/blog/sistema-para-marcenaria-nao-perca-margem-no-orcamento/)
