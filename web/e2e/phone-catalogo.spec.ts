import { expect, test, type Locator, type Page, type Route } from '@playwright/test'

import { createStore, latestEmail, mailpitUrl, openMenu, password, signUp, unique } from './support'

// The Catálogo phone layouts (ADR 0029, step 3.5.4), in the `phone` project of
// playwright.config.ts: 390 x 844, touch. Like phone.spec.ts, the first describe
// runs against `vite preview` with the API mocked, so CI runs it; the second
// runs the milestone 3 flow against the real stack (`make stack-up`, E2E_BASE_URL
// and E2E_MAILPIT_URL). That one signs up two accounts, which count against the
// 10-an-hour-per-IP sign-up limit; clear the Redis `g:rl:*` keys between full runs.
const realStack = process.env.E2E_BASE_URL !== undefined && process.env.E2E_BASE_URL !== ''

/** expectNoSideScroll fails when the page is wider than the screen: a phone must never scroll sideways. */
async function expectNoSideScroll(page: Page): Promise<void> {
  const widths = await page.evaluate(() => ({ scroll: document.documentElement.scrollWidth, inner: window.innerWidth }))
  expect(widths.scroll, `page is ${widths.scroll} px wide on a ${widths.inner} px screen`).toBeLessThanOrEqual(widths.inner)
}

/**
 * expectSheetFits checks the open sheet: it fills the screen, and nothing in it
 * is wider than the screen or scrolls sideways inside it (the sheet's own body
 * would hide that from the page's scrollWidth).
 */
async function expectSheetFits(page: Page, name: string): Promise<Locator> {
  const sheet = page.getByRole('dialog', { name, exact: true })
  await expect(sheet).toBeVisible()
  const box = await sheet.boundingBox()
  expect([box?.x, box?.y, box?.width, box?.height]).toEqual([0, 0, 390, 844])
  await expectNoSideScroll(page)
  const wide = await sheet.evaluate((element) =>
    [...element.querySelectorAll('*')]
      .filter((child) => child.getBoundingClientRect().right > window.innerWidth + 0.5 && getComputedStyle(child).position !== 'absolute')
      .map((child) => child.outerHTML.slice(0, 100)),
  )
  expect(wide, 'elements wider than the screen').toEqual([])
  const inner = await sheet.evaluate((element) => {
    const body = element.querySelector('.enc-dialog__body')
    return body === null ? 0 : body.scrollWidth - body.clientWidth
  })
  expect(inner, 'the sheet scrolls sideways').toBeLessThanOrEqual(0)
  return sheet
}

/**
 * expectThumbSized checks the controls a thumb must hit are at least 48 px tall.
 * A checkbox or radio is hit through its label, which is what is measured.
 */
async function expectThumbSized(...controls: Locator[]): Promise<void> {
  for (const control of controls) {
    const height = await control.evaluate((element) => {
      const boxed = element instanceof HTMLInputElement && (element.type === 'checkbox' || element.type === 'radio')
      const target = (boxed ? element.closest('label') : null) ?? element
      return { height: target.getBoundingClientRect().height, html: element.outerHTML.slice(0, 100) }
    })
    expect(height.height, height.html).toBeGreaterThanOrEqual(48)
  }
}

/** expectOnScreen fails when a control is scrolled out of the 844 px screen, or sits under the bottom edge. */
async function expectOnScreen(control: Locator): Promise<void> {
  await expect(control).toBeInViewport({ ratio: 1 })
}

// A 1x1 PNG: small, and a real image so the upload check accepts it.
const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==', 'base64')

