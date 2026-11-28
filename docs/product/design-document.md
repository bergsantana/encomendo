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
- A ready kit has a fixed price. On each kit the owner chooses what a swap does: keep the discount (kit price − removed item + new item, both at single-item price; the default) or lose it (the sum of single-item prices). Either way the date is recalculated.
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

### Roles

A store's team (Equipe) has four roles (ADR 0017). Each person has one role per store.

| Role (code) | Shown as | Can do (docs/architecture.md 5.4) |
| --- | --- | --- |
| `owner` | Dono | Everything, finance, billing, the status workflow; one per store |
| `admin` | Admin | Catalog, suppliers, purchasing, orders, stock, production, portal, the team except the owner |
| `staff` | Funcionário | Orders, receiving, production stages; reads catalog and stock |
| `viewer` | Visualizador | Reads only, without costs or margins |

### Account and store screens (staff)

The exact wording of the screens that start, move and end a staff session. Web and Android use the same text. `{…}` is filled in by the app.

**Messages any of these screens can show**

| When | Text |
| --- | --- |
| A field to fix, empty or refused by the server (above the form) | Confira os campos destacados. |
| No connection | Sem conexão com o servidor. Confira a internet e tente de novo. |
| Server failure (5xx) | O servidor não respondeu. Tente de novo em instantes. |
| Too many attempts (429) | Muitas tentativas. Tente de novo em {wait}. |
| Any other refusal | The pt-BR `title` the API sent. |
| A password rule (hint under the field) | Pelo menos 8 caracteres. |

- `{wait}` comes from `Retry-After`: under 60 seconds it is "1 segundo" or "N segundos"; from 60 on it is minutes, rounded up, "1 minuto" or "N minutos". Without the header it is "1 minuto".
- When the session could not be restored on load (offline or a server failure), the connection or server message shows above the form on every screen below.

**Entrar (login)**

- Title and button: "Entrar". Fields: "E-mail", "Senha". Links: "Esqueci a senha", "Criar conta".
- Empty fields: "Informe seu e-mail.", "Informe sua senha."
- Wrong email or password: "E-mail ou senha incorretos."
- After a password reset: "Senha alterada. Entre com a nova senha."
- When the session could not be restored on load: the connection, server or rate-limit message.
  - Except after a rate limit, a button "Tentar de novo" asks for the session again (logging in again would start a new login). While it waits: "Tentando de novo…".
  - The page opens once the server answers with the session. If the server answers that there is none, the message goes and the login form stays.

**Criar conta (sign-up)**

- Title and button: "Criar conta". Fields: "Nome", "E-mail", "Senha" (hint "Pelo menos 8 caracteres."). Link: "Já tenho conta".
- Empty fields: "Informe seu nome.", "Informe seu e-mail.", "Informe uma senha."
- Email in use, next to the field: "Já existe uma conta com este e-mail."

**Lojas (store picker)**

- With stores: title "Escolha a loja", one button per store with its link `{slug}.encomendo.com.br` beside it, then "Criar outra loja".
- A suspended store (the platform suspends a store; decided by the user on 2026-11-26, ADR 0022):
  - stays in the list with "Suspensa" beside it, and cannot be entered;
  - choosing it shows "Esta loja está suspensa. Fale com o suporte do Encomendo." and stays on the picker;
  - when a screen inside the store learns that the store was suspended, it shows the same message and goes back to the picker.
- Without stores: title "Crie sua loja".
- Create form: fields "Nome da loja" and "Link da loja", button "Criar loja"; "Sair" below.
  - Link hint: "Use de 3 a 40 letras minúsculas, números e hífens." until the link is valid, then "Sua loja ficará em {link}.encomendo.com.br."
  - Empty fields: "Informe o nome da loja.", "Informe o link da loja."
  - Link in use, next to the field: "Este link já está em uso. Escolha outro."
- A store the member no longer belongs to: "Você não faz mais parte desta loja."
- In the menu: "Trocar de loja", only for members of two or more stores.

**Esqueci a senha**

- Title "Esqueci a senha". First step: "Informe o e-mail da sua conta. Enviaremos um código para você criar uma nova senha.", field "E-mail", button "Enviar código". Empty field: "Informe seu e-mail."
- Second step, the same whether or not the account exists: "Se houver uma conta com {e-mail}, enviamos um código de 6 dígitos. Ele vale por 10 minutos."
  - Fields "Código" and "Nova senha" (hint "Pelo menos 8 caracteres."), button "Alterar senha".
  - Buttons "Enviar outro código" (then "Se houver uma conta com {e-mail}, enviamos um novo código. O anterior deixou de valer.") and "Usar outro e-mail".
  - "Informe os 6 dígitos do código.", "Informe a nova senha.", and next to the code: "Código errado ou vencido. Confira o e-mail ou peça um novo código."
- Link: "Voltar para o login".

**Convite (accepting an invitation)**

- While loading: "Verificando o convite…"
- Title: "Entrar na equipe da loja {loja}".
  - New account: "Convite para {e-mail}. Crie sua conta para aceitar.", fields "Nome" and "Senha" (hint "Pelo menos 8 caracteres."). Empty fields: "Informe seu nome.", "Informe uma senha."
  - Existing account: "Você já tem uma conta com {e-mail}. Entre com a sua senha para aceitar.", field "Senha", link "Esqueci a senha". Empty field: "Informe sua senha."
  - Button "Aceitar convite". Wrong password, next to the field: "Senha incorreta." Already in the store: "Você já faz parte desta loja."
- An invitation that no longer works, also when it runs out between opening and accepting: title "Convite indisponível", "Este convite não vale mais. Peça um novo a quem convidou você."
- A link without its token: title "Abra o convite pelo e-mail", "Este endereço não traz o convite. Abra de novo o link que chegou no seu e-mail."
- When the invitation cannot be read for another reason (offline, 5xx, 429): title "Convite", the matching message from the table above, button "Tentar de novo".
- Link: "Entrar".

**Menu**

- The store's name under the logo; the member's name and "Sair" at the foot.
- On a phone (640 px wide or less) the menu is a drawer: a top bar shows the button "Menu" and the store's name, and the drawer holds everything above, plus a button "Fechar menu". Choosing an entry closes it. *Drafted by Claude on 2026-11-27 for milestone 3.5, for the user to review: the words "Menu" and "Fechar menu".*
- When the server cannot be reached: "Não foi possível sair. {the connection or server message}"

### The Android app

The Android app opens the web admin, so every screen and word above is the web's (ADR 0029). The app shows only two pages of its own:

- **Offline:** when the admin cannot be loaded, "Sem conexão com o servidor. Confira a internet e tente de novo." and the button "Tentar de novo", which opens the admin again.
- **Old WebView:** when the phone's Android System WebView is too old to open the admin safely, "Para abrir o Encomendo, atualize o {WebView} deste aparelho na Play Store e abra o app de novo." and the button "Atualizar na Play Store", which opens that app's Play Store page. `{WebView}` is the name the phone gives it, usually "Android System WebView". *Drafted by Claude on 2026-11-27 in step 3.5.2, for the user to review.*

### Equipe (the team)

