import { expect, test, type Page } from '@playwright/test'

import { apiSession, createStore, expectNoWhatsappRequests, itemPhoto, latestEmail, mailpitUrl, noClientData, password, placeOrder, seedBoughtItem, signUp, unique, watchWhatsapp } from './support'

// Compras and Receber itens against the real stack (see admin.spec.ts for how
// to run it). An owner sells a bought item and records the deposit; the item
// shows in "A comprar"; the message the API wrote has no client data; sending
// it opens only the API's wa.me link and creates the pedido de compra C-0001;
// the supplier's date is recorded; the item arrives "Com defeito" with a
// photo, comes back as "Reposição", is sent again and arrives "Tudo certo",
// and the order moves into Montagem. An admin sees no costs and no price paid.
// Bought items move only through Compras (ADR 0031).
//
// This spec signs up one account. A full run of the suite signs up several
// from one address, and the API allows 10 sign-ups an hour per IP (ADR 0021),
// so clear the counts before every full run (see admin.spec.ts):
//   docker exec <redis container> sh -c "redis-cli --scan --pattern 'g:rl:*' | xargs -r redis-cli del"
const realStack = process.env.E2E_BASE_URL !== undefined && process.env.E2E_BASE_URL !== ''

/** inDays is the civil date `days` from today in the browser's clock, which decides "from today" (the plan's browser-clock rule). */
async function inDays(page: Page, days: number): Promise<string> {
  return page.evaluate((ahead) => {
    const day = new Date()
    day.setDate(day.getDate() + ahead)
    return `${day.getFullYear()}-${String(day.getMonth() + 1).padStart(2, '0')}-${String(day.getDate()).padStart(2, '0')}`
  }, days)
}

