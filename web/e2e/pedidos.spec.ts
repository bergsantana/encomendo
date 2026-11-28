import { expect, test, type Page } from '@playwright/test'

import { apiSession, createStore, latestEmail, mailpitUrl, password, seedCatalog, signUp, unique } from './support'

// Pedidos against the real stack (see admin.spec.ts for how to run it): an
// owner chooses the store's status list, sells a kit to a new customer,
// records the deposit, receives the items and moves the order to handover.
// An admin opens the same order and sees no Custo and no Margem. Changing the
// delivery date answers a wa.me link that the app only opens in a tab: the
// run records every request the browser makes and asserts that none went to
// WhatsApp ("nothing is sent automatically", ADR 0030).
//
// This spec signs up one account. A full run of the suite signs up ten from
// one address, and the API allows 10 sign-ups an hour per IP (ADR 0021), so
// clear the counts before every full run (see admin.spec.ts):
//   docker exec <redis container> sh -c "redis-cli --scan --pattern 'g:rl:*' | xargs -r redis-cli del"
const realStack = process.env.E2E_BASE_URL !== undefined && process.env.E2E_BASE_URL !== ''

/**
 * watchWhatsapp stubs window.open before any page script runs, so a wa.me tab
 * never really opens, and returns the links the app tried to open and every
 * request the page made.
 */
async function watchWhatsapp(page: Page): Promise<{ opened: () => Promise<string[]>; requests: string[] }> {
  const requests: string[] = []
  page.on('request', (request) => requests.push(request.url()))
  await page.addInitScript(() => {
    const opened: string[] = []
    Object.assign(window, { __opened: opened })
    window.open = (url?: string | URL) => {
      opened.push(String(url))
      return null
    }
  })
  return { requests, opened: () => page.evaluate(() => (window as unknown as { __opened: string[] }).__opened) }
}

/** expectNoWhatsappRequests fails when the browser called WhatsApp itself: the app only hands over a link. */
function expectNoWhatsappRequests(requests: string[]): void {
  expect(requests.filter((url) => /(^|\.)(wa\.me|whatsapp\.com|whatsapp\.net)(\/|:|$)/.test(new URL(url).host + '/')), 'requests to WhatsApp').toEqual([])
  expect(requests.length, 'the browser made requests').toBeGreaterThan(0)
}