Only for roles with `members:manage` (Dono, Admin): the menu entry "Equipe" is hidden from the others. Role names are in "Roles" above.

- **Title** "Equipe". Table "Nome", "E-mail", "Papel", and an "Ações" column read only by screen readers. The signed-in member's row says "(você)" after the name.
  - The owner's row shows "Dono" and nothing to change: the owner's role never changes and the owner is never removed.
  - Other rows: the role as a choice of Admin, Funcionário, Visualizador (read aloud as "Papel de {nome}"), and "Remover".
  - Picking a role only drafts it. "Alterar papel" appears once it differs, and saves it. On the member's own row it asks first: "Você deixará de gerenciar a equipe.", buttons "Cancelar" and "Confirmar".
  - "Remover" asks first: "Remover {nome} da equipe?" (own row: "Você sairá da equipe e perderá o acesso a esta loja."), buttons "Cancelar" and "Confirmar remoção".
  - Screen readers hear each row's buttons with the person: "Alterar papel de {nome}", "Remover {nome}".
  - "Carregar mais" when the team runs past one page. While loading: "Carregando a equipe…"
- **After a change:** "{nome} agora é {papel}.", "{nome} saiu da equipe."
- **When a change fails:** the message shows in that row, such as "Você não tem mais permissão para gerenciar a equipe.", after which the screen leaves for Hoje once the session catches up, or a message from the table above. "Esta pessoa não faz mais parte da equipe." shows at the top instead, since that row goes when the list is read again. If the team could not be read at all: the message and "Tentar de novo".
- "Cancelar" puts things back as they were: a role question restores the role the member has.
- **Invite form** "Convidar para a equipe": fields "E-mail" and "Papel" (Funcionário unless another is chosen), button "Enviar convite".
  - "Informe o e-mail." Already on the team, next to the email: "Esta pessoa já faz parte da equipe."
  - Sent: "Convite enviado para {e-mail}." Saved but the email did not go out: 'O convite foi salvo, mas o e-mail não saiu. Use "Reenviar" para tentar de novo.'
- **"Convites pendentes"**: table "E-mail", "Papel", "Vale até" (dd/mm/aaaa), buttons "Reenviar" and "Cancelar convite" (read aloud with the email: "Reenviar convite para {e-mail}", "Cancelar convite para {e-mail}"); "Carregar mais convites".
  - After them: "Convite reenviado para {e-mail}.", "Convite para {e-mail} cancelado."
  - "Reenviar" whose email did not go out, in the row: 'O convite foi salvo, mas o e-mail não saiu. Use "Reenviar" para tentar de novo.' (the list is read again for the new expiry).
  - At the top, with the lists read again: "Este convite não está mais pendente.", or "Esta pessoa já faz parte da equipe." when the person joined meanwhile.

### Catálogo

Drafted by Claude on 2026-11-26 for milestone 3, for the user to review (ADR 0025). Web and Android use the same text. The messages in "Account and store screens" above (no connection, server failure, rate limit, another refusal) apply here too. `{Montagem}` is a niche word from the template, "Montagem" until templates exist.

**Who sees what**

- Every role sees the menu entry "Catálogo" (`catalog:read`).
- Only Dono and Admin (`catalog:write`) see buttons that change anything. The other roles see the same screens read-only.
- "Custo" and "Margem" show only to the Dono, and only because the API sends them. The other roles see neither the column nor the field.

**Tabs:** "Itens" and "Kits".

**Itens**

- Table: "Item", "Categoria", "Origem", "Fornecedor", "Prazo", "Custo", "Preço", "Margem", "No portal".
  - "Origem" is "Sob encomenda", "Estoque" or "Produção própria".
  - "Prazo" is "{n} dias" ("1 dia").
  - "Margem" is "{valor} ({n}%)", or "—" without a cost.
  - "No portal" is a switch, read aloud as "Mostrar {item} na página da loja".
- Above the table: the search field "Buscar item", the filter "Todas as categorias", the box "Mostrar arquivados", and the buttons "Novo item", "Categorias" and "Campos".
- While loading: "Carregando o catálogo…". At the end of a page: "Carregar mais".
- On the phone each row is a card with the same columns, one labelled line each (Categoria, Origem, Fornecedor, Prazo, Preço, and for the Dono Custo and Margem), then the "No portal" switch. Tapping a card opens the item.
- On Android a role that only reads opens the item as its details in labelled lines (name, category, origin, price, suppliers, fields, options) with the title "Item" and one "Fechar" button, not as disabled fields.
- Nothing is kept on the phone (ADR 0029): without a connection the catalog shows "Sem conexão com o servidor. Confira a internet e tente de novo.", as on the web.
- Empty catalog: "Nenhum item no catálogo ainda." with "Novo item". A search with no results: "Nenhum item encontrado."
- An archived row shows "Arquivado" beside the name.

**Item form** (drawer on the web, a full screen on the phone): title "Novo item" or "Editar item". While the item is read: "Carregando o item…".

- **Fields:**
  - "Nome";
  - "Descrição" (optional);
  - "Categoria" ("Sem categoria" unless one is chosen);
  - "Origem";
  - "Preço".
  - "Estoque" and "Produção própria" appear in "Origem" only when the store turned them on.
- **Sob encomenda and Estoque**, section "Fornecedor":
  - "Fornecedor principal", a choice of the store's suppliers plus "Novo fornecedor";
  - "Prazo do fornecedor (dias)";
  - "Custo" (Dono only).
  - "Adicionar fornecedor reserva" opens the same three fields under "Fornecedor reserva", with "Remover fornecedor reserva".
- **Produção própria:** "Dias de produção" and "Custo de produção" (Dono only).
- **"Novo fornecedor":**
  - fields "Nome do fornecedor" and "WhatsApp do fornecedor", with the hint "Com DDD, como (11) 98765-4321.";
  - button "Adicionar fornecedor";
  - empty fields: "Informe o nome do fornecedor.", "Informe o WhatsApp do fornecedor.";
  - an invalid number: "Confira o número: use DDD e 8 ou 9 dígitos.";
  - a name over 120 characters: "Use um nome de até 120 caracteres.";
  - a name with line breaks or other control characters: "Use um nome sem quebras de linha ou caracteres de controle.";
  - a name already in the store: "Já existe um fornecedor com este nome."
- **Campos:** one choice per store field, labelled with the field's name (such as "Serve em"). Several values may be picked.
- **"Opções do cliente":**
  - each option has "Nome da opção", the box "Obrigatória" and its choices;
  - each choice has "Escolha", "Acréscimo no preço" and "Dias a mais";
  - buttons "Nova opção", "Adicionar escolha", "Remover escolha", "Remover opção";
  - hint: "O cliente escolhe uma destas opções ao pedir. Dias a mais entram no prazo."
- **"Fotos":**
  - button "Adicionar fotos", hint "JPEG, PNG ou WebP, até 10 MB. A primeira é a capa.";
  - on the phone, "Adicionar fotos" asks "Da galeria" or "Tirar foto";
  - each photo has "Usar como capa" and "Remover foto"; the first shows "Capa" in place of "Usar como capa";
  - photos change at once, not with "Salvar", and belong to a saved item.
