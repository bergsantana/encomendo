import { expect, test, type Locator, type Page } from '@playwright/test'

import {
  apiSession,
  createStore,
  expectNoSideScroll,
  expectNoWhatsappRequests,
  expectSheetFits,
  expectThumbSized,
  itemPhoto,
  mailpitUrl,
  openMenu,
  placeOrder,
  seedBoughtItem,
  signUp,
  unique,
  watchWhatsapp,
} from './support'

// Compras and Receber itens at phone size (ADR 0029), in the `phone` project of
// playwright.config.ts: 390 x 844, touch. The path of compras.spec.ts runs here
// against the real stack (`make stack-up`, E2E_BASE_URL and E2E_MAILPIT_URL):
// the deposit, A comprar, Ver mensagem, Enviar pelo WhatsApp, the pedido de
// compra with its dialogs, Receber itens with a defective item and a photo,
// the replacement bought again, and the order moved into Montagem. Every screen
// must fit the width, every control a thumb hits must be 48 px tall and the
// dialogs are full-screen sheets. Receber itens is the workshop screen: 18 px
// text, 56 px targets and a 64 px "Salvar recebimento". The admin's missing
// costs are checked by compras.spec.ts, so this run signs up one account only
// (the API allows 10 sign-ups an hour per IP, ADR 0021: clear the counts before
// a full run, see admin.spec.ts).
const realStack = process.env.E2E_BASE_URL !== undefined && process.env.E2E_BASE_URL !== ''

/** expectWorkshopSized checks Receber itens' sizes: 56 px targets (the main button 64 px) and 18 px text. */
async function expectWorkshopSized(page: Page, targets: Locator[], text: Locator[]): Promise<void> {
  await expectThumbSized(56, ...targets)
  for (const element of text) {
    const size = await element.evaluate((node) => Number.parseFloat(getComputedStyle(node).fontSize))
    expect(size, 'text size in px').toBeGreaterThanOrEqual(18)
  }
  const save = page.getByRole('button', { name: 'Salvar recebimento' })
  const height = await save.evaluate((node) => node.getBoundingClientRect().height)
  expect(height, 'Salvar recebimento is 64 px tall').toBeGreaterThanOrEqual(64)
}

