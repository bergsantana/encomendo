import { expect, test, type Locator, type Page } from '@playwright/test'

import { apiSession, createStore, mailpitUrl, openMenu, seedCatalog, signUp, unique } from './support'

// The milestone 4 screens at phone size (ADR 0029), in the `phone` project of
// playwright.config.ts: 390 x 844, touch. The main path of pedidos.spec.ts runs
// here against the real stack (`make stack-up`, E2E_BASE_URL and
// E2E_MAILPIT_URL): choose the status list, make a customer, sell a kit, record
// the deposit, change the delivery date and receive the items, then visit
// Hoje, Pedidos, Clientes, Financeiro and Configurações. Every screen must fit
// the width and every control a thumb hits must be 48 px tall. The dialogs the
// flow opens are checked as full-screen sheets: the customer form, the kit and
// item pickers, Registrar pagamento, Anular, Liberar sem sinal, Cancelar
// pedido, Alterar data and the handover question. Com defeito and "Enviar link
// de acompanhamento" are buttons on the page, not dialogs, so they are checked
// for thumb size only. The run signs up one account. A full run of the suite
// signs up ten from one address, and the API allows 10 sign-ups an hour per IP
// (ADR 0021), so clear the counts before every full run (see admin.spec.ts).
const realStack = process.env.E2E_BASE_URL !== undefined && process.env.E2E_BASE_URL !== ''