test.describe('Compras against the real stack', () => {
  test.skip(!realStack, 'set E2E_BASE_URL (and E2E_MAILPIT_URL) to run against `make stack-up`')

  test.beforeEach(() => {
    expect(mailpitUrl, 'E2E_MAILPIT_URL must name Mailpit').not.toBe('')
  })

  test.setTimeout(180_000)

  const menu = (page: Page) => page.getByRole('navigation', { name: 'Menu principal' })

  test('an owner buys, receives a defective item, buys it again and receives it; an admin sees no costs', async ({ page, request, browser }) => {
    const owner = unique('compras')
    const invitee = unique('admin')
    const whatsapp = await watchWhatsapp(page)

    await signUp(page, 'Elisa Prado', owner.email)
    await createStore(page, 'Compras Teste', owner.slug)

    const api = await apiSession(request, owner.email)
    const bought = await seedBoughtItem(api)
    const first = await placeOrder(api, bought.itemId, `${owner.slug}-1`, false)
    expect(first.number).toBe('P-0001')

    // Nothing to buy before the deposit.
    await menu(page).getByRole('link', { name: 'Compras' }).click()
    await expect(page.getByRole('heading', { name: 'Compras', exact: true })).toBeVisible()
    await expect(page.getByText('Nada para comprar agora.')).toBeVisible()
    await expect(page.getByText('Os itens aparecem aqui quando o cliente paga o sinal.')).toBeVisible()

    // Pedido: the deposit, by hand. The bought item has no "Marcar como enviada" any more: only a way into Compras.
    await menu(page).getByRole('link', { name: 'Pedidos' }).click()
    await page.getByRole('link', { name: first.number }).click()
    await expect(page.getByRole('heading', { name: 'Pedido P-0001' })).toBeVisible()
    await expect(page.getByRole('button', { name: /Marcar como enviada ao fornecedor|Marcar como em trânsito/ })).toHaveCount(0)
    await expect(page.getByRole('button', { name: `Tudo certo: ${bought.item}` })).toHaveCount(0)
    await expect(page.getByRole('button', { name: `Com defeito: ${bought.item}` })).toHaveCount(0)
    await page.getByRole('button', { name: 'Registrar pagamento' }).click()
    const payment = page.getByRole('dialog')
    await expect(payment.getByLabel('Valor')).toHaveValue(/150,00/)
    await payment.getByLabel('Forma').selectOption({ label: 'Pix' })
    await payment.getByRole('button', { name: 'Registrar', exact: true }).click()
    await expect(page.getByRole('status').filter({ hasText: 'Pagamento registrado.' })).toBeVisible()

    // Compras, A comprar: the supplier's card with the item, the order, the limit and the owner's cost.
    await menu(page).getByRole('link', { name: 'Compras' }).click()
    const card = page.getByRole('article', { name: bought.supplier })
    await expect(card).toBeVisible()
    await expect(card).toContainText('(11) 98765-4321')
    await expect(card).toContainText('1 item')
    await expect(card.getByRole('checkbox', { name: `1× ${bought.item}` })).toBeChecked()
    await expect(card.getByRole('link', { name: 'P-0001' })).toBeVisible()
    await expect(card.getByText(/Precisa chegar até \d{2}\/\d{2}\/\d{4}/)).toBeVisible()
    await expect(card.getByText(/Custo \(por unidade\)/)).toContainText(/180,00/)
    await expect(card.getByText('Os itens marcados ficam como Enviada ao fornecedor.')).toBeVisible()

    // Ver mensagem: the API's text, with the supplier and the store, and nothing of the client.
    await card.getByRole('button', { name: 'Ver mensagem' }).click()
    const preview = card.getByRole('region', { name: `Mensagem para ${bought.supplier}` })
    await expect(preview).toContainText('Compras Teste')
    await expect(preview).toContainText(`1× ${bought.item}`)
    const message = await preview.locator('pre').innerText()
    expect(message).toContain(bought.supplier)
    expect(message).not.toMatch(noClientData)
    await expect(card.getByRole('button', { name: 'Copiar mensagem' })).toBeVisible()

    // Enviar pelo WhatsApp: the app opens exactly the link the API answered, and calls WhatsApp for nothing.
    const sentResponse = page.waitForResponse((response) => response.request().method() === 'POST' && response.url().endsWith('/supplier-orders'))
    await card.getByRole('button', { name: 'Enviar pelo WhatsApp' }).click()
    const sentOrder = (await (await sentResponse).json()) as { whatsappUrl: string }
    await expect(page.getByRole('status').filter({ hasText: 'Pedido de compra C-0001 criado.' })).toBeVisible()
    await expect.poll(async () => (await whatsapp.opened()).length).toBe(1)
    const [supplierLink] = await whatsapp.opened()
    expect(supplierLink.startsWith(`https://wa.me/${bought.supplierPhone}`)).toBe(true)
    expect(supplierLink, 'the app opens exactly the link the API answered').toBe(sentOrder.whatsappUrl)
    const supplierText = new URL(supplierLink).searchParams.get('text') ?? ''
    expect(supplierText).toContain('C-0001')
    expect(supplierText).toContain(`1× ${bought.item}`)
    expect(supplierText).not.toMatch(noClientData)
    // The first send's words, as the design document gives them (Supplier messages).
    expect(supplierText).toContain(`Olá, ${bought.supplier}! Aqui é da Compras Teste.`)
    expect(supplierText).toContain('Pedido de compra C-0001:')
    expect(supplierText).toContain('Pode me confirmar a data de entrega?')
    await expect(page.getByText('Nada para comprar agora.')).toBeVisible()

    // The pedido de compra: the supplier, who sent it and the line.
    await page.getByRole('link', { name: 'Ver pedido de compra' }).click()
    await expect(page.getByRole('heading', { name: 'Pedido de compra C-0001' })).toBeVisible()
    await expect(page.getByText(/Enviado em \d{2}\/\d{2}\/\d{4} por Elisa Prado/)).toBeVisible()
    await expect(page.getByText('Prazo do fornecedor: Sem prazo')).toBeVisible()
    const lines = page.getByRole('table', { name: 'Itens do pedido de compra' })
    await expect(lines.getByRole('row', { name: new RegExp(bought.item) })).toContainText('Enviada ao fornecedor')
    // The owner sees the cost and may record the price paid.
    await expect(lines.getByRole('columnheader', { name: 'Custo (por unidade)' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Informar preço pago' })).toBeVisible()

    // Informar prazo: a date in the browser's clock.
    const promised = await inDays(page, 3)
    const [year, month, day] = promised.split('-')
    await page.getByRole('button', { name: 'Informar prazo' }).click()
    const dateDialog = page.getByRole('dialog', { name: 'Informar prazo' })
    await dateDialog.getByLabel('Data prometida pelo fornecedor').fill(promised)
    await dateDialog.getByRole('button', { name: 'Salvar prazo' }).click()
    await expect(page.getByRole('status').filter({ hasText: 'Prazo salvo. As datas previstas dos pedidos foram atualizadas.' })).toBeVisible()
    await expect(page.getByText(`Prazo do fornecedor: ${day}/${month}/${year}`)).toBeVisible()

    // Receber itens, narrowed to this pedido de compra: the item arrives defective, with a note and a photo.
    await page.getByRole('link', { name: 'Receber itens' }).click()
    await expect(page.getByRole('heading', { name: 'Receber itens', exact: true })).toBeVisible()
    await expect(page.getByText('Mostrando só o pedido de compra C-0001.')).toBeVisible()
    await expect(page.getByRole('heading', { name: `C-0001 · ${bought.supplier}` })).toBeVisible()
    await expect(page.getByText('Marque um item só quando toda a quantidade chegou.')).toBeVisible()
    await expect(page.getByText('Marque Tudo certo ou Com defeito em ao menos um item.')).toBeVisible()
    const save = page.getByRole('button', { name: 'Salvar recebimento' })
    await expect(save).toBeDisabled()
    await page.getByRole('button', { name: `Com defeito: ${bought.item}` }).click()
    // A defect needs its note.
    await save.click()
    await expect(page.getByText(`Conte o que houve com ${bought.item}.`)).toBeVisible()
    await page.getByLabel('O que houve?').fill('Costura aberta no encosto')
    await page.getByLabel(`Foto do item: ${bought.item}`).setInputFiles(itemPhoto)
    await expect(page.getByText('Foto 1', { exact: true })).toBeVisible()
    await expect(page.getByText('Enviando a foto…')).toHaveCount(0)
    await save.click()
    await expect(page.getByText('Recebimento salvo.')).toBeVisible()
    await expect(page.getByText(`${bought.item} voltou para A comprar. A reposição está em Compras.`)).toBeVisible()

    // Back in A comprar as "Reposição", and the pedido de compra says what happened.
    await menu(page).getByRole('link', { name: 'Compras' }).click()
    const again = page.getByRole('article', { name: bought.supplier })
    await expect(again.getByText('Reposição', { exact: true })).toBeVisible()
    await expect(again.getByRole('link', { name: 'P-0001' })).toBeVisible()
    await page.getByRole('tab', { name: 'Enviadas' }).click()
    await page.getByLabel('Todos', { exact: true }).check()
    await page.getByRole('link', { name: 'C-0001' }).click()
    await expect(lines.getByRole('row', { name: new RegExp(bought.item) })).toContainText('Com defeito (reposição em Compras)')
    await expect(lines).toContainText('Costura aberta no encosto')

    // Fornecedores: the supplier the item came with.
    await menu(page).getByRole('link', { name: 'Compras' }).click()
    await page.getByRole('tab', { name: 'Fornecedores' }).click()
    await expect(page.getByRole('row', { name: new RegExp(bought.supplier) })).toContainText('(11) 98765-4321')
    await expect(page.getByRole('button', { name: 'Novo fornecedor' })).toBeVisible()

    // The same item again: a second pedido de compra, then it arrives right.
    await menu(page).getByRole('link', { name: 'Compras' }).click()
    await page.getByRole('tab', { name: 'A comprar' }).click()
    await page.getByRole('article', { name: bought.supplier }).getByRole('button', { name: 'Enviar pelo WhatsApp' }).click()
    await expect(page.getByRole('status').filter({ hasText: 'Pedido de compra C-0002 criado.' })).toBeVisible()
    await expect.poll(async () => (await whatsapp.opened()).length).toBe(2)
    const replacementLink = (await whatsapp.opened())[1]
    expect(replacementLink.startsWith(`https://wa.me/${bought.supplierPhone}`)).toBe(true)
    // The replacement's message has no client data either.
    const replacementText = new URL(replacementLink).searchParams.get('text') ?? ''
    expect(replacementText).toContain('C-0002')
    expect(replacementText).toContain(`1× ${bought.item}`)
    expect(replacementText).not.toMatch(noClientData)
    await page.getByRole('tab', { name: 'Receber itens' }).click()
    await expect(page.getByRole('heading', { name: `C-0002 · ${bought.supplier}` })).toBeVisible()
    await page.getByRole('button', { name: `Tudo certo: ${bought.item}` }).click()
    await page.getByRole('button', { name: 'Salvar recebimento' }).click()
    await expect(page.getByText('Recebimento salvo.')).toBeVisible()
    await expect(page.getByText('Pedido P-0001: todos os itens chegaram e foram conferidos.')).toBeVisible()

    // The order opens from the receipt and moves into Montagem.
    await page.getByRole('link', { name: 'Abrir pedido P-0001' }).click()
    await expect(page.getByRole('heading', { name: 'Pedido P-0001' })).toBeVisible()
    await expect(page.getByRole('row', { name: new RegExp(`${bought.item}.*Recebida`) })).toBeVisible()
    await page.getByRole('button', { name: 'Mover para Em montagem' }).click()
    await expect(page.getByRole('heading', { name: 'Pedido P-0001' }).locator('xpath=..').getByText('Em montagem', { exact: true })).toBeVisible()

    // A second sold order, for the admin to see what is still to buy.
    const second = await placeOrder(api, bought.itemId, `${owner.slug}-2`, true)
    expect(second.number).toBe('P-0002')

    // An admin is invited through Equipe and sees Compras with no cost and no price paid.
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

      await menu(guest).getByRole('link', { name: 'Compras' }).click()
      const guestCard = guest.getByRole('article', { name: bought.supplier })
      await expect(guestCard.getByRole('checkbox', { name: `1× ${bought.item}` })).toBeVisible()
      await expect(guestCard.getByRole('link', { name: 'P-0002' })).toBeVisible()
      await expect(guestCard.getByText(/Precisa chegar até/)).toBeVisible()
      await expect(guest.getByText(/Custo/)).toHaveCount(0)
      await expect(guest.getByText('R$ 180,00')).toHaveCount(0)

      await guest.getByRole('tab', { name: 'Enviadas' }).click()
      await guest.getByLabel('Todos', { exact: true }).check()
      await guest.getByRole('link', { name: 'C-0001' }).click()
      await expect(guest.getByRole('heading', { name: 'Pedido de compra C-0001' })).toBeVisible()
      await expect(guest.getByRole('table', { name: 'Itens do pedido de compra' }).getByRole('row', { name: new RegExp(bought.item) })).toBeVisible()
      await expect(guest.getByRole('columnheader', { name: /Custo|Preço pago/ })).toHaveCount(0)
      await expect(guest.getByRole('button', { name: 'Informar preço pago' })).toHaveCount(0)
      await expect(guest.getByText(/Custo|Preço pago|R\$ 180,00/)).toHaveCount(0)
      expect(await guestWhatsapp.opened()).toEqual([])
      expectNoWhatsappRequests(guestWhatsapp)
    } finally {
      await guestContext.close()
    }

    // Nothing was sent automatically: two links opened, both the API's, and the browser asked WhatsApp for nothing.
    expect(await whatsapp.opened()).toHaveLength(2)
    expectNoWhatsappRequests(whatsapp)
  })
})