test.describe('Compras and Receber itens at phone size, against the real stack', () => {
  test.skip(!realStack, 'set E2E_BASE_URL (and E2E_MAILPIT_URL) to run against `make stack-up`')

  test.beforeEach(() => {
    expect(mailpitUrl, 'E2E_MAILPIT_URL must name Mailpit').not.toBe('')
  })

  test.setTimeout(180_000)

  /** goTo opens a menu entry through the drawer and checks the drawer closed. */
  async function goTo(page: Page, entry: string, heading: string): Promise<void> {
    await openMenu(page)
    await page.getByRole('navigation', { name: 'Menu principal' }).getByRole('link', { name: entry, exact: true }).click()
    await expect(page.getByRole('heading', { name: heading, exact: true })).toBeVisible()
    await expect(page.getByRole('navigation', { name: 'Menu principal' })).toBeHidden()
  }

  test('an owner buys, receives a defective item with a photo, buys it again and receives it', async ({ page, request }) => {
    const owner = unique('fonecompras')
    const whatsapp = await watchWhatsapp(page)

    await signUp(page, 'Elisa Prado', owner.email)
    await createStore(page, 'Compras Fone', owner.slug)
    const api = await apiSession(request, owner.email)
    const bought = await seedBoughtItem(api)
    const first = await placeOrder(api, bought.itemId, `${owner.slug}-1`, false)

    // Compras with nothing to buy: the tabs are thumb-sized and fit.
    await goTo(page, 'Compras', 'Compras')
    await expect(page.getByText('Nada para comprar agora.')).toBeVisible()
    await expectNoSideScroll(page)
    await expectThumbSized(48, page.getByRole('tab', { name: 'A comprar' }), page.getByRole('tab', { name: 'Enviadas' }), page.getByRole('tab', { name: 'Receber itens' }), page.getByRole('tab', { name: 'Fornecedores' }))

    // Pedido: the deposit; the bought item has no purchasing buttons, only the way into Compras.
    await goTo(page, 'Pedidos', 'Pedidos')
    await page.getByRole('link', { name: first.number }).click()
    await expect(page.getByRole('heading', { name: 'Pedido P-0001' })).toBeVisible()
    await expect(page.getByRole('button', { name: /Marcar como enviada ao fornecedor|Marcar como em trânsito|Tudo certo|Com defeito/ })).toHaveCount(0)
    await expectThumbSized(48, page.getByRole('link', { name: `Ver em Compras: ${bought.item}` }))
    await page.getByRole('button', { name: 'Registrar pagamento' }).click()
    const payment = await expectSheetFits(page, 'Registrar pagamento')
    await payment.getByLabel('Forma').selectOption({ label: 'Pix' })
    await payment.getByRole('button', { name: 'Registrar', exact: true }).click()
    await expect(page.getByRole('status').filter({ hasText: 'Pagamento registrado.' })).toBeVisible()
    await expectNoSideScroll(page)

    // A comprar: the card fits, its controls are thumb-sized.
    await goTo(page, 'Compras', 'Compras')
    const card = page.getByRole('article', { name: bought.supplier })
    await expect(card.getByRole('checkbox', { name: `1× ${bought.item}` })).toBeChecked()
    await expect(card.getByText(/Custo \(por unidade\)/)).toContainText(/180,00/)
    await expectNoSideScroll(page)
    await expectThumbSized(48, card.getByRole('checkbox', { name: `1× ${bought.item}` }), card.getByRole('button', { name: 'Ver mensagem' }), card.getByRole('button', { name: 'Enviar pelo WhatsApp' }))

    // Ver mensagem: the API's text, nothing of the client.
    await card.getByRole('button', { name: 'Ver mensagem' }).click()
    const preview = card.getByRole('region', { name: `Mensagem para ${bought.supplier}` })
    await expect(preview).toContainText(`1× ${bought.item}`)
    expect(await preview.locator('pre').innerText()).not.toMatch(/Rafael|Lima|992|9123|4567|P-\d{4}/)
    await expectNoSideScroll(page)
    await expectThumbSized(48, card.getByRole('button', { name: 'Copiar mensagem' }))

    // Enviar pelo WhatsApp: only the API's link opens.
    const sentResponse = page.waitForResponse((response) => response.request().method() === 'POST' && response.url().endsWith('/supplier-orders'))
    await card.getByRole('button', { name: 'Enviar pelo WhatsApp' }).click()
    const sentOrder = (await (await sentResponse).json()) as { whatsappUrl: string }
    await expect(page.getByRole('status').filter({ hasText: 'Pedido de compra C-0001 criado.' })).toBeVisible()
    await expect.poll(async () => (await whatsapp.opened()).length).toBe(1)
    expect(await whatsapp.opened()).toEqual([sentOrder.whatsappUrl])
    expect(sentOrder.whatsappUrl.startsWith(`https://wa.me/${bought.supplierPhone}`)).toBe(true)
    await expectNoSideScroll(page)
    await expectThumbSized(48, page.getByRole('button', { name: 'Abrir o WhatsApp' }), page.getByRole('button', { name: 'Copiar mensagem' }), page.getByRole('link', { name: 'Ver pedido de compra' }))

    // The pedido de compra: its actions are thumb-sized and its dialogs are sheets.
    await page.getByRole('link', { name: 'Ver pedido de compra' }).click()
    await expect(page.getByRole('heading', { name: 'Pedido de compra C-0001' })).toBeVisible()
    await expectNoSideScroll(page)
    await expectThumbSized(
      48,
      page.getByRole('button', { name: 'Abrir conversa no WhatsApp' }),
      page.getByRole('button', { name: 'Informar prazo' }),
      page.getByRole('button', { name: 'Marcar como em trânsito' }),
      page.getByRole('link', { name: 'Receber itens' }),
      page.getByRole('button', { name: 'Informar preço pago' }),
      page.getByRole('button', { name: 'Desfazer envio' }),
    )
    const lines = page.getByRole('table', { name: 'Itens do pedido de compra' })
    await expect(lines.getByRole('row', { name: new RegExp(bought.item) })).toContainText('Enviada ao fornecedor')

    await page.getByRole('button', { name: 'Desfazer envio' }).click()
    const undo = await expectSheetFits(page, 'Desfazer envio')
    await expectThumbSized(48, undo.getByRole('button', { name: 'Desfazer envio' }), undo.getByRole('button', { name: 'Voltar' }))
    await undo.getByRole('button', { name: 'Voltar' }).click()
    await expect(undo).toBeHidden()

    await page.getByRole('button', { name: 'Informar preço pago' }).click()
    const prices = await expectSheetFits(page, 'Informar preço pago')
    await expectThumbSized(48, prices.getByLabel(/Preço pago/), prices.getByRole('button', { name: 'Salvar preços' }), prices.getByRole('button', { name: 'Cancelar' }))
    await prices.getByRole('button', { name: 'Cancelar' }).click()
    await expect(prices).toBeHidden()

    const promised = await page.evaluate(() => {
      const day = new Date()
      day.setDate(day.getDate() + 3)
      return `${day.getFullYear()}-${String(day.getMonth() + 1).padStart(2, '0')}-${String(day.getDate()).padStart(2, '0')}`
    })
    await page.getByRole('button', { name: 'Informar prazo' }).click()
    const dateSheet = await expectSheetFits(page, 'Informar prazo')
    await expectThumbSized(48, dateSheet.getByLabel('Data prometida pelo fornecedor'), dateSheet.getByRole('button', { name: 'Salvar prazo' }), dateSheet.getByRole('button', { name: 'Cancelar' }))
    await dateSheet.getByLabel('Data prometida pelo fornecedor').fill(promised)
    await dateSheet.getByRole('button', { name: 'Salvar prazo' }).click()
    await expect(page.getByRole('status').filter({ hasText: 'Prazo salvo. As datas previstas dos pedidos foram atualizadas.' })).toBeVisible()
    await expectNoSideScroll(page)

    await page.getByRole('button', { name: 'Marcar como em trânsito' }).click()
    await expect(page.getByRole('status').filter({ hasText: 'Itens em trânsito.' })).toBeVisible()
    await expect(lines.getByRole('row', { name: new RegExp(bought.item) })).toContainText('Em trânsito')

    // Receber itens, workshop sizes: the defective item with its note and a photo.
    await page.getByRole('link', { name: 'Receber itens' }).click()
    await expect(page.getByText('Mostrando só o pedido de compra C-0001.')).toBeVisible()
    await expectNoSideScroll(page)
    const save = page.getByRole('button', { name: 'Salvar recebimento' })
    const defective = page.getByRole('button', { name: `Com defeito: ${bought.item}` })
    const photoLabel = page.getByText('Foto do item', { exact: false }).first()
    await expectWorkshopSized(
      page,
      [page.getByRole('button', { name: `Tudo certo: ${bought.item}` }), defective, page.getByLabel('Buscar item, pedido ou fornecedor'), page.getByRole('button', { name: 'Mostrar todos' })],
      [page.getByText(`1× ${bought.item}`), page.getByText('Marque um item só quando toda a quantidade chegou.')],
    )
    await defective.click()
    await expect(photoLabel).toBeVisible()
    await expectWorkshopSized(page, [photoLabel, page.getByText('Foto da nota fiscal'), page.getByLabel('O que houve?')], [page.getByText('O que houve?')])
    await save.click()
    await expect(page.getByText(`Conte o que houve com ${bought.item}.`)).toBeVisible()
    await page.getByLabel('O que houve?').fill('Costura aberta no encosto')
    await page.getByLabel(`Foto do item: ${bought.item}`).setInputFiles(itemPhoto)
    await expect(page.getByText('Foto 1', { exact: true })).toBeVisible()
    await expect(page.getByText('Enviando a foto…')).toHaveCount(0)
    await expectWorkshopSized(page, [page.getByRole('button', { name: /^Remover foto/ })], [page.getByText('Foto 1', { exact: true })])
    await expectNoSideScroll(page)
    await save.click()
    await expect(page.getByText('Recebimento salvo.')).toBeVisible()
    await expect(page.getByText(`${bought.item} voltou para A comprar. A reposição está em Compras.`)).toBeVisible()
    await expectNoSideScroll(page)

    // The replacement is back in A comprar and goes out again.
    await goTo(page, 'Compras', 'Compras')
    const again = page.getByRole('article', { name: bought.supplier })
    await expect(again.getByText('Reposição', { exact: true })).toBeVisible()
    await expectNoSideScroll(page)
    await again.getByRole('button', { name: 'Enviar pelo WhatsApp' }).click()
    await expect(page.getByRole('status').filter({ hasText: 'Pedido de compra C-0002 criado.' })).toBeVisible()
    await expect.poll(async () => (await whatsapp.opened()).length).toBe(2)

    // The Receber itens tab of Compras has the same sizes; the item arrives right and the order is complete.
    await page.getByRole('tab', { name: 'Receber itens' }).click()
    await expect(page.getByRole('heading', { name: `C-0002 · ${bought.supplier}` })).toBeVisible()
    await expectNoSideScroll(page)
    await page.getByRole('button', { name: `Tudo certo: ${bought.item}` }).click()
    await expectWorkshopSized(page, [page.getByRole('button', { name: `Tudo certo: ${bought.item}` }), page.locator('label', { hasText: 'Foto do item' }).first()], [page.getByText(`1× ${bought.item}`)])
    await save.click()
    await expect(page.getByText('Recebimento salvo.')).toBeVisible()
    await expect(page.getByText('Pedido P-0001: todos os itens chegaram e foram conferidos.')).toBeVisible()
    await expectThumbSized(56, page.getByRole('link', { name: 'Abrir pedido P-0001' }))

    // The order opens from the receipt and moves into Montagem.
    await page.getByRole('link', { name: 'Abrir pedido P-0001' }).click()
    await expect(page.getByRole('heading', { name: 'Pedido P-0001' })).toBeVisible()
    await expect(page.getByRole('row', { name: new RegExp(`${bought.item}.*Recebida`) })).toBeVisible()
    await expectNoSideScroll(page)
    await expectThumbSized(48, page.getByRole('button', { name: 'Mover para Em montagem' }))
    await page.getByRole('button', { name: 'Mover para Em montagem' }).click()
    await expect(page.getByText('Em montagem', { exact: true })).toBeVisible()

    // Nothing was sent automatically: two links opened, and the browser asked WhatsApp for nothing.
    expect(await whatsapp.opened()).toHaveLength(2)
    expectNoWhatsappRequests(whatsapp.requests)
  })
})