test.describe('without an API', () => {
  test.skip(realStack, 'runs only against `vite preview`, which has no API')

  const store = { id: '0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d00', name: 'Alta RPM', slug: 'altarpm' }
  const ana = { id: '0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d9e', name: 'Ana Souza', email: 'ana@altarpm.com.br' }
  const brl = (cents: number) => ({ cents, currency: 'BRL' })
  const id = (n: number) => `0190f3c4-5e1a-7b2c-8d3e-${String(n).padStart(12, '0')}`

  const session = (role: 'owner' | 'admin') => ({
    accessToken: 'e2e-access',
    tokenType: 'Bearer',
    expiresIn: 900,
    user: ana,
    tenant: store,
    role,
    permissions: ['catalog:read', 'catalog:write', 'settings:read'],
    memberships: [{ tenant: store, role, status: 'active' }],
  })

  const bancos = { id: id(0xc1), name: 'Bancos de couro para motos de todas as cilindradas', position: 0 }
  const guidoes = { id: id(0xc2), name: 'Guidões', position: 1 }
  const couros = { id: id(0xf1), name: 'Couros Paulista', whatsappPhone: '5511999990000', version: 1 }
  const serveEm = {
    id: id(0xa1),
    name: 'Serve em',
    position: 0,
    values: [
      { id: id(0xb1), label: 'CB 500X', usageCount: 3 },
      { id: id(0xb2), label: 'XRE 300', usageCount: 0 },
    ],
  }

  const bancoId = id(0x101)
  const premiumId = id(0x102)
  const kitId = id(0x201)
  const itemName = 'Banco de couro Scrambler com costura dupla vermelha'

  /** summaries are the rows of the list; the owner's carry cost and margin, and nobody else's do (ADR 0025). */
  const summaries = (owner: boolean) => [
    {
      id: bancoId,
      name: itemName,
      categoryId: bancos.id,
      sourcingMode: 'to_order',
      price: brl(45000),
      readyDays: 15,
      mainSupplierName: couros.name,
      published: true,
      archived: false,
      cover: null,
      version: 3,
      ...(owner ? { cost: brl(31500), margin: { cents: 13500, basisPoints: 3000 } } : {}),
    },
    { id: premiumId, name: 'Banco Premium', categoryId: null, sourcingMode: 'to_order', price: brl(60000), readyDays: 12, mainSupplierName: null, published: false, archived: true, cover: null, version: 1, ...(owner ? { cost: null, margin: null } : {}) },
  ]

  const itemDetail = (owner: boolean, photos: unknown[]) => ({
    id: bancoId,
    name: itemName,
    description: 'Couro sintético com costura dupla.',
    categoryId: bancos.id,
    sourcingMode: 'to_order',
    price: brl(45000),
    productionDays: null,
    readyDays: 15,
    mainSupplier: { supplierId: couros.id, supplierName: couros.name, leadDays: 15, ...(owner ? { cost: brl(31500) } : {}) },
    backupSupplier: { supplierId: couros.id, supplierName: couros.name, leadDays: 20, ...(owner ? { cost: brl(33000) } : {}) },
    attributeValueIds: [],
    options: [
      {
        id: id(0xd1),
        name: 'Cor da costura',
        required: false,
        choices: [
          { id: id(0xd2), label: 'Vermelha com detalhe branco', extraPrice: brl(5000), extraDays: 2 },
          { id: id(0xd3), label: 'Preta', extraPrice: brl(0), extraDays: 0 },
        ],
      },
    ],
    photos,
    published: true,
    archived: false,
    version: 3,
    ...(owner ? { cost: brl(31500), margin: { cents: 13500, basisPoints: 3000 } } : {}),
  })

  const kitSummary = (owner: boolean) => ({
    id: kitId,
    name: 'Kit Scrambler CB 500X completo para a cidade',
    itemCount: 2,
    price: brl(150000),
    singleItemsTotal: brl(160000),
    totalDays: 20,
    published: true,
    archived: false,
    cover: null,
    version: 4,
    ...(owner ? { cost: brl(112000), margin: { cents: 38000, basisPoints: 2530 } } : {}),
  })

  const kitDetail = (owner: boolean) => ({
    ...kitSummary(owner),
    description: 'Visual scrambler',
    discount: brl(10000),
    swapDiscount: 'keep',
    assemblyDays: null,
    safetyMarginDays: null,
    days: { slowestItem: 15, assembly: 3, safetyMargin: 2, total: 20 },
    items: [
      { itemId: bancoId, itemName, quantity: 1, swappable: true, alternativeItemIds: [premiumId], unitPrice: brl(45000), readyDays: 15, archived: false },
      { itemId: id(0x103), itemName: 'Guidão alto', quantity: 1, swappable: false, alternativeItemIds: [], unitPrice: brl(30000), readyDays: 7, archived: false },
    ],
    attributeValueIds: [],
    photos: [],
    published: true,
    archived: false,
  })

  /** answers serves what Catálogo reads, for the owner or an admin. */
  async function mockCatalog(page: Page, role: 'owner' | 'admin'): Promise<void> {
    const owner = role === 'owner'
    const photos = [
      { id: id(0xe1), url: `http://localhost:4173/photo-1.png`, expiresAt: '2030-01-01T00:00:00Z', position: 0 },
      { id: id(0xe2), url: `http://localhost:4173/photo-2.png`, expiresAt: '2030-01-01T00:00:00Z', position: 1 },
    ]
    await page.route('**/auth/refresh', (route) => route.fulfill({ json: session(role) }))
    await page.route('**/photo-*.png', (route) => route.fulfill({ contentType: 'image/png', body: png }))
    await page.route('**/api/v1/admin/**', (route: Route) => {
      const path = new URL(route.request().url()).pathname.replace('/api/v1/admin', '')
      const answers: Record<string, unknown> = {
        '/catalog/items': { items: summaries(owner) },
        [`/catalog/items/${bancoId}`]: itemDetail(owner, photos),
        '/catalog/kits': { items: [kitSummary(owner)] },
        [`/catalog/kits/${kitId}`]: kitDetail(owner),
        '/catalog/categories': { items: [bancos, guidoes] },
        '/catalog/attributes': { items: [serveEm] },
        '/suppliers': { items: [couros] },
        '/settings': {
          stockEnabled: false,
          productionEnabled: false,
          purchaseBeforeDeposit: false,
          depositPercent: 50,
          assemblyDays: 3,
          safetyMarginDays: 2,
          whatsappPhone: null,
          currency: 'BRL',
          statusWorkflow: { id: id(1), version: 1, name: 'Encomenda com montagem' },
          version: 1,
        },
      }
      const answer = answers[path]
      if (answer === undefined) return route.fulfill({ status: 404, contentType: 'application/problem+json', body: JSON.stringify({ type: 'about:blank', title: 'Não achei', status: 404, code: 'not_found' }) })
      return route.fulfill({ json: answer })
    })
  }

  test.describe('an owner', () => {
    test.beforeEach(async ({ page }) => {
      await mockCatalog(page, 'owner')
      await page.goto('/catalogo')
      await expect(page.getByRole('heading', { name: 'Catálogo', exact: true })).toBeVisible()
      await expect(page.getByRole('button', { name: itemName })).toBeVisible()
    })

    test('Itens are cards with cost and margin, and every control fits a thumb', async ({ page }) => {
      await expectNoSideScroll(page)
      const rows = page.locator('tbody tr')
      await expect(rows).toHaveCount(2)
      await expect(rows.first()).toHaveCSS('display', 'grid')

      // The same fields as the table, with the owner's Custo and Margem.
      const card = rows.first()
      for (const label of ['Categoria', 'Origem', 'Fornecedor', 'Prazo', 'Custo', 'Preço', 'Margem', 'No portal']) {
        await expect(card.locator(`td[data-label="${label}"]`), label).toHaveCount(1)
      }
      await expect(card.locator('td[data-label="Custo"]')).toContainText('315,00')
      await expect(card.locator('td[data-label="Margem"]')).toContainText('135,00')
      await expect(card.locator('td[data-label="Preço"]')).toContainText('450,00')
      // The header row stays for screen readers.
      await expect(page.getByRole('columnheader', { name: 'Custo' })).toHaveCount(1)

      // A long name wraps inside the card; the controls keep their names and 48 px.
      const name = page.getByRole('button', { name: itemName })
      await expectThumbSized(
        name,
        page.getByRole('switch', { name: `Mostrar ${itemName} na página da loja` }),
        page.getByRole('tab', { name: 'Itens' }),
        page.getByRole('tab', { name: 'Kits' }),
        page.getByLabel('Buscar item'),
        page.getByLabel('Filtrar por categoria'),
        page.getByLabel('Mostrar arquivados'),
        page.getByRole('button', { name: 'Categorias' }),
        page.getByRole('button', { name: 'Campos' }),
        page.getByRole('button', { name: 'Novo item' }),
      )
      const switchBox = await page.getByRole('switch', { name: `Mostrar ${itemName} na página da loja` }).boundingBox()
      expect(switchBox?.width).toBeGreaterThanOrEqual(48)
      // The toolbar's three buttons and the tab row stay inside the screen.
      for (const control of [page.getByRole('button', { name: 'Novo item' }), page.getByRole('tab', { name: 'Kits' })]) {
        const box = await control.boundingBox()
        expect((box?.x ?? 0) + (box?.width ?? 0)).toBeLessThanOrEqual(390)
      }
    })

    test('Kits are cards with the margin', async ({ page }) => {
      await page.getByRole('tab', { name: 'Kits' }).click()
      const name = page.getByRole('button', { name: 'Kit Scrambler CB 500X completo para a cidade' })
      await expect(name).toBeVisible()
      await expectNoSideScroll(page)
      const card = page.locator('tbody tr').first()
      await expect(card).toHaveCSS('display', 'grid')
      for (const label of ['Itens', 'Preço do kit', 'Soma avulsa', 'Margem', 'Prazo', 'No portal']) {
        await expect(card.locator(`td[data-label="${label}"]`), label).toHaveCount(1)
      }
      await expect(card.locator('td[data-label="Margem"]')).toContainText('380,00')
      await expectThumbSized(
        name,
        page.getByRole('switch', { name: 'Mostrar Kit Scrambler CB 500X completo para a cidade na página da loja' }),
        page.getByLabel('Mostrar arquivados'),
        page.getByRole('button', { name: 'Novo kit' }),
      )
    })

    test('the item sheet: fits, its buttons stay on screen, the photos have thumb-sized buttons', async ({ page }) => {
      await page.getByRole('button', { name: itemName }).click()
      const sheet = await expectSheetFits(page, 'Editar item')
      await expect(sheet.getByRole('img', { name: 'Foto 1' })).toBeVisible()

      // The form is far taller than the screen, and the buttons are on it anyway.
      const body = sheet.locator('.enc-dialog__body')
      expect(await body.evaluate((element) => element.scrollHeight - element.clientHeight)).toBeGreaterThan(400)
      const save = sheet.getByRole('button', { name: 'Salvar', exact: true })
      // The sheet's own buttons: "Novo fornecedor" has a "Cancelar" of its own.
      const cancel = sheet.locator('.enc-sheet-actions').getByRole('button', { name: 'Cancelar', exact: true })
      const archive = sheet.getByRole('button', { name: 'Arquivar', exact: true })
      await expectThumbSized(save, cancel, archive)
      for (const button of [save, cancel, archive]) await expectOnScreen(button)

      // Scrolled to the end, to the middle: still there, never covering the last field.
      await body.evaluate((element) => element.scrollTo(0, element.scrollHeight))
      for (const button of [save, cancel, archive]) await expectOnScreen(button)
      const last = sheet.getByLabel('Mostrar na página da loja')
      await expectOnScreen(last)
      const lastBox = await last.boundingBox()
      const saveBox = await save.boundingBox()
      expect((lastBox?.y ?? 0) + (lastBox?.height ?? 0)).toBeLessThanOrEqual(saveBox?.y ?? 0)
      await body.evaluate((element) => element.scrollTo(0, element.scrollHeight / 2))
      for (const button of [save, cancel]) await expectOnScreen(button)

      // Photos: thumbnails side by side, the cover marked, and 48 px buttons with their names.
      await sheet.getByRole('img', { name: 'Foto 2' }).scrollIntoViewIfNeeded()
      await expect(sheet.getByText('Capa', { exact: true })).toBeVisible()
      await expectThumbSized(sheet.getByRole('button', { name: 'Usar como capa, foto 2' }), sheet.getByRole('button', { name: 'Remover foto 1' }), sheet.getByRole('button', { name: 'Remover foto 2' }), sheet.getByText('Adicionar fotos'))
      const first = await sheet.getByRole('img', { name: 'Foto 1' }).boundingBox()
      const second = await sheet.getByRole('img', { name: 'Foto 2' }).boundingBox()
      expect(second?.y).toBe(first?.y)
      expect(first?.width).toBeGreaterThan(100)

      // The camera or the gallery, the shell decides: no `capture`, any image.
      const input = sheet.getByLabel('Adicionar fotos')
      await expect(input).toHaveAttribute('type', 'file')
      await expect(input).toHaveAttribute('accept', 'image/*')
      await expect(input).not.toHaveAttribute('capture')

      // The supplier, backup and options editors fit in one column, with 48 px fields.
      await sheet.getByRole('group', { name: 'Fornecedor reserva' }).scrollIntoViewIfNeeded()
      await expectThumbSized(sheet.getByLabel('Fornecedor principal'), sheet.getByLabel('Prazo do fornecedor (dias)').first(), sheet.getByLabel('Custo', { exact: true }).first(), sheet.getByRole('button', { name: 'Remover fornecedor reserva' }), sheet.getByRole('button', { name: 'Nova opção' }))
      await sheet.getByLabel('Fornecedor principal').selectOption({ label: 'Novo fornecedor' })
      await expect(sheet.getByRole('group', { name: 'Novo fornecedor' })).toBeVisible()
      await expectSheetFits(page, 'Editar item')
      await expectThumbSized(sheet.getByRole('button', { name: 'Adicionar fornecedor', exact: true }), sheet.getByLabel('WhatsApp do fornecedor'))
      await sheet.getByRole('button', { name: 'Nova opção' }).click()
      await expectSheetFits(page, 'Editar item')
      await expectThumbSized(sheet.getByRole('button', { name: 'Remover escolha' }).first(), sheet.getByRole('button', { name: 'Adicionar escolha' }).first(), sheet.getByLabel('Obrigatória').first())

      // An edit then "Cancelar" asks first, and the question is on screen with its answers.
      await sheet.getByLabel('Nome', { exact: true }).fill('Outro nome')
      await cancel.click()
      const question = sheet.getByRole('group', { name: 'Descartar as alterações' })
      await expect(question).toBeVisible()
      await expectOnScreen(question.getByRole('button', { name: 'Continuar editando' }))
      await expectThumbSized(question.getByRole('button', { name: 'Continuar editando' }), question.getByRole('button', { name: 'Descartar' }))
      await expectSheetFits(page, 'Editar item')
      await question.getByRole('button', { name: 'Descartar' }).click()
      await expect(sheet).toBeHidden()
    })

    test('the new item sheet fits and its buttons are on screen', async ({ page }) => {
      await page.getByRole('button', { name: 'Novo item' }).first().click()
      const sheet = await expectSheetFits(page, 'Novo item')
      await expect(sheet.getByText('Salve o item para adicionar fotos.')).toBeVisible()
      await expectOnScreen(sheet.getByRole('button', { name: 'Salvar', exact: true }))
      await expectOnScreen(sheet.getByRole('button', { name: 'Cancelar', exact: true }))
      await sheet.getByRole('button', { name: 'Cancelar', exact: true }).click()
      await expect(sheet).toBeHidden()
    })

    test('the kit sheet: fits, swap choices in one column, buttons on screen', async ({ page }) => {
      await page.getByRole('tab', { name: 'Kits' }).click()
      await page.getByRole('button', { name: 'Kit Scrambler CB 500X completo para a cidade' }).click()
      const sheet = await expectSheetFits(page, 'Editar kit')
      await expect(sheet.getByLabel('Totais do kit')).toContainText('Margem')

      const body = sheet.locator('.enc-dialog__body')
      expect(await body.evaluate((element) => element.scrollHeight - element.clientHeight)).toBeGreaterThan(400)
      const save = sheet.getByRole('button', { name: 'Salvar', exact: true })
      const cancel = sheet.getByRole('button', { name: 'Cancelar', exact: true })
      await expectThumbSized(save, cancel, sheet.getByRole('button', { name: 'Arquivar', exact: true }))
      await expectOnScreen(save)
      await expectOnScreen(cancel)
      await body.evaluate((element) => element.scrollTo(0, element.scrollHeight))
      await expectOnScreen(save)

      // The item row, its swap checkboxes and the picker.
      const row = sheet.getByRole('group', { name: itemName })
      await row.scrollIntoViewIfNeeded()
      await expectThumbSized(row.getByLabel('Quantidade'), row.getByLabel('Cliente pode trocar'), row.getByLabel(/Banco Premium/), row.getByRole('button', { name: `Remover ${itemName} do kit` }))
      await expectThumbSized(sheet.getByLabel('Item a adicionar'), sheet.getByRole('button', { name: 'Adicionar item' }))
      await expectThumbSized(sheet.getByLabel(/Manter o desconto/), sheet.getByLabel(/Cobrar a soma dos itens avulsos/))
      await expectSheetFits(page, 'Editar kit')
    })

    test('the Categorias and Campos sheets fit and keep "Fechar" on screen', async ({ page }) => {
      await page.getByRole('button', { name: 'Categorias' }).click()
      const categories = await expectSheetFits(page, 'Categorias')
      await expect(categories.getByText(bancos.name)).toBeVisible()
      await expectThumbSized(
        categories.getByRole('button', { name: `Mover ${guidoes.name} para cima` }),
        categories.getByRole('button', { name: `Renomear ${bancos.name}` }),
        categories.getByRole('button', { name: `Excluir ${bancos.name}` }),
        categories.getByLabel('Nome da categoria'),
        categories.getByRole('button', { name: 'Nova categoria' }),
        categories.getByRole('button', { name: 'Fechar' }),
      )
      await expectOnScreen(categories.getByRole('button', { name: 'Fechar' }))
      await categories.getByRole('button', { name: `Renomear ${bancos.name}` }).click()
      await expectSheetFits(page, 'Categorias')
      await expectThumbSized(categories.getByLabel(`Novo nome de ${bancos.name}`), categories.getByRole('button', { name: 'Salvar' }))
      await categories.getByRole('button', { name: 'Fechar' }).click()
      await expect(categories).toBeHidden()

      await page.getByRole('button', { name: 'Campos' }).click()
      const attributes = await expectSheetFits(page, 'Campos dos itens')
      await expectThumbSized(attributes.getByLabel('Nome do campo'), attributes.getByLabel('Valor 1'), attributes.getByRole('button', { name: 'Remover valor CB 500X' }), attributes.getByRole('button', { name: 'Adicionar valor' }), attributes.getByRole('button', { name: 'Salvar campo' }), attributes.getByRole('button', { name: 'Novo campo' }), attributes.getByRole('button', { name: 'Fechar' }))
      await expectOnScreen(attributes.getByRole('button', { name: 'Fechar' }))
      // A value in use asks before it goes: the question and its answers fit.
      await attributes.getByRole('button', { name: 'Remover valor CB 500X' }).click()
      await expect(attributes.getByRole('group', { name: 'Confirmar remoção' })).toBeVisible()
      await expectSheetFits(page, 'Campos dos itens')
      await attributes.getByRole('button', { name: 'Novo campo' }).click()
      await expectSheetFits(page, 'Campos dos itens')
      await expectOnScreen(attributes.getByRole('button', { name: 'Fechar' }))
      await attributes.getByRole('button', { name: 'Fechar' }).click()
      await expect(attributes).toBeHidden()
    })
  })

  test.describe('an admin', () => {
    test.beforeEach(async ({ page }) => {
      await mockCatalog(page, 'admin')
      await page.goto('/catalogo')
      await expect(page.getByRole('button', { name: itemName })).toBeVisible()
    })

    test('sees no cost or margin on the cards or in the sheets', async ({ page }) => {
      await expectNoSideScroll(page)
      const card = page.locator('tbody tr').first()
      await expect(card.locator('td[data-label="Preço"]')).toContainText('450,00')
      await expect(card.locator('td[data-label="Custo"]')).toHaveCount(0)
      await expect(card.locator('td[data-label="Margem"]')).toHaveCount(0)
      await expect(page.getByRole('columnheader', { name: 'Custo' })).toHaveCount(0)

      await page.getByRole('button', { name: itemName }).click()
      const sheet = await expectSheetFits(page, 'Editar item')
      await expect(sheet.getByLabel('Prazo do fornecedor (dias)').first()).toHaveValue('15')
      await expect(sheet.getByLabel('Custo', { exact: true })).toHaveCount(0)
      await expectOnScreen(sheet.getByRole('button', { name: 'Salvar', exact: true }))
      await sheet.getByRole('button', { name: 'Cancelar', exact: true }).click()

      await page.getByRole('tab', { name: 'Kits' }).click()
      await expect(page.locator('tbody tr').first().locator('td[data-label="Margem"]')).toHaveCount(0)
      await page.getByRole('button', { name: 'Kit Scrambler CB 500X completo para a cidade' }).click()
      const kit = await expectSheetFits(page, 'Editar kit')
      await expect(kit.getByLabel('Totais do kit')).not.toContainText('Margem')
    })
  })
})