/** expectNoSideScroll fails when the page is wider than the screen: a phone must never scroll sideways. */
async function expectNoSideScroll(page: Page): Promise<void> {
  const widths = await page.evaluate(() => ({ scroll: document.documentElement.scrollWidth, inner: window.innerWidth }))
  expect(widths.scroll, `page is ${widths.scroll} px wide on a ${widths.inner} px screen`).toBeLessThanOrEqual(widths.inner)
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

/** expectSheetFits checks the open sheet fills the 390 x 844 screen and nothing in it spills sideways. */
async function expectSheetFits(page: Page, name: string | RegExp): Promise<Locator> {
  const sheet = page.getByRole('dialog', typeof name === 'string' ? { name, exact: true } : { name })
  await expect(sheet).toBeVisible()
  const box = await sheet.boundingBox()
  expect([box?.x, box?.y, box?.width, box?.height]).toEqual([0, 0, 390, 844])
  await expectNoSideScroll(page)
  const inner = await sheet.evaluate((element) => {
    const body = element.querySelector('.enc-dialog__body')
    return body === null ? 0 : body.scrollWidth - body.clientWidth
  })
  expect(inner, 'the sheet scrolls sideways').toBeLessThanOrEqual(0)
  return sheet
}

test.describe('against the real stack', () => {
  test.skip(!realStack, 'set E2E_BASE_URL (and E2E_MAILPIT_URL) to run against `make stack-up`')

  test.beforeEach(() => {
    expect(mailpitUrl, 'E2E_MAILPIT_URL must name Mailpit').not.toBe('')
  })

  test.setTimeout(180_000)

  // A status shows as a badge whose whole text is its name; the moves are "Mover para {status}" and the history is a sentence.
  const status = (page: Page, label: string) => page.getByText(label, { exact: true })

  /** goTo opens a menu entry through the drawer and checks the drawer closed. */
  async function goTo(page: Page, entry: string, heading: string): Promise<void> {
    await openMenu(page)
    await page.getByRole('navigation', { name: 'Menu principal' }).getByRole('link', { name: entry, exact: true }).click()
    await expect(page.getByRole('heading', { name: heading, exact: true })).toBeVisible()
    await expect(page.getByRole('navigation', { name: 'Menu principal' })).toBeHidden()
  }

  test('an owner sells a kit and moves the order along, on every M4 screen', async ({ page, request }) => {
    const owner = unique('fonepedidos')
    // window.open is stubbed before any page script runs, so a wa.me tab never really opens; it lives on the document, so the flow stays SPA-only.
    await page.addInitScript(() => {
      const opened: string[] = []
      Object.assign(window, { __opened: opened })
      window.open = (url?: string | URL) => {
        opened.push(String(url))
        return null
      }
    })

    await signUp(page, 'Elisa Prado', owner.email)
    await createStore(page, 'Pedidos Fone', owner.slug)

    // Hoje, with nothing to do yet.
    await expectNoSideScroll(page)
    await expect(page.getByText('Nada precisa de você agora.')).toBeVisible()

    // Configurações: the cards of the status lists fit, and the choice is a thumb-sized button.
    await goTo(page, 'Configurações', 'Configurações')
    await expectNoSideScroll(page)
    const preset = page.getByRole('article', { name: 'Orçamento primeiro' })
    const choose = preset.getByRole('button', { name: 'Usar esta lista' })
    await expectThumbSized(choose, page.getByRole('button', { name: 'Salvar', exact: true }))
    await choose.click()
    await expect(page.getByRole('status').filter({ hasText: 'Lista de status alterada. Os próximos pedidos usam Orçamento primeiro.' })).toBeVisible()
    await expectNoSideScroll(page)

    const api = await apiSession(request, owner.email)
    const catalog = await seedCatalog(api)

    // Clientes: the empty state, then the customer sheet.
    await goTo(page, 'Clientes', 'Clientes')
    await expectNoSideScroll(page)
    await expect(page.getByText('Nenhum cliente ainda.')).toBeVisible()
    await expectThumbSized(page.getByRole('button', { name: 'Novo cliente' }).first())
    await page.getByRole('button', { name: 'Novo cliente' }).first().click()
    const customerSheet = await expectSheetFits(page, 'Novo cliente')
    await expectThumbSized(customerSheet.getByLabel('Nome', { exact: true }), customerSheet.getByLabel('WhatsApp', { exact: true }), customerSheet.getByRole('button', { name: 'Adicionar endereço' }))
    await customerSheet.getByLabel('Nome', { exact: true }).fill('Rafael Lima')
    await customerSheet.getByLabel('WhatsApp', { exact: true }).fill('(92) 99123-4567')
    await customerSheet.getByRole('button', { name: 'Salvar', exact: true }).click()
    await expect(page.getByRole('status').filter({ hasText: 'Cliente salvo.' })).toBeVisible()
    await expectNoSideScroll(page)
    const customerCard = page.getByRole('row', { name: /Rafael Lima/ })
    await expect(customerCard).toContainText('(92) 99123-4567')

    // Pedidos: the empty list, then Novo pedido.
    await goTo(page, 'Pedidos', 'Pedidos')
    await expectNoSideScroll(page)
    await expect(page.getByText('Nenhum pedido ainda.')).toBeVisible()
    await expectThumbSized(page.getByRole('button', { name: 'Novo pedido' }).first())
    await page.getByRole('button', { name: 'Novo pedido' }).first().click()
    await expect(page.getByRole('heading', { name: 'Novo pedido', exact: true })).toBeVisible()
    await expectNoSideScroll(page)
    await expectThumbSized(page.getByLabel('Buscar cliente'))
    await page.getByLabel('Buscar cliente').fill('Rafael')
    await page.getByRole('button', { name: /Rafael Lima/ }).click()
    await expectThumbSized(page.getByRole('button', { name: 'Adicionar kit' }), page.getByRole('button', { name: 'Adicionar item' }), page.getByLabel('Retirada na loja'), page.getByLabel('Entrega', { exact: true }))
    await page.getByRole('button', { name: 'Adicionar kit' }).click()
    const picker = await expectSheetFits(page, 'Adicionar kit')
    await expectThumbSized(picker.getByRole('button', { name: new RegExp(catalog.kit) }), picker.getByRole('button', { name: 'Fechar' }))
    await picker.getByRole('button', { name: new RegExp(catalog.kit) }).click()
    await page.getByRole('button', { name: 'Adicionar item' }).click()
    const itemPicker = await expectSheetFits(page, 'Adicionar item')
    await expectThumbSized(itemPicker.getByLabel('Buscar item'), itemPicker.getByRole('button', { name: new RegExp(catalog.first) }), itemPicker.getByRole('button', { name: 'Fechar' }))
    await itemPicker.getByRole('button', { name: 'Fechar' }).click()
    await expect(itemPicker).toBeHidden()
    const summary = page.getByRole('region', { name: 'Resumo' })
    await expect(summary.getByRole('definition').filter({ hasText: '(7 dias depois do sinal)' })).toBeVisible()
    await expect(summary.getByRole('term').filter({ hasText: /^Margem$/ })).toBeVisible()
    await expectNoSideScroll(page)
    await expectThumbSized(page.getByRole('button', { name: 'Remover Kit esportivo' }), page.getByRole('button', { name: 'Salvar rascunho' }), page.getByRole('button', { name: 'Salvar e enviar ao cliente' }), page.getByRole('button', { name: 'Cancelar' }))
    await page.getByRole('button', { name: 'Salvar rascunho' }).click()
    await expect(page.getByRole('status').filter({ hasText: 'Pedido P-0001 criado.' })).toBeVisible()
    await expectNoSideScroll(page)
    await page.getByRole('link', { name: 'Abrir pedido' }).click()

    // Pedido: the order screen, its dialogs as sheets.
    await expect(page.getByRole('heading', { name: 'Pedido P-0001' })).toBeVisible()
    await expectNoSideScroll(page)
    await expectThumbSized(
      page.getByRole('button', { name: 'Abrir conversa no WhatsApp' }),
      page.getByRole('button', { name: 'Cancelar pedido' }),
      page.getByRole('button', { name: 'Enviar link de acompanhamento' }),
      page.getByRole('button', { name: 'Alterar data' }),
      page.getByRole('button', { name: 'Mover para Aguardando sinal' }),
      page.getByRole('button', { name: 'Registrar pagamento' }),
      page.getByRole('button', { name: `Marcar como enviada ao fornecedor: ${catalog.first}` }),
      page.getByRole('button', { name: `Tudo certo: ${catalog.first}` }),
      page.getByRole('button', { name: `Com defeito: ${catalog.first}` }),
    )
    await page.getByRole('button', { name: 'Mover para Aguardando sinal' }).click()
    await expect(status(page, 'Aguardando sinal')).toBeVisible()

    // The owner's release and the cancel question are sheets with a thumb-sized "Voltar".
    await expectThumbSized(page.getByRole('button', { name: 'Liberar sem sinal' }))
    await page.getByRole('button', { name: 'Liberar sem sinal' }).click()
    const release = await expectSheetFits(page, 'Liberar o pedido sem o sinal?')
    await expectThumbSized(release.getByRole('button', { name: 'Liberar pedido' }), release.getByRole('button', { name: 'Voltar' }))
    await release.getByRole('button', { name: 'Voltar' }).click()
    await expect(release).toBeHidden()
    await page.getByRole('button', { name: 'Cancelar pedido' }).click()
    const cancel = await expectSheetFits(page, 'Cancelar o pedido P-0001?')
    await expectThumbSized(cancel.getByLabel('Motivo'), cancel.getByRole('button', { name: 'Cancelar pedido' }), cancel.getByRole('button', { name: 'Voltar' }))
    await cancel.getByRole('button', { name: 'Voltar' }).click()
    await expect(cancel).toBeHidden()

    // "Enviar link de acompanhamento" is not a dialog: it asks for a new link and opens it, which the stub records.
    await page.getByRole('button', { name: 'Enviar link de acompanhamento' }).click()
    await expect(page.getByText('Link de acompanhamento criado.')).toBeVisible()
    await expect.poll(() => page.evaluate(() => (window as unknown as { __opened: string[] }).__opened)).toEqual([expect.stringMatching(/^https:\/\/wa\.me\//)])
    await expectNoSideScroll(page)

    await page.getByRole('button', { name: 'Registrar pagamento' }).click()
    const payment = await expectSheetFits(page, 'Registrar pagamento')
    await expectThumbSized(payment.getByLabel('Tipo'), payment.getByLabel('Valor'), payment.getByLabel('Forma'), payment.getByRole('button', { name: 'Registrar', exact: true }), payment.getByRole('button', { name: 'Cancelar' }))
    await expect(payment.getByLabel('Valor')).toHaveValue(/260,00/)
    await payment.getByLabel('Forma').selectOption({ label: 'Pix' })
    await payment.getByRole('button', { name: 'Registrar', exact: true }).click()
    await expect(page.getByRole('status').filter({ hasText: 'Pagamento registrado.' })).toBeVisible()
    await expectNoSideScroll(page)
    await expect(page.getByRole('region', { name: 'Pagamentos' })).toContainText(/Pago\s*R\$\s*260,00/)

    // Anular (the owner's) opens a sheet; it is left unanswered, so the deposit stays.
    await page.getByRole('button', { name: /^Anular o pagamento de/ }).click()
    const voiding = await expectSheetFits(page, /^Anular o pagamento de/)
    await expectThumbSized(voiding.getByLabel('Motivo'), voiding.getByRole('button', { name: 'Anular pagamento' }), voiding.getByRole('button', { name: 'Voltar' }))
    await voiding.getByRole('button', { name: 'Voltar' }).click()
    await expect(voiding).toBeHidden()

    await page.getByRole('button', { name: 'Alterar data' }).click()
    const dateSheet = await expectSheetFits(page, 'Alterar data')
    await expectThumbSized(dateSheet.getByLabel('Nova data de entrega'), dateSheet.getByLabel('Motivo'), dateSheet.getByRole('button', { name: 'Salvar e avisar o cliente' }), dateSheet.getByRole('button', { name: 'Cancelar' }))
    await dateSheet.getByRole('button', { name: 'Cancelar' }).click()
    await expect(dateSheet).toBeHidden()

    // Hoje with an active order: the summary cards, "Precisa de você" and the next deliveries, then back to the order.
    await goTo(page, 'Hoje', 'Hoje')
    await expect(page.getByRole('heading', { name: 'Precisa de você' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Próximas entregas' })).toBeVisible()
    await expect(page.getByText('Pedidos ativos')).toBeVisible()
    await expect(page.getByText('P-0001').first()).toBeVisible()
    await expectNoSideScroll(page)
    await goTo(page, 'Pedidos', 'Pedidos')
    await expectNoSideScroll(page)
    await page.getByRole('link', { name: 'P-0001' }).click()
    await expect(page.getByRole('heading', { name: 'Pedido P-0001' })).toBeVisible()

    // The items arrive and the order moves to Pronto, then the handover sheet asks how it was delivered.
    await page.getByRole('button', { name: 'Mover para Aprovado' }).click()
    await page.getByRole('button', { name: `Tudo certo: ${catalog.first}` }).click()
    await page.getByRole('button', { name: `Tudo certo: ${catalog.second}` }).click()
    await expect(page.getByRole('row', { name: new RegExp(`${catalog.second}.*Recebida`) })).toBeVisible()
    await page.getByRole('button', { name: 'Mover para Em preparação' }).click()
    await page.getByRole('button', { name: 'Mover para Pronto' }).click()
    await expect(status(page, 'Pronto')).toBeVisible()
    await page.getByRole('button', { name: 'Mover para Entregue' }).click()
    const handover = await expectSheetFits(page, 'Como o pedido foi entregue?')
    await expectThumbSized(handover.getByLabel('Entrega', { exact: true }), handover.getByLabel('Retirada na loja'), handover.getByRole('button', { name: 'Mover para Entregue' }), handover.getByRole('button', { name: 'Voltar' }))
    await handover.getByLabel('Retirada na loja').check()
    await handover.getByRole('button', { name: 'Mover para Entregue' }).click()
    await expect(handover.getByRole('alert').filter({ hasText: /Falta receber o saldo de R\$\s260,00\./ })).toBeVisible()
    await expectSheetFits(page, 'Como o pedido foi entregue?')
    await handover.getByRole('button', { name: 'Voltar' }).click()
    await page.getByRole('button', { name: 'Registrar pagamento' }).click()
    const balance = await expectSheetFits(page, 'Registrar pagamento')
    await expect(balance.getByLabel('Tipo')).toHaveValue('balance')
    await balance.getByRole('button', { name: 'Registrar', exact: true }).click()
    await expect(page.getByRole('status').filter({ hasText: 'Pagamento registrado.' })).toBeVisible()
    await page.getByRole('button', { name: 'Mover para Entregue' }).click()
    await handover.getByLabel('Retirada na loja').check()
    await handover.getByRole('button', { name: 'Mover para Entregue' }).click()
    await expect(status(page, 'Entregue')).toBeVisible()
    await expectNoSideScroll(page)

    // Pedidos with a delivered order: the card, the status tabs and the search fit.
    await goTo(page, 'Pedidos', 'Pedidos')
    await page.getByRole('tab', { name: 'Entregues' }).click()
    const card = page.getByRole('row', { name: /P-0001/ })
    await expect(card).toContainText('Rafael Lima')
    await expect(card.locator('td[data-label="Total"]')).toContainText('520,00')
    await expect(card.locator('td[data-label="Entrega prevista"]')).toBeVisible()
    await expectNoSideScroll(page)
    await expectThumbSized(page.getByRole('tab', { name: 'Entregues' }), page.getByLabel('Buscar por número ou cliente'))

    // Clientes with a customer: the card shows the orders count.
    await goTo(page, 'Clientes', 'Clientes')
    await expect(page.getByRole('row', { name: /Rafael Lima/ }).locator('td[data-label="Pedidos"]')).toHaveText('1')
    await expectNoSideScroll(page)
    await expectThumbSized(page.getByLabel('Buscar por nome ou WhatsApp'))

    // A second order, made through the API and left unpaid, so Financeiro has a balance owed to list.
    await api.send('POST', '/api/v1/admin/orders', { newCustomer: { name: 'Marina Alves', phone: '5592991234568' }, lines: [{ kitId: catalog.kitId }], terms: { handover: 'pickup', freightCents: 0 } }, { 'idempotency-key': `${owner.slug}-saldo` })

    // Financeiro: the owner's period, what was received and what is owed.
    await goTo(page, 'Financeiro', 'Financeiro')
    await expect(page.getByText('Recebido no período')).toBeVisible()
    await expect(page.getByText('A receber', { exact: true })).toBeVisible()
    // "A receber" lists the unpaid order at phone size, with its balance and its delivery date.
    const owed = page.getByRole('row', { name: /P-0002/ })
    await expect(owed).toContainText('Marina Alves')
    await expect(owed.locator('td[data-label="Saldo"]')).toContainText('520,00')
    await expect(owed.locator('td[data-label="Entrega prevista"]')).toBeVisible()
    await expect(page.getByRole('row', { name: /P-0001/ })).toContainText('170,00')
    await expectNoSideScroll(page)
    await expectThumbSized(page.getByLabel('Período', { exact: true }))
  })
})
