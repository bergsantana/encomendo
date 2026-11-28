import { expect, test, type Page } from '@playwright/test'

import { apiSession, createStore, latestEmail, mailpitUrl, password, signUp, unique } from './support'

// Catálogo against the real stack (see admin.spec.ts for how to run it): an
// owner makes an item with a new supplier, a lead time, a cost and a photo
// that really goes to Silo, then a kit with a swappable item. An admin opens
// the same catalog and sees no Custo and no Margem (ADR 0025).
const realStack = process.env.E2E_BASE_URL !== undefined && process.env.E2E_BASE_URL !== ''

// A 1x1 PNG: small, and a real image so the upload check accepts it.
const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==', 'base64')

test.describe('Catálogo against the real stack', () => {
  test.skip(!realStack, 'set E2E_BASE_URL (and E2E_MAILPIT_URL) to run against `make stack-up`')

  test.beforeEach(() => {
    expect(mailpitUrl, 'E2E_MAILPIT_URL must name Mailpit').not.toBe('')
  })

  async function openCatalog(page: Page): Promise<void> {
    await page.getByRole('navigation', { name: 'Menu principal' }).getByRole('link', { name: 'Catálogo' }).click()
    await expect(page.getByRole('heading', { name: 'Catálogo', exact: true })).toBeVisible()
  }

  async function saveDrawer(page: Page, notice: string): Promise<void> {
    await page.getByRole('button', { name: 'Salvar', exact: true }).click()
    await expect(page.getByRole('status').filter({ hasText: notice })).toBeVisible()
  }

  test.setTimeout(120_000)

  test('an owner makes an item with a supplier, a photo and a kit; an admin sees no costs', async ({ page, request, browser }) => {
    const owner = unique('catalogo')
    const invitee = unique('admin')

    await signUp(page, 'Elisa Prado', owner.email)
    await createStore(page, 'Catálogo Teste', owner.slug)
    await openCatalog(page)

    // A new item with a new supplier, a lead time and a cost.
    await page.getByRole('button', { name: 'Novo item' }).first().click()
    await page.getByLabel('Nome', { exact: true }).fill('Escapamento esportivo')
    await page.getByLabel('Preço', { exact: true }).fill('450,00')
    await page.getByLabel('Fornecedor principal', { exact: true }).selectOption({ label: 'Novo fornecedor' })
    await page.getByLabel('Nome do fornecedor').fill('Escapes do Sul')
    await page.getByLabel('WhatsApp do fornecedor').fill('(11) 98765-4321')
    await page.getByRole('button', { name: 'Adicionar fornecedor', exact: true }).click()
    await expect(page.getByLabel('Fornecedor principal', { exact: true })).toHaveValue(/.+/)
    await page.getByLabel('Prazo do fornecedor (dias)').fill('5')
    await page.getByLabel('Custo', { exact: true }).fill('280,00')
    await saveDrawer(page, 'Item salvo.')

    // The table shows the supplier, the lead time, the cost and the margin to the owner.
    const row = page.getByRole('row', { name: /Escapamento esportivo/ })
    await expect(row).toContainText('Escapes do Sul')
    await expect(row).toContainText('5 dias')
    await expect(page.getByRole('columnheader', { name: 'Custo' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: 'Margem' })).toBeVisible()

    // The photo goes to Silo with a signed URL, then reads back through another one.
    await row.getByRole('button', { name: 'Escapamento esportivo' }).click()
    await page.getByLabel('Adicionar fotos').setInputFiles({ name: 'escapamento.png', mimeType: 'image/png', buffer: png })
    const photo = page.getByRole('img', { name: 'Foto 1' })
    await expect(photo).toBeVisible()
    // The thumbnail comes from the photo store (Silo, bucket path t/{tenant}/items/...), not from the admin origin.
    await expect(photo).toHaveAttribute('src', /^https?:\/\/[^/]+\/encomendo-photos\/t\/[\w-]+\/items\/[\w-]+\/[\w-]+/)
    expect(new URL((await photo.getAttribute('src')) ?? '').origin).not.toBe(new URL(process.env.E2E_BASE_URL ?? '').origin)
    await expect.poll(() => photo.evaluate((img: HTMLImageElement) => img.naturalWidth)).toBe(1)
    await expect(page.getByText('Capa', { exact: true })).toBeVisible()
    await page.getByRole('button', { name: 'Cancelar' }).click()

    // A second item, the alternative.
    await page.getByRole('button', { name: 'Novo item' }).first().click()
    await page.getByLabel('Nome', { exact: true }).fill('Escapamento de série')
    await page.getByLabel('Preço', { exact: true }).fill('300,00')
    await page.getByLabel('Fornecedor principal', { exact: true }).selectOption({ label: 'Escapes do Sul' })
    await page.getByLabel('Prazo do fornecedor (dias)').fill('2')
    await page.getByLabel('Custo', { exact: true }).fill('180,00')
    await saveDrawer(page, 'Item salvo.')
    await expect(page.getByRole('row', { name: /Escapamento de série/ })).toContainText('2 dias')

    // A kit with the first item, which the client may swap for the second.
    await page.getByRole('tab', { name: 'Kits' }).click()
    await page.getByRole('button', { name: 'Novo kit' }).first().click()
    await page.getByLabel('Nome', { exact: true }).fill('Kit esportivo')
    await page.getByLabel('Preço do kit').fill('420,00')
    await page.getByLabel('Item a adicionar').selectOption({ label: 'Escapamento esportivo' })
    await page.getByRole('button', { name: 'Adicionar item' }).click()
    const kitRow = page.getByRole('group', { name: 'Escapamento esportivo' })
    await kitRow.getByLabel('Cliente pode trocar').check()
    await kitRow.getByLabel(/Escapamento de série/).check()
    await saveDrawer(page, 'Kit salvo.')
    await expect(page.getByRole('row', { name: /Kit esportivo/ })).toBeVisible()

    // The saved kit stays open, keeps the swap, and the owner sees the margin.
    await expect(page.getByRole('dialog', { name: 'Editar kit' })).toBeVisible()
    await expect(page.getByRole('group', { name: 'Escapamento esportivo' }).getByLabel('Cliente pode trocar')).toBeChecked()
    await expect(page.getByRole('group', { name: 'Escapamento esportivo' }).getByLabel(/Escapamento de série/)).toBeChecked()
    await expect(page.getByLabel('Totais do kit')).toContainText('Margem')
    await page.getByRole('button', { name: 'Cancelar' }).click()

    // An admin is invited through Equipe and sees the item with no Custo and no Margem.
    await page.getByRole('navigation', { name: 'Menu principal' }).getByRole('link', { name: 'Equipe' }).click()
    await page.getByRole('form', { name: 'Convidar para a equipe' }).getByLabel('E-mail').fill(invitee.email)
    await page.getByLabel('Papel', { exact: true }).selectOption({ label: 'Admin' })
    await page.getByRole('button', { name: 'Enviar convite', exact: true }).click()
    await expect(page.getByText(`Convite enviado para ${invitee.email}.`)).toBeVisible()

    const link = /http\S*\/convite#token=[\w-]+/.exec(await latestEmail(request, invitee.email))?.[0]
    expect(link, 'the invitation link in the email').toBeDefined()
    const url = new URL(link ?? '')
    const guestContext = await browser.newContext({ baseURL: process.env.E2E_BASE_URL, locale: 'pt-BR' })
    try {
      const guest = await guestContext.newPage()
      await guest.goto(new URL(url.pathname + url.hash, process.env.E2E_BASE_URL).toString())
      await guest.getByLabel('Nome').fill('Fábio Admin')
      await guest.getByLabel('Senha').fill(password)
      await guest.getByRole('button', { name: 'Aceitar convite' }).click()
      await expect(guest).toHaveURL(/\/hoje$/)
      await openCatalog(guest)

      const adminRow = guest.getByRole('row', { name: /Escapamento esportivo/ })
      await expect(adminRow).toContainText('Escapes do Sul')
      await expect(adminRow).toContainText('5 dias')
      await expect(guest.getByRole('columnheader', { name: 'Preço' })).toBeVisible()
      await expect(guest.getByRole('columnheader', { name: 'Custo' })).toHaveCount(0)
      await expect(guest.getByRole('columnheader', { name: 'Margem' })).toHaveCount(0)

      await adminRow.getByRole('button', { name: 'Escapamento esportivo' }).click()
      await expect(guest.getByLabel('Prazo do fornecedor (dias)')).toHaveValue('5')
      await expect(guest.getByLabel('Custo', { exact: true })).toHaveCount(0)
      await expect(guest.getByRole('img', { name: 'Foto 1' })).toBeVisible()
      await guest.getByRole('button', { name: 'Cancelar' }).click()

      await guest.getByRole('tab', { name: 'Kits' }).click()
      await expect(guest.getByRole('columnheader', { name: 'Margem' })).toHaveCount(0)
      await guest.getByRole('button', { name: 'Kit esportivo' }).click()
      await expect(guest.getByLabel('Totais do kit')).not.toContainText('Margem')
    } finally {
      await guestContext.close()
    }
  })

  // The half of "created on one app, shown on the other" that the web can
  // prove: an item written through the API alone (what the Android app
  // sends) shows in the web list with its supplier and lead time.
  test('an item created through the API shows in the web list', async ({ page, request }) => {
    const owner = unique('apiitem')
    // Node cannot resolve *.localhost the way Chromium does, so call the stack by
    // address and keep the admin Host header nginx routes on.
    const admin = new URL(process.env.E2E_BASE_URL ?? '')
    const api = `http://127.0.0.1:${admin.port}`
    const json = { 'content-type': 'application/json', host: admin.host }

    const signed = await request.post(`${api}/auth/signup`, { headers: json, data: { name: 'Gil Prado', email: owner.email, password } })
    expect(signed.status()).toBe(201)
    const first = (await signed.json()) as { accessToken: string }
    const created = await request.post(`${api}/api/v1/admin/tenants`, {
      headers: { ...json, authorization: `Bearer ${first.accessToken}` },
      data: { name: 'Loja pela API', slug: owner.slug },
    })
    expect(created.status()).toBe(201)

    // A new login has the store selected.
    const session = await apiSession(request, owner.email)
    const supplier = await session.send<{ id: string }>('POST', '/api/v1/admin/suppliers', { name: 'Fornecedor da API', whatsappPhone: '5511987654321' })
    await session.send('POST', '/api/v1/admin/catalog/items', {
      name: 'Item pela API',
      description: '',
      categoryId: null,
      sourcingMode: 'to_order',
      priceCents: 12000,
      mainSupplier: { supplierId: supplier.id, leadDays: 4, costCents: 7000 },
      attributeValueIds: [],
      options: [],
      published: false,
    })

    await page.goto('/')
    await page.getByLabel('E-mail').fill(owner.email)
    await page.getByLabel('Senha').fill(password)
    await page.getByRole('button', { name: 'Entrar' }).click()
    await expect(page).toHaveURL(/\/hoje$/)
    await openCatalog(page)

    const row = page.getByRole('row', { name: /Item pela API/ })
    await expect(row).toContainText('Fornecedor da API')
    await expect(row).toContainText('4 dias')
    await expect(row).toContainText('120,00')
    await expect(row).toContainText('70,00')
  })
})