test.describe('against the real stack', () => {
  test.skip(!realStack, 'set E2E_BASE_URL (and E2E_MAILPIT_URL) to run against `make stack-up`')

  test.beforeEach(() => {
    expect(mailpitUrl, 'E2E_MAILPIT_URL must name Mailpit').not.toBe('')
  })

  test.setTimeout(150_000)

  async function openCatalog(page: Page): Promise<void> {
    await openMenu(page)
    await page.getByRole('navigation', { name: 'Menu principal' }).getByRole('link', { name: 'Catálogo' }).click()
    await expect(page.getByRole('heading', { name: 'Catálogo', exact: true })).toBeVisible()
    await expect(page.getByRole('navigation', { name: 'Menu principal' })).toBeHidden()
  }

  async function saveSheet(page: Page, notice: string): Promise<void> {
    await page.getByRole('button', { name: 'Salvar', exact: true }).click()
    await expect(page.getByRole('status').filter({ hasText: notice })).toBeVisible()
  }

  test('an owner makes an item with a supplier and a photo, and a kit with a swap; an admin sees no costs', async ({ page, request, browser }) => {
    const owner = unique('fonecat')
    const invitee = unique('foneadm')

    await signUp(page, 'Elisa Prado', owner.email)
    await createStore(page, 'Catálogo Fone', owner.slug)
    await openCatalog(page)
    await expectNoSideScroll(page)

    // A new item with a new supplier, a lead time and a cost: every step is on a phone.
    await page.getByRole('button', { name: 'Novo item' }).first().click()
    const sheet = await expectSheetFits(page, 'Novo item')
    await sheet.getByLabel('Nome', { exact: true }).fill('Escapamento esportivo')
    await sheet.getByLabel('Preço', { exact: true }).fill('450,00')
    await sheet.getByLabel('Fornecedor principal', { exact: true }).selectOption({ label: 'Novo fornecedor' })
    await sheet.getByLabel('Nome do fornecedor').fill('Escapes do Sul')
    await sheet.getByLabel('WhatsApp do fornecedor').fill('(11) 98765-4321')
    await sheet.getByRole('button', { name: 'Adicionar fornecedor', exact: true }).click()
    await expect(sheet.getByLabel('Fornecedor principal', { exact: true })).toHaveValue(/.+/)
    await sheet.getByLabel('Prazo do fornecedor (dias)').fill('5')
    await sheet.getByLabel('Custo', { exact: true }).fill('280,00')
    await expectOnScreen(sheet.getByRole('button', { name: 'Salvar', exact: true }))
    await saveSheet(page, 'Item salvo.')
    await expectNoSideScroll(page)

    // The card shows the supplier, the lead time, the cost and the margin to the owner.
    const card = page.getByRole('row', { name: /Escapamento esportivo/ })
    await expect(card).toContainText('Escapes do Sul')
    await expect(card).toContainText('5 dias')
    await expect(card.locator('td[data-label="Custo"]')).toContainText('280,00')
    await expect(card.locator('td[data-label="Margem"]')).toBeVisible()

    // The photo goes to Silo through the phone's file chooser, then reads back.
    await card.getByRole('button', { name: 'Escapamento esportivo' }).click()
    const editing = await expectSheetFits(page, 'Editar item')
    await editing.getByLabel('Adicionar fotos').setInputFiles({ name: 'escapamento.png', mimeType: 'image/png', buffer: png })
    const photo = editing.getByRole('img', { name: 'Foto 1' })
    await expect(photo).toBeVisible()
    await expect(photo).toHaveAttribute('src', /^https?:\/\/[^/]+\/encomendo-photos\/t\/[\w-]+\/items\/[\w-]+\/[\w-]+/)
    await expect.poll(() => photo.evaluate((img: HTMLImageElement) => img.naturalWidth)).toBe(1)
    await expect(editing.getByText('Capa', { exact: true })).toBeVisible()
    await expectSheetFits(page, 'Editar item')
    await editing.getByRole('button', { name: 'Cancelar', exact: true }).click()

    // A second item, the alternative.
    await page.getByRole('button', { name: 'Novo item' }).first().click()
    await page.getByLabel('Nome', { exact: true }).fill('Escapamento de série')
    await page.getByLabel('Preço', { exact: true }).fill('300,00')
    await page.getByLabel('Fornecedor principal', { exact: true }).selectOption({ label: 'Escapes do Sul' })
    await page.getByLabel('Prazo do fornecedor (dias)').fill('2')
    await page.getByLabel('Custo', { exact: true }).fill('180,00')
    await saveSheet(page, 'Item salvo.')
    await expect(page.getByRole('row', { name: /Escapamento de série/ })).toContainText('2 dias')

    // A kit with the first item, which the client may swap for the second.
    await page.getByRole('tab', { name: 'Kits' }).click()
    await page.getByRole('button', { name: 'Novo kit' }).first().click()
    const kitSheet = await expectSheetFits(page, 'Novo kit')
    await kitSheet.getByLabel('Nome', { exact: true }).fill('Kit esportivo')
    await kitSheet.getByLabel('Preço do kit').fill('420,00')
    await kitSheet.getByLabel('Item a adicionar').selectOption({ label: 'Escapamento esportivo' })
    await kitSheet.getByRole('button', { name: 'Adicionar item' }).click()
    const kitRow = kitSheet.getByRole('group', { name: 'Escapamento esportivo' })
    await kitRow.getByLabel('Cliente pode trocar').check()
    await kitRow.getByLabel(/Escapamento de série/).check()
    await expectSheetFits(page, 'Novo kit')
    await saveSheet(page, 'Kit salvo.')
    await expect(page.getByRole('row', { name: /Kit esportivo/ })).toBeVisible()

    // The saved kit stays open, keeps the swap, and the owner sees the margin.
    const saved = await expectSheetFits(page, 'Editar kit')
    await expect(saved.getByRole('group', { name: 'Escapamento esportivo' }).getByLabel('Cliente pode trocar')).toBeChecked()
    await expect(saved.getByRole('group', { name: 'Escapamento esportivo' }).getByLabel(/Escapamento de série/)).toBeChecked()
    await expect(saved.getByLabel('Totais do kit')).toContainText('Margem')
    await saved.getByRole('button', { name: 'Cancelar', exact: true }).click()

    // An admin is invited through Equipe and sees the item with no Custo and no Margem.
    await openMenu(page)
    await page.getByRole('navigation', { name: 'Menu principal' }).getByRole('link', { name: 'Equipe' }).click()
    await page.getByRole('form', { name: 'Convidar para a equipe' }).getByLabel('E-mail').fill(invitee.email)
    await page.getByLabel('Papel', { exact: true }).selectOption({ label: 'Admin' })
    await page.getByRole('button', { name: 'Enviar convite', exact: true }).click()
    await expect(page.getByText(`Convite enviado para ${invitee.email}.`)).toBeVisible()

    const link = /http\S*\/convite#token=[\w-]+/.exec(await latestEmail(request, invitee.email))?.[0]
    expect(link, 'the invitation link in the email').toBeDefined()
    const url = new URL(link ?? '')
    const guestContext = await browser.newContext({ ...test.info().project.use, baseURL: process.env.E2E_BASE_URL })
    try {
      const guest = await guestContext.newPage()
      await guest.goto(new URL(url.pathname + url.hash, process.env.E2E_BASE_URL).toString())
      await guest.getByLabel('Nome').fill('Fábio Admin')
      await guest.getByLabel('Senha').fill(password)
      await guest.getByRole('button', { name: 'Aceitar convite' }).click()
      await expect(guest).toHaveURL(/\/hoje$/)
      await openCatalog(guest)
      await expectNoSideScroll(guest)

      const adminCard = guest.getByRole('row', { name: /Escapamento esportivo/ })
      await expect(adminCard).toContainText('Escapes do Sul')
      await expect(adminCard).toContainText('5 dias')
      await expect(adminCard.locator('td[data-label="Preço"]')).toBeVisible()
      await expect(adminCard.locator('td[data-label="Custo"]')).toHaveCount(0)
      await expect(adminCard.locator('td[data-label="Margem"]')).toHaveCount(0)

      await adminCard.getByRole('button', { name: 'Escapamento esportivo' }).click()
      const adminSheet = await expectSheetFits(guest, 'Editar item')
      await expect(adminSheet.getByLabel('Prazo do fornecedor (dias)')).toHaveValue('5')
      await expect(adminSheet.getByLabel('Custo', { exact: true })).toHaveCount(0)
      await expect(adminSheet.getByRole('img', { name: 'Foto 1' })).toBeVisible()
      await adminSheet.getByRole('button', { name: 'Cancelar', exact: true }).click()

      await guest.getByRole('tab', { name: 'Kits' }).click()
      await expect(guest.locator('td[data-label="Margem"]')).toHaveCount(0)
      await guest.getByRole('button', { name: 'Kit esportivo' }).click()
      await expect(guest.getByLabel('Totais do kit')).not.toContainText('Margem')
    } finally {
      await guestContext.close()
    }
  })
})