test.describe('Pedidos against the real stack', () => {
  test.skip(!realStack, 'set E2E_BASE_URL (and E2E_MAILPIT_URL) to run against `make stack-up`')

  test.beforeEach(() => {
    expect(mailpitUrl, 'E2E_MAILPIT_URL must name Mailpit').not.toBe('')
  })

  test.setTimeout(180_000)

  // A status shows as a badge whose whole text is its name; the moves are "Mover para {status}" and the history is a sentence.
  const status = (page: Page, label: string) => page.getByText(label, { exact: true })
  const menu = (page: Page) => page.getByRole('navigation', { name: 'Menu principal' })

  test('an owner sells a kit from first contact to handover; an admin sees no costs; a new date opens only a wa.me link', async ({ page, request, browser }) => {
    const owner = unique('pedidos')
    const invitee = unique('admin')
    const whatsapp = await watchWhatsapp(page)

    await signUp(page, 'Elisa Prado', owner.email)
    await createStore(page, 'Pedidos Teste', owner.slug)

    // Configurações: the store picks the other status list, and keeps it after a reload.
    await menu(page).getByRole('link', { name: 'Configurações' }).click()
    await expect(page.getByRole('heading', { name: 'Configurações', exact: true })).toBeVisible()
    const preset = page.getByRole('article', { name: 'Orçamento primeiro' })
    await preset.getByRole('button', { name: 'Usar esta lista' }).click()
    await expect(page.getByRole('status').filter({ hasText: 'Lista de status alterada. Os próximos pedidos usam Orçamento primeiro.' })).toBeVisible()
    await page.reload()
    await expect(page.getByRole('article', { name: 'Orçamento primeiro' }).getByText('Em uso')).toBeVisible()

    const catalog = await seedCatalog(await apiSession(request, owner.email))

    // Clientes: a new customer.
    await menu(page).getByRole('link', { name: 'Clientes' }).click()
    await page.getByRole('button', { name: 'Novo cliente' }).first().click()
    await page.getByLabel('Nome', { exact: true }).fill('Rafael Lima')
    await page.getByLabel('WhatsApp', { exact: true }).fill('(92) 99123-4567')
    await page.getByRole('button', { name: 'Salvar', exact: true }).click()
    await expect(page.getByRole('status').filter({ hasText: 'Cliente salvo.' })).toBeVisible()

    // Novo pedido: the customer, the kit, the summary the API calculated.
    await menu(page).getByRole('link', { name: 'Pedidos' }).click()
    await page.getByRole('button', { name: 'Novo pedido' }).first().click()
    await expect(page.getByRole('heading', { name: 'Novo pedido', exact: true })).toBeVisible()
    await page.getByLabel('Buscar cliente').fill('Rafael')
    await page.getByRole('button', { name: /Rafael Lima/ }).click()
    await page.getByRole('button', { name: 'Adicionar kit' }).click()
    await page.getByRole('button', { name: new RegExp(catalog.kit) }).click()
    const summary = page.getByRole('region', { name: 'Resumo' })
    await expect(summary.getByText('R$ 520,00').first()).toBeVisible()
    await expect(summary.getByRole('definition').filter({ hasText: '(7 dias depois do sinal)' })).toBeVisible()
    // The owner sees what the order costs and earns.
    await expect(summary.getByRole('term').filter({ hasText: /^Custo$/ })).toBeVisible()
    await expect(summary.getByRole('term').filter({ hasText: /^Margem$/ })).toBeVisible()
    await page.getByRole('button', { name: 'Salvar rascunho' }).click()
    await expect(page.getByRole('status').filter({ hasText: 'Pedido P-0001 criado.' })).toBeVisible()
    await page.getByRole('link', { name: 'Abrir pedido' }).click()
    await expect(page.getByRole('heading', { name: 'Pedido P-0001' })).toBeVisible()
    // The order screen: moves come from the API, in the store's own words.
    await page.getByRole('button', { name: 'Mover para Aguardando sinal' }).click()
    await expect(status(page, 'Aguardando sinal')).toBeVisible()
    await page.getByRole('button', { name: 'Mover para Aprovado' }).click()
    await expect(page.getByRole('alert').filter({ hasText: 'O sinal ainda não foi pago. Registre o sinal ou libere o pedido sem ele.' })).toBeVisible()

    // The deposit, by hand: the amount comes filled with what is still owed.
    await page.getByRole('button', { name: 'Registrar pagamento' }).click()
    const payment = page.getByRole('dialog')
    await expect(payment.getByLabel('Valor')).toHaveValue(/260,00/)
    await payment.getByLabel('Forma').selectOption({ label: 'Pix' })
    await payment.getByRole('button', { name: 'Registrar', exact: true }).click()
    await expect(page.getByRole('status').filter({ hasText: 'Pagamento registrado.' })).toBeVisible()
    await expect(page.getByRole('region', { name: 'Pagamentos' })).toContainText(/Pago\s*R\$\s*260,00/)

    // A new delivery date: the API answers a wa.me link, the app opens it in a tab and sends nothing.
    const delivery = page.getByRole('region', { name: 'Entrega' })
    // The promised date is the API's, so read it from the screen instead of assuming a day.
    const before = /\d{2}\/\d{2}\/\d{4}/.exec(await delivery.getByText(/^Entrega prevista:/).innerText())?.[0]
    expect(before, 'the promised date on the order screen').toBeDefined()
    await page.getByRole('button', { name: 'Alterar data' }).click()
    const dateDialog = page.getByRole('dialog', { name: 'Alterar data' })
    // The date is picked in the browser's clock, which decides "from today" (the plan's browser-clock rule).
    const later = await page.evaluate(() => {
      const day = new Date()
      day.setDate(day.getDate() + 14)
      return `${day.getFullYear()}-${String(day.getMonth() + 1).padStart(2, '0')}-${String(day.getDate()).padStart(2, '0')}`
    })
    const [year, month, day] = later.split('-')
    await dateDialog.getByLabel('Nova data de entrega').fill(later)
    await dateDialog.getByLabel('Motivo').fill('Atraso na transportadora')
    const changed = page.waitForResponse((response) => response.request().method() === 'PUT' && response.url().endsWith('/delivery-date'))
    await dateDialog.getByRole('button', { name: 'Salvar e avisar o cliente' }).click()
    const answer = (await (await changed).json()) as { whatsappUrl: string }
    await expect(page.getByText('Data alterada. Envie a mensagem para avisar o cliente.')).toBeVisible()
    await expect(page.getByRole('button', { name: 'Abrir o WhatsApp' })).toBeVisible()
    // __opened lives on this document (the init script runs on every load and
    // resets it), so the flow must stay inside the SPA: no reload or full navigation until it is read.
    await expect.poll(async () => (await whatsapp.opened()).length).toBe(1)
    const [dateLink] = await whatsapp.opened()
    expect(dateLink.startsWith('https://wa.me/')).toBe(true)
    expect(dateLink, 'the app opens exactly the link the API answered').toBe(answer.whatsappUrl)
    const dateMessage = new URL(dateLink).searchParams.get('text') ?? ''
    expect(dateMessage).toContain('P-0001')
    expect(dateMessage).toContain('Atraso na transportadora')
    expect(dateMessage).toContain(`${day}/${month}/${year}`)
    expect(dateMessage).not.toMatch(/fornecedor|custo|margem|Escapes do Sul|Pode atrasar/i)
    await expect(delivery.getByText(/^Entrega prevista:/)).toContainText(`${day}/${month}/${year}`)
    await expect(page.getByText(`Elisa Prado alterou a entrega de ${before} para ${day}/${month}/${year}`)).toBeVisible()

    // The status moves; the items have not arrived, so the guard says so.
    await page.getByRole('button', { name: 'Mover para Aprovado' }).click()
    await expect(status(page, 'Aprovado')).toBeVisible()
    await page.getByRole('button', { name: 'Mover para Pronto' }).click()
    await expect(page.getByRole('alert').filter({ hasText: 'Nem todos os itens chegaram e foram conferidos.' })).toBeVisible()

    // Items: the first one step at a time, the second straight to received.
    await page.getByRole('button', { name: `Marcar como enviada ao fornecedor: ${catalog.first}` }).click()
    await page.getByRole('button', { name: `Marcar como em trânsito: ${catalog.first}` }).click()
    await page.getByRole('button', { name: `Tudo certo: ${catalog.first}` }).click()
    await page.getByRole('button', { name: `Tudo certo: ${catalog.second}` }).click()
    await expect(page.getByRole('row', { name: new RegExp(`${catalog.first}.*Recebida`) })).toBeVisible()
    await expect(page.getByRole('row', { name: new RegExp(`${catalog.second}.*Recebida`) })).toBeVisible()

    await page.getByRole('button', { name: 'Mover para Em preparação' }).click()
    await expect(status(page, 'Em preparação')).toBeVisible()
    await page.getByRole('button', { name: 'Mover para Pronto' }).click()
    await expect(status(page, 'Pronto')).toBeVisible()

    // Handing over needs the balance first.
    await page.getByRole('button', { name: 'Mover para Entregue' }).click()
    const handover = page.getByRole('dialog', { name: 'Como o pedido foi entregue?' })
    await handover.getByLabel('Retirada na loja').check()
    await handover.getByRole('button', { name: 'Mover para Entregue' }).click()
    await expect(handover.getByRole('alert').filter({ hasText: /Falta receber o saldo de R\$\s260,00\./ })).toBeVisible()
    await handover.getByRole('button', { name: 'Voltar' }).click()
    await page.getByRole('button', { name: 'Registrar pagamento' }).click()
    await expect(page.getByRole('dialog').getByLabel('Tipo')).toHaveValue('balance')
    await expect(page.getByRole('dialog').getByLabel('Valor')).toHaveValue(/260,00/)
    await page.getByRole('dialog').getByRole('button', { name: 'Registrar', exact: true }).click()
    await expect(page.getByRole('status').filter({ hasText: 'Pagamento registrado.' })).toBeVisible()
    await page.getByRole('button', { name: 'Mover para Entregue' }).click()
    await page.getByRole('dialog').getByLabel('Retirada na loja').check()
    await page.getByRole('dialog').getByRole('button', { name: 'Mover para Entregue' }).click()
    await expect(status(page, 'Entregue')).toBeVisible()

    // The delivered order leaves Hoje and the active tab, and sits under Entregues; Financeiro reads it.
    await menu(page).getByRole('link', { name: 'Hoje' }).click()
    await expect(page.getByText('Nada precisa de você agora.')).toBeVisible()
    await expect(page.getByText('Nenhuma entrega prevista.')).toBeVisible()
    await menu(page).getByRole('link', { name: 'Pedidos' }).click()
    await expect(page.getByRole('heading', { name: 'Pedidos', exact: true })).toBeVisible()
    // The empty message proves the list was read before "no P-0001" means anything.
    await expect(page.getByText(/^Nenhum pedido (ainda|encontrado)\.$/)).toBeVisible()
    await expect(page.getByText('P-0001')).toHaveCount(0)
    await page.getByRole('tab', { name: 'Entregues' }).click()
    await expect(page.getByRole('row', { name: /P-0001/ })).toContainText('Rafael Lima')
    await menu(page).getByRole('link', { name: 'Financeiro' }).click()
    await expect(page.getByRole('heading', { name: 'Financeiro', exact: true })).toBeVisible()
    await expect(page.getByText('Recebido no período')).toBeVisible()
    await expect(page.getByRole('row', { name: /P-0001/ })).toContainText('170,00')

    // An admin is invited through Equipe and sees the order with no Custo and no Margem.
    await menu(page).getByRole('link', { name: 'Equipe' }).click()
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
      const guestWhatsapp = await watchWhatsapp(guest)
      await guest.goto(new URL(url.pathname + url.hash, process.env.E2E_BASE_URL).toString())
      await guest.getByLabel('Nome').fill('Fábio Admin')
      await guest.getByLabel('Senha').fill(password)
      await guest.getByRole('button', { name: 'Aceitar convite' }).click()
      await expect(guest).toHaveURL(/\/hoje$/)
      await expect(menu(guest).getByRole('link', { name: 'Financeiro' })).toHaveCount(0)

      await menu(guest).getByRole('link', { name: 'Pedidos' }).click()
      await guest.getByRole('tab', { name: 'Entregues' }).click()
      await guest.getByRole('link', { name: 'P-0001' }).click()
      await expect(guest.getByRole('heading', { name: 'Pedido P-0001' })).toBeVisible()
      await expect(guest.getByRole('table', { name: 'Itens do pedido' }).getByRole('row', { name: new RegExp(catalog.first) })).toBeVisible()
      await expect(guest.getByRole('columnheader', { name: 'Custo' })).toHaveCount(0)
      await expect(guest.getByText(/Custo|Margem/)).toHaveCount(0)
      await expect(guest.getByText('R$ 280,00')).toHaveCount(0)
      await expect(guest.getByText('R$ 170,00')).toHaveCount(0)

      // The same on "Novo pedido": the quote has prices and dates, and no Custo and no Margem.
      await menu(guest).getByRole('link', { name: 'Pedidos' }).click()
      await guest.getByRole('button', { name: 'Novo pedido' }).first().click()
      await guest.getByRole('button', { name: 'Adicionar kit' }).click()
      await guest.getByRole('button', { name: new RegExp(catalog.kit) }).click()
      const guestSummary = guest.getByRole('region', { name: 'Resumo' })
      await expect(guestSummary.getByRole('term').filter({ hasText: /^Total$/ })).toBeVisible()
      await expect(guestSummary.getByRole('definition').filter({ hasText: 'dias depois do sinal' })).toBeVisible()
      await expect(guestSummary.getByRole('term').filter({ hasText: /^Custo$/ })).toHaveCount(0)
      await expect(guestSummary.getByRole('term').filter({ hasText: /^Margem$/ })).toHaveCount(0)
      expect(await guestWhatsapp.opened()).toEqual([])
      expectNoWhatsappRequests(guestWhatsapp.requests)
    } finally {
      await guestContext.close()
    }

    // Nothing is sent automatically: the browser asked WhatsApp for nothing, and the API only wrote the link.
    expect(await whatsapp.opened()).toHaveLength(1)
    expectNoWhatsappRequests(whatsapp.requests)
  })
})