- **"Mostrar na página da loja"** (the "No portal" switch).
- **Buttons:** "Salvar" and "Cancelar". When editing, also "Arquivar", or "Restaurar" for an archived item.
- **Empty or wrong fields**, with "Confira os campos destacados." above the form:
  - "Informe o nome do item.", "Informe o preço.";
  - "Escolha o fornecedor principal.", "Informe o prazo do fornecedor.", "Use um prazo de 0 a 365 dias.", "Informe um custo de zero ou mais.", "O fornecedor reserva precisa ser outro.";
  - "Informe os dias de produção.";
  - "Informe o nome da opção.", "Adicione ao menos uma escolha.", "Já existe uma escolha com este nome."
  - Also: "Use um nome de até 120 caracteres.", "Use um nome sem quebras de linha ou caracteres de controle.", "Use uma descrição de até 2000 caracteres.", "Use um valor de até R$ 1.000.000.000,00.", "Escolha a origem do item.", "Escolha o fornecedor reserva.", "Produção própria não usa fornecedor.", "Use de 1 a 365 dias de produção.", "Dias e custo de produção só valem para produção própria.", "Escolha até 200 valores.", "Cada valor só pode ser escolhido uma vez.", "Use até 20 opções.";
  - options: "Use um nome de até 80 caracteres.", "Use até 50 escolhas.", "Informe a escolha.", "Use uma escolha de até 80 caracteres.", "Use uma escolha sem quebras de linha ou caracteres de controle.", "Informe um acréscimo de zero ou mais.", "Use de 0 a 365 dias a mais.";
  - photos: "Liste todas as fotos, cada uma uma vez." (the order sent when moving a photo).
- **Photos:**
  - "Esta foto passa de 10 MB.", "Use uma foto JPEG, PNG ou WebP.";
  - "Cada item pode ter até 8 fotos.";
  - "A foto não foi enviada. Tente de novo."
- **After saving:** "Item salvo.", "{item} foi arquivado.", "{item} voltou ao catálogo."
- **Refusals:**
  - archiving an item in a kit: "Este item está no kit {kit}. Tire-o do kit antes de arquivar.";
  - someone else saved first: "Outra pessoa alterou este item. Recarregue para ver a versão atual.", with the button "Recarregar".

**Categorias** (dialog)

- Title "Categorias" (while loading: "Carregando as categorias…"). A list in the store's order, with "Mover para cima", "Mover para baixo", "Renomear" and "Excluir". Button "Nova categoria", field "Nome da categoria".
- "Informe o nome da categoria.", "Já existe uma categoria com este nome."
- Also: "Use um nome de até 120 caracteres.", "Use um nome sem quebras de linha ou caracteres de controle.", "Use uma posição de zero ou mais."
- A category in use: "Esta categoria tem itens. Mude os itens de categoria antes de excluir."

**Campos** (dialog)

- Title "Campos dos itens" (while loading: "Carregando os campos…"), hint "Campos como \"Serve em\" ajudam o cliente a achar o que serve para ele."
- Each field has "Nome do campo" and its "Valores", with "Adicionar valor", "Remover valor" and "Excluir campo". Button "Novo campo".
- "Informe o nome do campo.", "Informe o valor.", "Este valor já existe neste campo."
- Also: "Use um nome de até 120 caracteres.", "Use um valor de até 80 caracteres.", "Use um valor sem quebras de linha ou caracteres de controle.", "Use até 200 valores por campo."
- Removing a value or a field in use asks first: "Remover {valor}? Ele sai de {n} itens e kits.", buttons "Cancelar" and "Remover".

**Also on the Itens screens** (added by Claude in step 3.12, for the user to review)

- Read-only roles: the item opens with the title "Item", the fields disabled and one button, "Fechar". "Categorias" and "Campos" open to read, without "Nova categoria", "Novo campo" or any button that changes anything. The "No portal" switch shows its state, disabled.
- The category filter is read aloud as "Filtrar por categoria". The table is read aloud as "Itens do catálogo".
- Leaving the item form with edits not saved (Cancelar, Escape or a click outside) asks "Descartar as alterações?", with "Continuar editando" (the keyboard lands here) and "Descartar".
- On an empty catalog only the toolbar's "Novo item" is the primary button; the one beside "Nenhum item no catálogo ainda." is a plain button.
- Item form:
  - "Descrição" is the label, with no "(opcional)": an empty one is allowed;
  - the choice of supplier starts at "Escolha o fornecedor"; the backup supplier's choice is labelled "Fornecedor reserva" and, left empty, says "Escolha o fornecedor reserva."; the group "Fornecedor reserva" has the same "Prazo do fornecedor (dias)" and "Custo" fields;
  - production days outside 1 to 365: "Use de 1 a 365 dias de produção.";
  - the "No portal" switch waits, disabled, while the change is saved;
  - the new supplier's number is typed with its DDD; the app sends it with 55 in front;
  - "Novo fornecedor" also has a "Cancelar" button;
  - options and choices are numbered for screen readers, "Opção 1", "Escolha 1";
  - an empty choice: "Informe a escolha.";
  - "Salvar" closes the form after "Item salvo.". A new item has no photos until it is saved: "Salve o item para adicionar fotos.";
  - photos are read aloud as "Foto {n}"; while a photo goes up: "Enviando a foto…". The first photo shows "Capa" instead of "Usar como capa";
  - a photo the server refuses as not JPEG, PNG or WebP, or over 10 MB: "Use uma foto JPEG, PNG ou WebP de até 10 MB.";
  - an item archived in a kit the API does not name: "Este item está em um kit. Tire-o do kit antes de arquivar.";
  - archiving an item that several kits hold: "Este item está nos kits {A}, {B} e {C}. Tire-o dos kits antes de arquivar."
- Categorias: a category that was deleted elsewhere meanwhile: "Esta categoria não existe mais."; renaming shows a field "Novo nome de {categoria}" with "Salvar" and "Cancelar"; the dialog closes with "Fechar". The buttons are read aloud with the category, as "Mover {categoria} para cima", "Renomear {categoria}", "Excluir {categoria}".
- Campos:
  - each field has "Salvar campo" (nothing is saved until then); its values are "Valor 1", "Valor 2" …, with "Remover valor";
  - removing a field whose values are in use asks first: "Remover {campo}? Seus valores saem dos itens e kits que os usam." (no count: the API counts each value, and adding them up would count an item twice);
  - a field name already used: "Já existe um campo com este nome.";
  - the question is read out as the group "Confirmar remoção", with the keyboard on "Cancelar"; "Remover" is not the primary button;
  - a field deleted elsewhere meanwhile: "Este campo não existe mais."

**Kits**

- Table: "Kit", "Itens", "Preço do kit", "Soma avulsa", "Margem", "Prazo", "No portal". Button "Novo kit". Empty: "Nenhum kit ainda."
- **Kit editor**, title "Novo kit" or "Editar kit":
  - **Fields:** "Nome", "Descrição" (optional), "Preço do kit".
  - **"Itens do kit":** "Adicionar item". Each row has the item, "Quantidade", the box "Cliente pode trocar" and "Remover do kit".
  - **A swappable item** shows "Pode trocar por", a choice of several items, with "Sugestões da categoria {categoria}" first.
  - **"Ao trocar um item":**
    - "Manter o desconto do kit" (the default), hint "Preço do kit − item retirado + item novo, pelos preços avulsos.";
    - "Cobrar a soma dos itens avulsos", hint "O kit trocado custa a soma dos preços avulsos dos seus itens."
  - **Days:**
    - "Dias de {Montagem}" and "Margem de segurança (dias)";
    - each with the hint "Padrão da loja: {n} dias" and blank to use it.
  - **Totals, from the API:**
    - "Soma dos itens avulsos: {valor}";
    - "Desconto do kit: {valor}";
    - "Margem: {valor} ({n}%)" (Dono only);
    - "Prazo calculado: {n} dias (item mais lento {a} + {Montagem} {b} + margem de segurança {c})".
  - **Other sections:** "Campos", "Fotos", "Mostrar na página da loja", "Salvar", "Cancelar", "Arquivar" / "Restaurar", as for items.
- **Kit errors:**
  - "Informe o nome do kit.", "Informe o preço do kit.";
  - the name, description and amount-too-high messages are those of items;
  - "Escolha o que acontece ao trocar um item.", "Use de 0 a 60 dias.", "Use até 50 itens no kit.", "Escolha um item do catálogo.", "Este item já está no kit.", "Use uma quantidade de 1 a 99.";
  - "Só itens que o cliente pode trocar têm alternativas.", "O item não pode ser trocado por ele mesmo.", "Esta alternativa já foi escolhida.", "Use até 50 alternativas.";
  - "Adicione ao menos um item.";
  - "Escolha ao menos um item para a troca.";
  - "{item} está arquivado e não pode entrar no kit.";
  - "Outra pessoa alterou este kit. Recarregue para ver a versão atual."
- A kit price above the single-item sum shows, without blocking: "O preço do kit está acima da soma avulsa."

**Also on the Kits screens** (added by Claude in step 3.13, for the user to review)

- Tabs: "Itens" and "Kits" are a tab list; the arrow keys, Home and End move between them.
- Table: read aloud as "Kits do catálogo"; the box "Mostrar arquivados"; while loading "Carregando os kits…"; at the end of a page "Carregar mais". "Margem" is "{valor} ({n}%)", or "—" without a cost. An archived row shows "Arquivado" beside the name. On an empty list only the toolbar's "Novo kit" is the primary button.
- Read-only roles: the kit opens with the title "Kit", the fields disabled and one button, "Fechar". The "No portal" switch shows its state, disabled.
- "Itens do kit":
  - the picker is "Item a adicionar", starting at "Escolha o item", with the button "Adicionar item"; it lists non-archived items that are not in the kit yet, and when there are none it says "Nenhum item disponível para adicionar.";
  - "Remover do kit" is read aloud with the item, "Remover {item} do kit";
  - a row whose item was archived after the kit was saved shows "Arquivado";
  - under "Pode trocar por", an archived item that the row still names is listed as "{item} (arquivado)" so it can be unticked; the items after the suggestions are under "Outros itens", and an item with no category gets one list with no heading.
- Quantity and days:
  - "Quantidade" outside 1 to 99: "Use uma quantidade de 1 a 99.";
  - days outside 0 to 60: "Use de 0 a 60 dias.";
  - the store's default reads "Padrão da loja: 1 dia" or "Padrão da loja: {n} dias".
- The totals block is read aloud as "Totais do kit". It shows what the API sent with the kit as last saved; nothing is worked out on the screen:
  - a new kit: "Os totais e o prazo aparecem depois de salvar.";
  - a saved kit whose form has edits: "Os totais e o prazo mostram o último salvamento. Eles se atualizam depois de salvar.";
  - "Margem: —" when the Dono's kit has no cost to take a margin from.
- After "Salvar" the editor stays open on the saved kit, so the new totals show and a new kit can take photos: "Kit salvo.". "Arquivar" and "Restaurar" close it: "{kit} foi arquivado.", "{kit} voltou ao catálogo."
- Photos: "Salve o kit para adicionar fotos." and "Cada kit pode ter até 8 fotos."
- An archived item the API names, said once per item: "{item} está arquivado e não pode entrar no kit."; when it names none: "Um item do kit está arquivado e não pode entrar no kit." Restoring a kit holding an archived item says the same.
- Leaving the kit form with edits not saved asks "Descartar as alterações?", as for items.

**On the phone, for Kits** (added by Claude in step 3.16, for the user to review)

- "Catálogo" is one screen with the tabs "Itens" and "Kits" under its title; the open tab is kept when a form is closed.
- Each kit row is a card with one labelled line each: "Itens", "Preço do kit", "Soma avulsa", "Margem" (only when the API sent it, "{valor} ({n}%)" or "—"), "Prazo", then the "No portal" switch, read aloud as "Mostrar {kit} na página da loja". Tapping the card opens the kit. There is no search field. An archived card shows "Arquivado" beside the name.
- On an empty list "Novo kit" is the toolbar's primary button, and the one beside "Nenhum kit ainda." is a plain button. After a failed read: "Tentar de novo".
- Nothing is kept on the phone (ADR 0029): without a connection the kits show "Sem conexão com o servidor. Confira a internet e tente de novo.", as for items.
- A role that only reads opens the kit as its details in labelled lines (name, description, price, each item with its quantity and what it can be swapped for, "Ao trocar um item", the days, the totals, fields, "Mostrar na página da loja") with the title "Kit" and one "Fechar" button.
- The editor shows "Carregando o kit…" while it reads, and "Tentar de novo" if it cannot.
- The totals block has the visible heading "Totais do kit".
- "Quantidade" and "Cliente pode trocar" sit on each item's card; "Pode trocar por" is a list of boxes, with the headings "Sugestões da categoria {categoria}" and "Outros itens" as on the web. The two ways to swap are a choice of two ("Manter o desconto do kit", "Cobrar a soma dos itens avulsos"), each with its hint.
- "Salvar" shows "Kit salvo." at the top and stays on the kit; system Back is "Cancelar".

## Milestone 4 screens

Drafted by Claude on 2026-11-27 for milestone 4, for the user to review in the plan's PR (ADR 0030). Web and the Android shell use the same text. The messages in "Account and store screens" (no connection, server failure, rate limit, another refusal) apply here too. `{Montagem}` is the template's niche word, "Montagem" until templates exist. Status names come from the order's workflow and are never written here. Dates are "dd/mm/aaaa"; a weekday is added where it helps ("sex., 12/12/2026").

### Menu

Hoje · Pedidos · Catálogo · Clientes · Financeiro · Equipe · Configurações. "Clientes" needs `order:read`, "Financeiro" `finance:read` (Dono only), "Configurações" `settings:write` (Dono only).

### Configurações

- Title "Configurações". While loading: "Carregando as configurações…". Sections "Lista de status", "Prazos e sinal", "Compras", "Estoque e Produção", "WhatsApp da loja", "Fuso horário".
- **Lista de status:** hint "Os status que você e seus clientes veem em cada pedido. Vale para os pedidos novos; os pedidos que já existem continuam com a lista em que foram criados."
  - One choice per preset, with its name (such as "Encomenda com montagem") and its steps in order. Each step shows the staff name with its status badge and, under it, "O cliente vê: {client label}".
  - The current one is marked "Em uso". Button "Usar esta lista". After it: "Lista de status alterada. Os próximos pedidos usam {nome}." If the list no longer exists: "Esta lista de status não está mais disponível. Recarregue para ver as listas atuais."
- **Prazos e sinal:** "Sinal (%)", hint "Quanto o cliente paga para confirmar o pedido."; "Dias de {Montagem}"; "Margem de segurança (dias)", hint "Folga somada ao prazo para não prometer em cima da hora."
- **Compras:** box "Liberar compras antes do sinal", hint "Desligado: nada é comprado antes de o cliente pagar o sinal."
- **Estoque e Produção:** boxes "Usar Estoque" and "Usar Produção própria", hint "Desligados, os menus e as origens ficam escondidos."
- **WhatsApp da loja:** field with the hint "Com DDD, como (92) 99123-4567."
- **Fuso horário:** a choice of "Horário de Brasília", "Horário do Amazonas (Manaus)", "Horário do Acre (Rio Branco)", "Horário de Fernando de Noronha"; hint "Decide quando um dia começa para os prazos e alertas."
- Button "Salvar". After it: "Configurações salvas." Errors: "Use um sinal de 0 a 100%.", "Use de 0 a 365 dias.", "Confira o número: use DDD e 8 ou 9 dígitos.", and, when someone saved first, "Outra pessoa alterou as configurações. Recarregue para ver a versão atual."

### Clientes

- Table "Nome", "WhatsApp", "Pedidos", "Último pedido". Search "Buscar por nome ou WhatsApp". Button "Novo cliente" (only with `customer:write`). While loading: "Carregando os clientes…". Empty: "Nenhum cliente ainda." A search with no results: "Nenhum cliente encontrado." "Carregar mais".
- On the phone each row is a card with the same lines; tapping opens the customer.
- **Customer form** (drawer; full screen on the phone), title "Novo cliente" or "Cliente":
  - "Nome", "WhatsApp" (hint "Com DDD, como (92) 99123-4567."), "E-mail" (optional), "Observações".
  - "Endereços": "Adicionar endereço"; each has "Nome do endereço" (such as "Casa"), "CEP", "Rua", "Número", "Complemento", "Bairro", "Cidade", "UF", and "Remover endereço".
  - "Pedidos deste cliente": the orders, newest first (the highest number first), with number, status and "Entrega prevista". The drawer reads up to 500 orders; with more, it shows the 500 with the nearest delivery and says "Mostrando os 500 pedidos com entrega mais próxima."
  - Buttons "Salvar", "Cancelar". Read-only roles: title "Cliente", the details in labelled lines, and "Fechar".
- A customer that is not there (a wrong address): "Cliente não encontrado."
- Errors: "Informe o nome do cliente.", "Informe o WhatsApp do cliente.", "Confira o número: use DDD e 8 ou 9 dígitos.", "Já existe um cliente com este WhatsApp.", "Confira o e-mail.", "Confira o CEP: use 8 dígitos.", "Escolha a UF.", "Use até 10 endereços.", "Use um nome de até 120 caracteres.", "Use observações de até 2000 caracteres.", and for each address "Informe o nome do endereço.", "Use um nome de endereço de até 40 caracteres.", "Informe a rua.", "Use uma rua de até 200 caracteres.", "Use um número de até 20 caracteres.", "Use um complemento de até 100 caracteres.", "Use um bairro de até 100 caracteres.", "Informe a cidade.", "Use uma cidade de até 100 caracteres.", "Outra pessoa alterou este cliente. Recarregue para ver a versão atual."
- After saving: "Cliente salvo."

### Pedidos

- Title "Pedidos". Tabs: "Ativos", then one per status of the store's current list, then "Entregues" and "Cancelados". Search "Buscar por número ou cliente". Filter "Só com alerta". Button "Novo pedido". A link from Hoje may preset a phase: the list then shows a chip with the word of that phase in the store's own list and a ×, such as "Aguardando sinal ×", which takes the filter off. With no result: "Nenhum pedido encontrado." (never "Nenhum pedido ainda." while a filter is on).
- Table "Pedido", "Cliente", "Status", "Itens prontos", "Entrega prevista", "Total". "Pedido" shows the number in mono (P-0231). "Itens prontos" is "{a} de {b}". An alert sits on top of the status: "Pode atrasar" or "Atrasada", with its icon.
- While loading: "Carregando os pedidos…". Empty: "Nenhum pedido ainda." with "Novo pedido". No results: "Nenhum pedido encontrado." "Carregar mais".
- On the phone each order is a card: number and status, then "Cliente", "Itens prontos", "Entrega prevista", "Total".

### Novo pedido and Alterar pedido

- Title "Novo pedido" or "Alterar pedido P-0231".
- **Cliente:** "Buscar cliente" (name or WhatsApp), or "Novo cliente" with "Nome" and "WhatsApp" and the button "Criar cliente" (which only takes the two fields: the customer is made, or found by its WhatsApp, when the order is saved). "Informe o cliente do pedido." Once chosen: the name and WhatsApp, with "Trocar cliente" (not when changing an order). A search with nothing: "Nenhum cliente encontrado."
- **Itens:** "Adicionar kit" and "Adicionar item".
  - A kit shows its items; a swappable one has "Trocar" with the kit's alternatives. A swap shows "{item} no lugar de {item}".
  - Each item has "Quantidade" (1 to 99); a kit line is one kit (add the kit again for another). Each item or kit has its client options ("Escolha uma opção" for a required one: "Escolha {opção}."), and "Remover".
  - "Adicione ao menos um item ou kit." "{item} está arquivado e não pode entrar no pedido." "Use uma quantidade de 1 a 99."
  - "Adicionar kit" and "Adicionar item" open a list with "Buscar kit" or "Buscar item", the price of each, "Nenhum kit encontrado." or "Nenhum item encontrado.", and "Fechar". A line shows what the quote said of it: its price and "{item}: previsto para {data}".
- **Entrega:** a choice of "Retirada na loja" and "Entrega". "Entrega" shows "Endereço" (the customer's, or "Novo endereço") and "Frete". "Escolha o endereço de entrega." "Novo endereço" shows the address fields of Clientes; the address is saved on the customer with the order (anyone who can make orders; ADR 0030, Amendments).
- **Valores** (Dono and Admin only change these): "Desconto", "Sinal (%)" with the hint "Padrão da loja: {n}%", "Dias de {Montagem}" and "Margem de segurança (dias)", each "Padrão da loja: {n} dias".
  - "O desconto não pode passar do valor dos itens." "Use um sinal de 0 a 100%." "Use de 0 a 60 dias." "Confira o valor." (a discount or freight that is not an amount).
- **Resumo**, always from the API ("Calculando…" while it waits; "Os valores aparecem quando o pedido tiver itens." before that): "Itens: {valor}", "Frete: {valor}", "Desconto: −{valor}", "Total: {valor}", "Sinal ({n}%): {valor}", "Entrega prevista: {data}" ("{n} dias depois do sinal"), and for the Dono "Custo: {valor}" and "Margem: {valor} ({n}%)" ("—" when the order has no cost to give). The other roles never see the last two.
- Buttons: "Salvar rascunho", "Salvar e enviar ao cliente" (moves the order to the next status and opens WhatsApp with the tracking link), "Cancelar". When changing: "Salvar alterações".
- **Changing a paid order** (Dono and Admin, before {Montagem}):
  - a notice "Este pedido já tem pagamento. Combine a mudança com o cliente antes de salvar.";
  - box "O cliente aprovou a mudança", required: "Confirme que o cliente aprovou a mudança.";
  - before saving, under "O que muda": "Total: {antes} → {depois}", "Entrega prevista: {antes} → {depois}" and, when the new total is below what was paid, "Valor a devolver: {valor}" (from the quote; it shows again after saving);
  - after saving: "Pedido alterado." and, only when the API sends the message, the button "Avisar o cliente pelo WhatsApp", which opens the link when tapped; and "Abrir pedido".
  - Past {Montagem}: "Este pedido já está em {Montagem} e não pode mais ser alterado."
- After creating: "Pedido P-0231 criado." with "Link de acompanhamento: {link}" and "Enviar pelo WhatsApp" (for a draft, which has no message yet, it asks for a new link first, so the link shown is the new one), "Abrir pedido" and "Voltar para Pedidos". "Salvar e enviar ao cliente" also opens WhatsApp at once; the button stays for a browser that blocks it. The link opened by "Salvar e enviar ao cliente" stays as that button, so a browser that blocks the automatic opening still lets the person tap it. Leaving with edits: "Descartar as alterações?" as in Catálogo.

### Pedido (the order screen)

- **Header:** "Pedido P-0231", the status badge, the customer's name and WhatsApp with the button "Abrir conversa no WhatsApp" (it opens the link the API sends with the order), "Criado em {data}". While loading: "Carregando o pedido…"; an order that is not there: "Pedido não encontrado." with "Tentar de novo" and "Voltar para Pedidos". The header holds "Alterar pedido" (opens Alterar pedido; not for staff once the order has a discount or a payment), "Cancelar pedido" and "Enviar link de acompanhamento".
- **Entrega:** "Entrega prevista: {data}", "Precisa chegar até: {data}", "{Retirada na loja | Entrega em {endereço}}". Button "Alterar data" (Dono, Admin).
- **Delay banner,** when an item has an alert: "{n} item pode atrasar a entrega" / "{n} itens podem atrasar a entrega" or "{n} item atrasado" / "{n} itens atrasados", then per item "{item}: previsto para {data}, precisa chegar até {data}." Ways to fix, as buttons: "Usar o fornecedor reserva ({fornecedor}, {n} dias)" (Dono and Admin, while the item is still "A comprar" and has a backup supplier), and "Alterar a data de entrega". After the switch: "{item} agora vem de {fornecedor}. Previsto: {data}." "Falar com o fornecedor" comes with Compras (M5).
- **Linha do tempo:** under the title, the marks as text ("Hoje: {data}", "Precisa chegar até: {data}", "{Montagem}: {data} a {data}", "Entrega: {data}"). One row per item, on a band from today to the delivery date, marked "Hoje", "Precisa chegar até", "{Montagem}" and "Entrega". Each item shows its expected date ("Previsto: {data}") or "Recebido em {data}".
- **Itens:** table "Item", "Origem", "Situação", "Previsto", "Ações". "Situação" names:
  - Sob encomenda (and Estoque until M6): "A comprar", "Enviada ao fornecedor", "Em trânsito", "Recebida", "Instalada";
  - Produção própria: "Planejado", "Em produção", "Pronto", "Instalado".
  - Actions, one step at a time: "Marcar como enviada ao fornecedor", "Marcar como em trânsito", "Marcar como recebida", "Marcar como instalada"; "Iniciar produção", "Marcar como pronto"; on receiving, "Tudo certo" or "Com defeito". A defective item goes back: "{item} voltou para A comprar." / "{item} voltou para Em produção."
- **Pagamentos:** "Total {valor}", "Sinal {valor} ({n}%)", "Pago {valor}", "Saldo {valor}", and "Valor a devolver {valor}" when it applies. List "Data", "Tipo" ("Sinal", "Saldo"), "Forma", "Valor", "Registrado por", with "Anulado" on a voided one.
  - "Registrar pagamento": "Tipo", "Valor" (filled with what is still owed: of the deposit for "Sinal", the balance for "Saldo"), "Forma" ("Pix", "Dinheiro", "Cartão de crédito", "Cartão de débito", "Transferência", "Outro"), "Registrar". "O valor passa do que falta pagar ({valor})." "Informe um valor maior que zero."
  - After it: "Pagamento registrado." Every dialog has "Voltar" or "Cancelar" next to its button.
  - "Anular" (Dono): "Anular o pagamento de {valor}?" with "Motivo" and "Anular pagamento". "Informe o motivo." After it: "Pagamento anulado."
  - "Liberar sem sinal" (Dono, while waiting for the deposit): "Liberar o pedido sem o sinal? As compras e a produção podem começar antes do pagamento.", "Liberar pedido" (the dialog is titled "Liberar o pedido sem o sinal?"). After it: "Pedido liberado."
- **Next status:** one button per status the order can move to, as "Mover para {status}". Refusals:
  - "O sinal ainda não foi pago. Registre o sinal ou libere o pedido sem ele.";
  - "Nem todos os itens chegaram e foram conferidos.";
  - "Falta receber o saldo de {valor}.";
  - "Este pedido não pode ir para {status}."
  - Moving to the handover status asks "Como o pedido foi entregue?" with "Entrega" and "Retirada na loja".
- **Alterar data** (Dono, Admin): "Nova data de entrega", "Motivo" (with the hint "Opcional."), "Salvar e avisar o cliente", which opens WhatsApp. "Escolha uma data a partir de hoje." After it: "Data alterada. Envie a mensagem para avisar o cliente." with the button "Abrir o WhatsApp", which stays until the next action, in case the browser blocked the automatic opening.
- **Alterar pedido by someone who may not:** "Só o Dono ou um Admin altera um pedido com pagamento ou desconto." Once the order is delivered or cancelled: "Este pedido já foi entregue ou cancelado."
- **Link de acompanhamento:** "Enviar link de acompanhamento", which opens WhatsApp; the hint "Cada envio cria um link novo. O link anterior deixa de funcionar." After it: "Link de acompanhamento criado." with the button "Abrir o WhatsApp", which stays until the next action.
- **Cancelar pedido** (Dono, Admin, before handover): "Cancelar o pedido P-0231?" with "Motivo" and "Cancelar pedido". "Informe o motivo." If paid: "Os pagamentos registrados continuam no pedido. A devolução é feita fora do Encomendo." After it: "Pedido cancelado."
- **Histórico:** "{quem} {o quê}" with date and time ("O sistema" when nobody did it), such as "Ana criou o pedido", "Ana marcou {item} como recebida (com defeito)", "Ana anulou o pagamento de {valor}: {motivo}", "Ana liberou o pedido sem o sinal", "Ana usou o fornecedor reserva em {item}", "Ana alterou a entrega de {data} para {data}" (with ". Motivo: {motivo}" when there is one), "Ana moveu para Em montagem", "Ana registrou o sinal de R$ 300,00 (Pix)", "Ana alterou o pedido (cliente aprovou)", "Ana enviou o link de acompanhamento", "Ana cancelou: {motivo}".
- "Outra pessoa alterou este pedido. Recarregue para ver a versão atual.", with "Recarregar".

### Hoje

- Title "Hoje", with the date ("sexta-feira, 27 de novembro").
- **Summary cards:** "Pedidos ativos", "Aguardando sinal", "Com alerta", "Entregas nos próximos 7 dias". Each opens Pedidos with that filter.
- **"Precisa de você"**, most urgent first. Each entry names the order and the customer and opens the order:
  - "{item} está atrasado." (Atrasada)
  - "{item} pode atrasar a entrega." (Pode atrasar)
  - "Aguardando o sinal há {n} dias." (after 3 days)
  - "Pronto há {n} dias e ainda não entregue." (after 2 days)
  - "Valor a devolver: {valor}."
  - Nothing: "Nada precisa de você agora."
- **"Próximas entregas":** the active orders by delivery date: number, customer, status, "Itens prontos {a} de {b}", "Entrega {data}". "Ver todos os pedidos". Nothing coming: "Nenhuma entrega prevista."
- While loading: "Carregando o resumo de hoje…". With more active orders than Hoje reads: "Mostrando só os 500 pedidos ativos com entrega mais próxima. Os números e os avisos desta tela cobrem só esses pedidos."
- "Entregas nos próximos 7 dias" opens the plain order list until the list can filter by delivery window.

### Financeiro

- Title "Financeiro", Dono only. Period: "Este mês", "Mês passado", "Últimos 30 dias", "Escolher período" (from, to).
- **Recebido no período:** total, then by type ("Sinal", "Saldo") and by form (Pix, Dinheiro …). Voided payments don't count.
- **A receber:** the sum of the balances of active orders, then "{n} pedidos com saldo" ("1 pedido com saldo") and the list "Pedido", "Cliente", "Saldo", "Entrega prevista".
- **A devolver:** orders with "Valor a devolver", when there are any: "Pedido", "Cliente", "Status", "Valor a devolver".
- **Margem dos pedidos entregues:** "Pedido", "Entregue em", "Total", "Custo", "Margem" ("{valor} ({n}%)", or "—" without a cost), and the period's total as "Total do período", with the hint "Custo e margem somam só os pedidos com todos os custos informados." No order handed over in the period: "Nenhum pedido entregue neste período."
- Empty period: "Nenhum pagamento neste período."
- The choice is "Período". "Escolher período" shows "De" and "Até" (days); the period is read when either day changes and both are filled in. A period the API refuses: "Confira o período: o início não pode passar do fim, e o período tem no máximo 366 dias."
- While loading: "Carregando o financeiro…". With more orders than Financeiro reads: "A loja tem mais pedidos do que o Financeiro lê de uma vez. "A receber", "A devolver" e as margens cobrem só parte dos pedidos."

### WhatsApp messages

Prefilled texts the API puts in `wa.me` links. `{loja}` is the store's name, `{nome}` the customer's first name.

- **Tracking link:** "Olá, {nome}! Aqui é da {loja}. Acompanhe seu pedido {número} por este link: {link}"
- **Date changed:** "Olá, {nome}! Aqui é da {loja}. A entrega do seu pedido {número} mudou de {data antiga} para {data nova}.{ Motivo: {motivo}.} Qualquer dúvida, é só responder esta mensagem."
- **Order changed after payment:** "Olá, {nome}! Aqui é da {loja}. Como combinamos, seu pedido {número} foi alterado. Novo total: {valor}. Entrega prevista: {data}.{ Valor a devolver: {valor}.} Qualquer dúvida, é só responder esta mensagem."
- **Sent with the order:** "Olá, {nome}! Aqui é da {loja}. Recebemos seu pedido {número}, total {valor}. O sinal é {valor}. Acompanhe por este link: {link}"

The texts never name a supplier, a cost or "Pode atrasar" (System rules).

## Milestone 5 screens

Drafted by Claude on 2026-11-28 for milestone 5, for the user to review in the plan's PR (ADR 0031). The messages in "Account and store screens" and the date and `{Montagem}` conventions of "Milestone 4 screens" apply here too. A **pedido de compra** is what the store sends one supplier, numbered per store as "C-0012"; a client's order stays a **pedido** ("P-0231"). Costs and "Preço pago" are the Dono's only.

### Menu

Hoje · Pedidos · Compras · Catálogo · Clientes · Financeiro · Equipe · Configurações. "Compras" needs `purchase:write` (Dono, Admin). A Funcionário, who only receives, gets the entry "Receber itens" in its place.

### Compras

- Title "Compras". Tabs "A comprar", "Enviadas", "Receber itens", "Fornecedores". While loading: "Carregando as compras…".
- **A comprar:** one card per supplier, with its name, its WhatsApp and "{n} itens" ("1 item").
  - Each line has a box (all checked at first), "{qtd}× {item}" with its choices ("Cor: Preto"), "Pedido P-0231" (opens the order) and "Precisa chegar até {data}". A replacement shows the badge "Reposição". For the Dono: "Custo {valor}".
  - "Ver mensagem" shows the text the API built for the checked lines, with "Copiar mensagem".
  - Button "Enviar pelo WhatsApp", with the hint "Os itens marcados ficam como Enviada ao fornecedor." "Marque ao menos um item."
  - After it: "Pedido de compra C-0012 criado." with "Abrir o WhatsApp" (it stays until the next action, in case the browser blocked the automatic opening), "Copiar mensagem" and "Ver pedido de compra".
  - Empty: "Nada para comprar agora." with the hint "Os itens aparecem aqui quando o cliente paga o sinal." (with "Liberar compras antes do sinal" on: "… quando o pedido sai do rascunho.").
- **Enviadas:** filter "Abertos" / "Todos".
  - Table "Pedido de compra", "Fornecedor", "Enviado em", "Prazo do fornecedor" ("Sem prazo" when there is none), "Recebidos" ("{a} de {b}"), "Situação" ("Enviado", "Em trânsito", "Recebido", "Desfeito").
  - On the phone each is a card with the same lines. Empty: "Nenhum pedido de compra ainda." "Carregar mais".
- **Avisar o fornecedor**, above the tabs when there are any: "{n} itens já enviados não são mais necessários." Each shows "C-0012 · {fornecedor}", "{qtd}× {item}", and why: "O pedido P-0231 foi cancelado." or "{item} saiu do pedido P-0231."
  - Buttons "Avisar pelo WhatsApp" and "Marcar como resolvido". After it: "Resolvido. A linha foi cancelada."

### Pedido de compra

- Title "Pedido de compra C-0012", the supplier's name with "Abrir conversa no WhatsApp", "Enviado em {data} por {quem}". A pedido de compra that is not there: "Pedido de compra não encontrado."
- **Itens:** table "Item", "Qtd", "Pedido", "Precisa chegar até", "Previsto", "Situação".
  - "Situação" names: "Enviada ao fornecedor", "Em trânsito", "Recebida", "Com defeito (reposição em Compras)", "Cancelada", "Avisar o fornecedor".
  - For the Dono, "Custo" and "Preço pago" too.
  - A line whose "Previsto" is after "Precisa chegar até" shows "Pode atrasar" with its icon.
- **Prazo do fornecedor:** "Informar prazo" opens "Data prometida pelo fornecedor" (for every item), then "Prazo diferente para um item" (one date per line), and "Salvar prazo".
  - "Escolha uma data a partir de hoje."
  - After it: "Prazo salvo. As datas previstas dos pedidos foram atualizadas." When a line is now late for its order: "{item} do pedido P-0231 pode atrasar a entrega. Veja o pedido para resolver."
- "Marcar como em trânsito". After it: "Itens em trânsito."
- "Receber itens" opens Receber itens on this pedido de compra.
- **Informar preço pago** (Dono): one field per line, "Preço pago (por unidade)", filled with the cost at order time; "Salvar preços".
  - "Confira o valor."
  - After it: "Preços salvos. As margens do Financeiro usam o preço pago."
- **Desfazer envio**, while nothing was received: "Desfazer o envio do pedido de compra C-0012? Os itens voltam para A comprar. O número C-0012 não será usado de novo." with "Desfazer envio" and "Voltar".
  - After it: "Envio desfeito."
  - Once something was received: "Este pedido de compra já tem itens recebidos e não pode ser desfeito."
- "Outra pessoa alterou este pedido de compra. Recarregue para ver a versão atual.", with "Recarregar".

### Receber itens

Workshop sizes: 18 px text, 56 px targets, a 64 px main button.

- Title "Receber itens". While loading: "Carregando o que falta receber…". Search "Buscar item, pedido ou fornecedor".
- One group per pedido de compra, "C-0012 · {fornecedor}", then "Comprados sem pedido de compra", the lines still in "A comprar", for items bought at the counter. Empty: "Nada para receber agora."
- Each line shows "{qtd}× {item}", its choices and "Pedido P-0231", with two large buttons, "Tudo certo" and "Com defeito".
  - Hint above the list: "Marque um item só quando toda a quantidade chegou."
  - "Com defeito" opens "O que houve?" (required: "Conte o que houve com {item}.") and "Foto do item".
  - "Tudo certo" also offers "Foto do item". At most 2 photos per item: "Cada item pode ter até 2 fotos."
- "Foto da nota fiscal", once per save, at most 4: "Cada recebimento pode ter até 4 fotos da nota."
  - The file input offers the camera or the gallery. While a photo goes up: "Enviando a foto…".
  - Errors as in Catálogo: "Use uma foto JPEG, PNG ou WebP de até 10 MB.", "A foto não foi enviada. Tente de novo."
- Button "Salvar recebimento" ("Marque Tudo certo ou Com defeito em ao menos um item.").
  - A save that fails keeps every check and photo, and "Tentar de novo" sends the same save again.
- After it: "Recebimento salvo." Then, as they apply:
  - "{item} voltou para A comprar. A reposição está em Compras.";
  - "Pedido P-0231: todos os itens chegaram e foram conferidos." with "Abrir pedido".

### Fornecedores (tab of Compras)

- Table "Fornecedor", "WhatsApp", "Itens", "Pedidos de compra abertos". Search "Buscar fornecedor". Button "Novo fornecedor".
- Empty: "Nenhum fornecedor ainda. Você também pode criar um fornecedor no cadastro do item."
- **Form** (drawer; full screen on the phone), title "Novo fornecedor" or "Fornecedor":
  - "Nome" and "WhatsApp" (hint "Com DDD, como (92) 99123-4567."), with the errors already used in Catálogo;
  - "Itens deste fornecedor": "Item", "Papel" ("Principal", "Reserva"), "Prazo (dias)" and, for the Dono, "Custo";
  - "Salvar", "Cancelar". After it: "Fornecedor salvo."

### Changes to Pedido (the order screen)

- An item bought from a supplier no longer has the buttons "Marcar como enviada ao fornecedor", "Marcar como em trânsito", "Tudo certo" or "Com defeito". Its "Situação" shows the status, then "C-0012" (opens the pedido de compra) and "Prazo do fornecedor {data}" when there is one.
  - It keeps "Marcar como instalada" once received.
  - While "A comprar" it shows "Ver em Compras"; once sent it shows "Receber itens" (for those who receive).
  - Items of Produção própria keep their buttons.
- If the API refuses a move because purchasing drives it: "Este item é movido por Compras. Use Compras ou Receber itens."
- The delay banner gains "Falar com o fornecedor ({fornecedor})" (Dono, Admin), which opens WhatsApp.
- **Histórico** gains:
  - "Ana enviou {item} ao fornecedor (C-0012)";
  - "Ana informou o prazo do fornecedor de {item}: {data}";
  - "Ana marcou {item} como em trânsito";
  - "Ana desfez o envio de {item} (C-0012)";
  - "Ana recebeu {item} (tudo certo)";
  - "Ana recebeu {item} com defeito: {nota}".

### Changes to Hoje

- A summary card "Compras a enviar" ({n} itens), for Dono and Admin, which opens Compras.
- **"Precisa de você"** gains, for Dono and Admin:
  - "{n} itens para comprar de {fornecedor}." (opens A comprar);
  - "Pedido de compra C-0012 sem prazo do fornecedor há {n} dias." (more than 2 days after sending);
  - "Avise {fornecedor}: {item} não é mais necessário (C-0012)."

### Supplier messages

Prefilled texts the API puts in `wa.me` links to a supplier. `{fornecedor}` is the supplier's name and `{loja}` the store's. The lines sum the same item with the same choices.

- **Pedido de compra:**
  "Olá, {fornecedor}! Aqui é da {loja}.
  Pedido de compra {C-0012}:
  • {qtd}× {item} ({opção}: {escolha}, …)
  Pode me confirmar a data de entrega?"
- **Falar com o fornecedor:** "Olá, {fornecedor}! Aqui é da {loja}. Sobre o pedido de compra {C-0012}: {qtd}× {item}. Consegue me confirmar a data de entrega?" (before it is sent: "Sobre {qtd}× {item}: consegue me dizer o prazo de entrega?")
- **Avisar o fornecedor:** "Olá, {fornecedor}! Aqui é da {loja}. Sobre o pedido de compra {C-0012}: não vou mais precisar de {qtd}× {item}. Consegue cancelar?"

Product rule: these texts never carry the client's name, phone or address, nor the client's order number (System rules).

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

Confirmed on 2026-11-26 (ADR 0023):

- Deposit is 50% by default.
- Assembly 3 days and safety margin 2 days by default, editable per kit and per order.
- Kit swap: the owner chooses per kit whether a swap keeps the discount.
- The client never sees "Pode atrasar"; the owner warns by WhatsApp.
- Produção and Estoque stay hidden until the store turns them on.

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
