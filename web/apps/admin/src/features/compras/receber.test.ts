import type { Permission, ToReceiveLine } from '@encomendo/api'
import { enableAutoUnmount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import { button, closeApps, field, link, openApp } from '../../testing/app'
import { authSession, gate, problem } from '../../testing/fixtures'
import { todayHandler } from '../../testing/today'
import { couros, newReceiving, orderIds, type Receiving, receiveLine, savedReceipt, serve, serveReceiving, supplierOrderId, toReceiveAnswer } from './testkit'

const server = setupServer()
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => {
  server.resetHandlers()
  vi.restoreAllMocks()
})
afterAll(() => server.close())
enableAutoUnmount(afterEach)
afterEach(closeApps)

const receiver: Permission[] = ['order:read', 'receiving:write']
const buyer: Permission[] = ['order:read', 'catalog:read', 'catalog:write', 'purchase:write', 'receiving:write', 'settings:read']

const signedInAs = (role: 'owner' | 'admin' | 'staff' | 'viewer', permissions: Permission[]) =>
  server.use(http.post('*/auth/refresh', () => HttpResponse.json(authSession({ role, permissions }))))

function use(state: Receiving = newReceiving()) {
  server.use(...serveReceiving(state), ...serve().handlers)
  return state
}

async function openReceber(path = '/compras/receber') {
  const app = await openApp(path)
  await vi.waitFor(() => expect(app.wrapper.text()).toContain('C-0012 · Couros Paulista'))
  return app
}

/** row finds the line of an item by its name. */
function row(wrapper: VueWrapper, name: string) {
  const found = wrapper.findAll('li.line').find((li) => li.get('p').text().includes(name))
  if (found === undefined) throw new Error(`no line for ${name}`)
  return found
}

const banco = 'Banco de couro CB 500X'
const capa = 'Capa de banco'

const png = (name = 'foto.png') => new File(['png-bytes'], name, { type: 'image/png' })

/** choose gives a file input the files a person picked and fires its change, as the browser does. */
type Input = { element: Element; trigger: (event: string) => Promise<void> }

async function pick(input: Input, ...files: File[]) {
  Object.defineProperty(input.element, 'files', { value: files, configurable: true })
  await input.trigger('change')
  await flushPromises()
}

/** choose picks files and waits until they are all up: the photos go up one after the other, over the network. */
async function choose(input: Input, ...files: File[]) {
  await pick(input, ...files)
  await vi.waitFor(() => expect(document.body.textContent).not.toContain('Enviando a foto…'))
}

/** invoice is the picker of "Foto da nota fiscal". */
const invoice = (wrapper: VueWrapper) => wrapper.get('.invoice')

/** keepOnlyLines changes what the fake API lists in its first group: what the next read of the list answers. */
function keepOnlyLines(state: Receiving, keep: (line: ToReceiveLine) => boolean) {
  const [first, ...rest] = state.toReceive.groups
  if (first === undefined) throw new Error('no group to change')
  state.toReceive = { ...state.toReceive, groups: [{ ...first, lines: first.lines.filter(keep) }, ...rest] }
}

const save = (wrapper: VueWrapper) => button(wrapper, 'Salvar recebimento')

beforeEach(() => {
  signedInAs('staff', receiver)
  vi.spyOn(window, 'open').mockReturnValue(null)
})

describe('the route and the menu entry', () => {
  it('gives a role that only receives the entry "Receber itens" in place of "Compras"', async () => {
    use()
    server.use(todayHandler())
    const { wrapper } = await openApp('/hoje')

    expect(wrapper.findAll('nav a.menu-link').map((a) => a.text())).toEqual(['Hoje', 'Pedidos', 'Receber itens', 'Clientes'])
    expect(link(wrapper, 'Receber itens').attributes('href')).toBe('/compras/receber')
  })

  it('gives a role that buys "Compras" and no separate entry, since Compras has the tab', async () => {
    signedInAs('admin', buyer)
    use()
    server.use(todayHandler())
    const { wrapper } = await openApp('/hoje')

    const entries = wrapper.findAll('nav a.menu-link').map((a) => a.text())
    expect(entries).toContain('Compras')
    expect(entries).not.toContain('Receber itens')
  })

  it('sends a role that cannot receive to Hoje', async () => {
    signedInAs('viewer', ['order:read'])
    use()
    server.use(todayHandler())

    const { router } = await openApp('/compras/receber')

    expect(router.currentRoute.value.name).toBe('hoje')
  })

  it('also shows the screen inside the "Receber itens" tab of Compras', async () => {
    signedInAs('admin', buyer)
    use()

    const { wrapper } = await openApp('/compras?tab=receber')
    await vi.waitFor(() => expect(wrapper.text()).toContain('C-0012 · Couros Paulista'))

    expect(wrapper.get('[role=tab][aria-selected=true]').text()).toBe('Receber itens')
    expect(wrapper.findAll('h1').map((h) => h.text())).toEqual(['Compras'])
    expect(button(wrapper, 'Salvar recebimento').exists()).toBe(true)
  })
})

describe('the list', () => {
  it('says it is loading', async () => {
    const release = gate()
    server.use(
      http.get('*/api/v1/admin/purchasing/to-receive', async () => {
        await release.opened
        return HttpResponse.json(toReceiveAnswer())
      }),
    )
    use()
    server.use(
      http.get('*/api/v1/admin/purchasing/to-receive', async () => {
        await release.opened
        return HttpResponse.json(toReceiveAnswer())
      }),
    )

    const { wrapper } = await openApp('/compras/receber')

    expect(wrapper.text()).toContain('Carregando o que falta receber…')
    release.open()
    await vi.waitFor(() => expect(wrapper.text()).toContain('C-0012'))
  })

  it('groups the lines by pedido de compra, then the items bought at the counter', async () => {
    use()
    const { wrapper } = await openReceber()

    expect(wrapper.get('h1').text()).toBe('Receber itens')
    expect(wrapper.findAll('h2').map((h) => h.text())).toEqual(['C-0012 · Couros Paulista', 'C-0013 · Moto Peças', 'Comprados sem pedido de compra'])
    const first = row(wrapper, banco).text().replace(/\s+/g, ' ')
    expect(first).toContain('2× Banco de couro CB 500X')
    expect(first).toContain('Cor: Preto')
    expect(first).toContain('Pedido P-0231')
    expect(wrapper.text()).toContain('Marque um item só quando toda a quantidade chegou.')
  })

  it('says there is nothing to receive', async () => {
    use(newReceiving({ toReceive: { groups: [], toBuy: [], truncated: false } }))
    const { wrapper } = await openApp('/compras/receber')

    await vi.waitFor(() => expect(wrapper.text()).toContain('Nada para receber agora.'))
    expect(wrapper.findAll('button').map((b) => b.text())).not.toContain('Salvar recebimento')
  })

  it('narrows by item, order or supplier', async () => {
    use()
    const { wrapper } = await openReceber()
    const search = field(wrapper, 'Buscar item, pedido ou fornecedor')

    await search.setValue('moto')
    expect(wrapper.findAll('h2').map((h) => h.text())).toEqual(['C-0013 · Moto Peças'])

    await search.setValue('P-0232')
    expect(wrapper.findAll('h2').map((h) => h.text())).toEqual(['C-0012 · Couros Paulista'])
    expect(wrapper.text()).toContain(capa)
    expect(wrapper.text()).not.toContain(banco)

    await search.setValue('parafuso')
    expect(wrapper.findAll('h2').map((h) => h.text())).toEqual(['Comprados sem pedido de compra'])

    await search.setValue('nada disso')
    expect(wrapper.text()).toContain('Nenhum item encontrado.')
  })

  it('offers a way to try again when the list cannot be read', async () => {
    use()
    server.use(http.get('*/api/v1/admin/purchasing/to-receive', () => problem(500, 'internal', 'Erro')))
    const { wrapper } = await openApp('/compras/receber')

    await vi.waitFor(() => expect(wrapper.text()).toContain('O servidor não respondeu'))
    server.use(...serveReceiving(newReceiving()))
    await button(wrapper, 'Tentar de novo').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('C-0012 · Couros Paulista'))
  })

  it('warns when the list was cut', async () => {
    use(newReceiving({ toReceive: { ...toReceiveAnswer(), truncated: true } }))
    const { wrapper } = await openReceber()

    expect(wrapper.text()).toContain('A lista está cortada.')
  })
})

describe('narrowed to one pedido de compra', () => {
  const received = '0190f3c4-5e1a-7b2c-8d3e-000000005099'

  async function openNarrowed(id: string) {
    use()
    const app = await openApp(`/compras/receber?pedido=${id}`)
    await vi.waitFor(() => expect(app.wrapper.text()).toContain('Mostrar todos'))
    return app
  }

  it('shows only that pedido de compra, with its number, and not the counter items', async () => {
    const { wrapper } = await openNarrowed(supplierOrderId)

    expect(wrapper.text()).toContain('Mostrando só o pedido de compra C-0012.')
    expect(wrapper.findAll('h2').map((h) => h.text())).toEqual(['C-0012 · Couros Paulista'])
    expect(wrapper.text()).not.toContain('Pastilha de freio')
    expect(wrapper.text()).not.toContain('Parafuso M8')
  })

  it('shows the others again on "Mostrar todos", keeping what was checked', async () => {
    const { wrapper, router } = await openNarrowed(supplierOrderId)
    await button(row(wrapper, banco), 'Tudo certo').trigger('click')

    await button(wrapper, 'Mostrar todos').trigger('click')
    await vi.waitFor(() => expect(wrapper.findAll('h2')).toHaveLength(3))

    expect(router.currentRoute.value.query.pedido).toBeUndefined()
    expect(wrapper.text()).not.toContain('Mostrando só')
    expect(button(row(wrapper, banco), 'Tudo certo').attributes('aria-pressed')).toBe('true')
  })

  it('keeps the Compras tab when it narrows inside the tab', async () => {
    signedInAs('admin', buyer)
    use()
    const { wrapper, router } = await openApp(`/compras?tab=receber&pedido=${supplierOrderId}`)
    await vi.waitFor(() => expect(wrapper.text()).toContain('Mostrando só'))

    await button(wrapper, 'Mostrar todos').trigger('click')
    await vi.waitFor(() => expect(wrapper.findAll('[role=tabpanel] h2')).toHaveLength(3))

    expect(router.currentRoute.value.query).toEqual({ tab: 'receber' })
  })

  it('says so when that pedido de compra has nothing left to receive, with a way to see the rest', async () => {
    const { wrapper } = await openNarrowed(received)

    expect(wrapper.text()).toContain('Este pedido de compra não tem itens para receber.')
    expect(wrapper.findAll('h2')).toHaveLength(0)
    expect(button(wrapper, 'Mostrar todos').exists()).toBe(true)
  })
})

describe('Tudo certo and Com defeito', () => {
  it('keeps "Salvar recebimento" off, with its hint, until a line is checked', async () => {
    use()
    const { wrapper } = await openReceber()

    expect(save(wrapper).attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('Marque Tudo certo ou Com defeito em ao menos um item.')

    await button(row(wrapper, banco), 'Tudo certo').trigger('click')

    expect(save(wrapper).attributes('disabled')).toBeUndefined()
    expect(wrapper.text()).not.toContain('Marque Tudo certo ou Com defeito em ao menos um item.')
  })

  it('shows the check as pressed, and the same button again takes it back', async () => {
    use()
    const { wrapper } = await openReceber()
    const line = row(wrapper, banco)

    await button(line, 'Tudo certo').trigger('click')
    expect(button(line, 'Tudo certo').attributes('aria-pressed')).toBe('true')
    expect(button(line, 'Com defeito').attributes('aria-pressed')).toBe('false')

    await button(line, 'Tudo certo').trigger('click')
    expect(button(line, 'Tudo certo').attributes('aria-pressed')).toBe('false')
    expect(save(wrapper).attributes('disabled')).toBeDefined()
  })

  it('offers "Foto do item" after a check, and "O que houve?" only for a defect', async () => {
    use()
    const { wrapper } = await openReceber()
    const line = row(wrapper, banco)
    expect(line.findAll('label')).toHaveLength(0)

    await button(line, 'Tudo certo').trigger('click')
    expect(line.findAll('label').map((l) => l.text())).toEqual(['Foto do item'])

    await button(line, 'Com defeito').trigger('click')
    expect(line.findAll('label').map((l) => l.text())).toEqual(['O que houve?', 'Foto do item'])
  })

  it('names each button by its item for a screen reader', async () => {
    use()
    const { wrapper } = await openReceber()

    expect(wrapper.find(`button[aria-label="Tudo certo: ${capa}"]`).exists()).toBe(true)
    expect(wrapper.find(`button[aria-label="Com defeito: ${capa}"]`).exists()).toBe(true)
  })
})

describe('Salvar recebimento', () => {
  it('refuses a defect without its note before any call, and sends it once the note is there', async () => {
    const state = use()
    const { wrapper } = await openReceber()
    const line = row(wrapper, banco)
    await button(line, 'Com defeito').trigger('click')

    await save(wrapper).trigger('click')
    await flushPromises()

    expect(state.receipts).toHaveLength(0)
    expect(line.get('[role=alert]').text()).toBe(`Conte o que houve com ${banco}.`)
    expect(line.get('textarea').attributes('aria-invalid')).toBe('true')

    await field(line, 'O que houve?').setValue('Veio riscado')
    expect(line.find('[role=alert]').exists()).toBe(false)
    await save(wrapper).trigger('click')
    await flushPromises()

    expect(state.receipts).toHaveLength(1)
  })

  it('treats a note of spaces as no note', async () => {
    const state = use()
    const { wrapper } = await openReceber()
    const line = row(wrapper, banco)
    await button(line, 'Com defeito').trigger('click')
    await field(line, 'O que houve?').setValue('   ')

    await save(wrapper).trigger('click')
    await flushPromises()

    expect(state.receipts).toHaveLength(0)
    expect(line.get('[role=alert]').text()).toContain('Conte o que houve')
  })

  it('sends each line by its lineId, the counter item by its orderItemId, the notes of defects only, and the key', async () => {
    const state = use()
    const { wrapper } = await openReceber()
    await button(row(wrapper, banco), 'Tudo certo').trigger('click')
    await button(row(wrapper, capa), 'Com defeito').trigger('click')
    await field(row(wrapper, capa), 'O que houve?').setValue('  Veio sem a costura  ')
    await button(row(wrapper, 'Parafuso M8'), 'Tudo certo').trigger('click')

    await save(wrapper).trigger('click')
    await flushPromises()

    const sent = state.receipts[0]
    expect(sent?.key).toMatch(/^[0-9a-f-]{36}$/)
    expect(sent?.body).toEqual({
      entries: [
        { lineId: receiveLine(1).lineId, check: 'ok' },
        { lineId: receiveLine(2).lineId, check: 'defective', note: 'Veio sem a costura' },
        { orderItemId: receiveLine(4, {}, true).orderItemId, check: 'ok' },
      ],
    })
  })

  it('drops the note of a line that went back to Tudo certo', async () => {
    const state = use()
    const { wrapper } = await openReceber()
    const line = row(wrapper, banco)
    await button(line, 'Com defeito').trigger('click')
    await field(line, 'O que houve?').setValue('Riscado')
    await button(line, 'Tudo certo').trigger('click')

    await save(wrapper).trigger('click')
    await flushPromises()

    expect(state.receipts[0]?.body.entries).toEqual([{ lineId: receiveLine(1).lineId, check: 'ok' }])
  })

  it('says what the receipt did: saved, back to A comprar, and the orders it completed with "Abrir pedido"', async () => {
    const state = use()
    state.receipt = savedReceipt({
      backToBuy: [{ orderItemId: receiveLine(1).orderItemId, name: banco }],
      completedOrders: [{ orderId: orderIds.second, orderNumber: 'P-0232' }],
    })
    const { wrapper, router } = await openReceber()
    await button(row(wrapper, banco), 'Com defeito').trigger('click')
    await field(row(wrapper, banco), 'O que houve?').setValue('Riscado')
    keepOnlyLines(state, () => false)

    await save(wrapper).trigger('click')
    await flushPromises()

    const result = wrapper.get('.result')
    expect(result.text()).toContain('Recebimento salvo.')
    expect(result.text()).toContain(`${banco} voltou para A comprar. A reposição está em Compras.`)
    expect(result.text()).toContain('Pedido P-0232: todos os itens chegaram e foram conferidos.')
    const open = link(result, 'Abrir pedido')
    expect(open.attributes('href')).toBe(`/pedidos/${orderIds.second}`)
    await open.trigger('click')
    await vi.waitFor(() => expect(router.currentRoute.value.name).toBe('order'))
  })

  it('clears the checks after a save and reads the list again', async () => {
    const state = use()
    const { wrapper } = await openReceber()
    const reads = state.reads
    await button(row(wrapper, banco), 'Tudo certo').trigger('click')

    await save(wrapper).trigger('click')
    await vi.waitFor(() => expect(state.reads).toBe(reads + 1))
    await flushPromises()

    expect(button(row(wrapper, banco), 'Tudo certo').attributes('aria-pressed')).toBe('false')
    expect(save(wrapper).attributes('disabled')).toBeDefined()
  })

  it('does not send twice while a save is on its way', async () => {
    const state = use()
    const release = gate()
    state.answers.receipt = async () => {
      await release.opened
      return HttpResponse.json(state.receipt, { status: 201 })
    }
    const { wrapper } = await openReceber()
    await button(row(wrapper, banco), 'Tudo certo').trigger('click')

    await save(wrapper).trigger('click')
    await flushPromises()
    expect(save(wrapper).attributes('disabled')).toBeDefined()
    await save(wrapper).trigger('click')
    release.open()
    await flushPromises()

    expect(state.receipts).toHaveLength(1)
  })

  describe('when the save fails', () => {
    async function failedSave() {
      const state = use()
      state.answers.receipt = () => HttpResponse.error()
      const app = await openReceber()
      const line = row(app.wrapper, banco)
      await button(line, 'Com defeito').trigger('click')
      await field(line, 'O que houve?').setValue('Riscado')
      await choose(line.get('input[type=file]'), png())
      await choose(invoice(app.wrapper).get('input[type=file]'), png('nota.png'))
      await save(app.wrapper).trigger('click')
      await flushPromises()
      return { ...app, state, line }
    }

    it('keeps every check, note and photo, and says why', async () => {
      const { wrapper, state, line } = await failedSave()

      expect(wrapper.get('.save [role=alert]').text()).toContain('Sem conexão com o servidor')
      expect(button(line, 'Com defeito').attributes('aria-pressed')).toBe('true')
      expect((line.get('textarea').element as HTMLTextAreaElement).value).toBe('Riscado')
      expect(line.findAll('li').map((li) => li.text())).toEqual([expect.stringContaining('Foto 1')])
      expect(invoice(wrapper).text()).toContain('Foto 1')
      expect(state.receipts).toHaveLength(1)
    })

    it('"Tentar de novo" sends the same save with the same Idempotency-Key and the same photos', async () => {
      const { wrapper, state } = await failedSave()
      state.answers.receipt = undefined

      await button(wrapper, 'Tentar de novo').trigger('click')
      await flushPromises()

      expect(state.receipts).toHaveLength(2)
      expect(state.receipts[1]?.key).toBe(state.receipts[0]?.key)
      expect(state.receipts[1]?.body).toEqual(state.receipts[0]?.body)
      expect(state.receipts[0]?.body.invoicePhotoKeys).toHaveLength(1)
      expect(wrapper.text()).toContain('Recebimento salvo.')
      expect(wrapper.text()).not.toContain('Tentar de novo')
    })

    it('makes a new key when the receipt changed before the retry', async () => {
      const { wrapper, state } = await failedSave()
      state.answers.receipt = undefined
    await button(row(wrapper, capa), 'Tudo certo').trigger('click')
      // The old message and "Tentar de novo" go: this is a different receipt now.
      expect(wrapper.text()).not.toContain('Tentar de novo')

      await save(wrapper).trigger('click')
      await flushPromises()

      expect(state.receipts[1]?.key).not.toBe(state.receipts[0]?.key)
      expect(state.receipts[1]?.body.entries).toHaveLength(2)
    })

    it('makes a new key for the next receipt, after a success', async () => {
      const { wrapper, state } = await failedSave()
      state.answers.receipt = undefined
      await button(wrapper, 'Tentar de novo').trigger('click')
      await flushPromises()
      await button(row(wrapper, banco), 'Tudo certo').trigger('click')

      await save(wrapper).trigger('click')
      await flushPromises()

      expect(state.receipts).toHaveLength(3)
      expect(state.receipts[2]?.key).not.toBe(state.receipts[0]?.key)
    })
  })

  it('reads the list again when a line was already received, and drops its check', async () => {
    const state = use()
    state.answers.receipt = () => problem(409, 'line_closed', 'Linha fechada')
    const { wrapper } = await openReceber()
    await button(row(wrapper, banco), 'Tudo certo').trigger('click')
    await button(row(wrapper, capa), 'Tudo certo').trigger('click')
    const reads = state.reads
    keepOnlyLines(state, (line) => line.name === capa)

    await save(wrapper).trigger('click')
    await flushPromises()

    expect(wrapper.get('.save [role=alert]').text()).toContain('A lista foi atualizada')
    await vi.waitFor(() => expect(state.reads).toBe(reads + 1))
    await flushPromises()
    expect(wrapper.text()).not.toContain(banco)
    expect(button(row(wrapper, capa), 'Tudo certo').attributes('aria-pressed')).toBe('true')
  })
})

describe('photos', () => {
  async function checkedLine() {
    const state = use()
    const app = await openReceber()
    const line = row(app.wrapper, banco)
    await button(line, 'Tudo certo').trigger('click')
    return { ...app, state, line, input: () => line.get('input[type=file]') }
  }

  it('takes any image, so the phone offers the camera or the gallery', async () => {
    const { line, wrapper } = await checkedLine()

    expect(line.get('input[type=file]').attributes('accept')).toBe('image/*')
    expect(field(line, 'Foto do item').element).toBe(line.get('input[type=file]').element)
    expect(field(wrapper, 'Foto da nota fiscal').attributes('accept')).toBe('image/*')
  })

  it('uploads the photo, names its key in the receipt, and says "Enviando a foto…" meanwhile', async () => {
    const { wrapper, state, input, line } = await checkedLine()
    const release = gate()
    state.answers.storage = async () => {
      await release.opened
      return new HttpResponse(null, { status: 200 })
    }

    await pick(input(), png())
    expect(line.text()).toContain('Enviando a foto…')
    expect(save(wrapper).attributes('disabled')).toBeDefined()
    release.open()
    await flushPromises()
    expect(line.text()).not.toContain('Enviando a foto…')
    await save(wrapper).trigger('click')
    await flushPromises()

    expect(state.uploads).toEqual(['image/png'])
    expect(state.receipts[0]?.body.entries).toEqual([{ lineId: receiveLine(1).lineId, check: 'ok', photoKeys: ['t/tenant/receipts/photo-1'] }])
  })

  it('sends the invoice photos once per save', async () => {
    const { wrapper, state } = await checkedLine()

    await choose(invoice(wrapper).get('input[type=file]'), png('nota-1.png'), png('nota-2.png'))
    await save(wrapper).trigger('click')
    await flushPromises()

    expect(state.receipts[0]?.body.invoicePhotoKeys).toEqual(['t/tenant/receipts/photo-1', 't/tenant/receipts/photo-2'])
  })

  it('removes a photo', async () => {
    const { wrapper, state, input, line } = await checkedLine()
    await choose(input(), png())

    await button(line, 'Remover foto').trigger('click')
    await save(wrapper).trigger('click')
    await flushPromises()

    expect(line.text()).not.toContain('Foto 1')
    expect(state.receipts[0]?.body.entries).toEqual([{ lineId: receiveLine(1).lineId, check: 'ok' }])
  })

  it('refuses a file that is not a JPEG, PNG or WebP, and one over 10 MB, before any request', async () => {
    const { state, input, line } = await checkedLine()

    await choose(input(), new File(['x'], 'figura.gif', { type: 'image/gif' }))
    expect(line.get('[role=alert]').text()).toBe('Use uma foto JPEG, PNG ou WebP de até 10 MB.')

    await choose(input(), new File([new Uint8Array(10 * 1024 * 1024 + 1)], 'grande.jpg', { type: 'image/jpeg' }))
    expect(line.get('[role=alert]').text()).toBe('Use uma foto JPEG, PNG ou WebP de até 10 MB.')
    expect(state.uploads).toEqual([])
    expect(line.text()).not.toContain('Foto 1')
  })

  it('says "A foto não foi enviada. Tente de novo." when storage refuses it, and keeps no photo', async () => {
    const { state, input, line } = await checkedLine()
    state.answers.storage = () => new HttpResponse(null, { status: 403 })

    await choose(input(), png())

    expect(line.get('[role=alert]').text()).toBe('A foto não foi enviada. Tente de novo.')
    expect(line.text()).not.toContain('Foto 1')
  })

  it('says it too when the server will not take the upload request, with its own wording for a limit', async () => {
    const { state, input, line } = await checkedLine()
    state.answers.upload = () => problem(429, 'rate_limited', 'Muitas tentativas', { 'Retry-After': '30' })

    await choose(input(), png())

    expect(line.get('[role=alert]').text()).toBe('Muitas tentativas. Tente de novo em 30 segundos.')
  })

  it('forgets the refusal when another photo is chosen', async () => {
    const { state, input, line } = await checkedLine()
    state.answers.storage = () => new HttpResponse(null, { status: 403 })
    await choose(input(), png())
    state.answers.storage = undefined

    await choose(input(), png())

    expect(line.find('[role=alert]').exists()).toBe(false)
    expect(line.text()).toContain('Foto 1')
  })

  it('takes at most 2 photos for an item', async () => {
    const { state, input, line } = await checkedLine()

    await choose(input(), png('a.png'), png('b.png'), png('c.png'))

    expect(line.findAll('.photos li')).toHaveLength(2)
    expect(line.get('[role=alert]').text()).toBe('Cada item pode ter até 2 fotos.')
    expect(state.uploads).toHaveLength(2)
  })

  it('takes at most 4 photos of the invoice', async () => {
    const { wrapper, state } = await checkedLine()
    const input = () => invoice(wrapper).get('input[type=file]')

    await choose(input(), png('1.png'), png('2.png'), png('3.png'))
    await choose(input(), png('4.png'), png('5.png'))

    expect(invoice(wrapper).findAll('.photos li')).toHaveLength(4)
    expect(invoice(wrapper).get('[role=alert]').text()).toBe('Cada recebimento pode ter até 4 fotos da nota.')
    expect(state.uploads).toHaveLength(4)
  })

  it('holds at most 20 photos in all', async () => {
    const state = use(
      newReceiving({
        toReceive: {
          groups: [{ supplierOrderId, number: 'C-0012', supplierName: couros.name, lines: Array.from({ length: 10 }, (_, n) => receiveLine(n + 1, { name: `Item ${n + 1}` })) }],
          toBuy: [],
          truncated: false,
        },
      }),
    )
    const { wrapper } = await openReceber()
    for (let n = 1; n <= 10; n += 1) {
      const line = row(wrapper, `Item ${n}`)
      await button(line, 'Tudo certo').trigger('click')
      await choose(line.get('input[type=file]'), png(), png())
    }
    expect(state.uploads).toHaveLength(20)

    await choose(invoice(wrapper).get('input[type=file]'), png())

    expect(invoice(wrapper).get('[role=alert]').text()).toBe('Cada recebimento pode ter até 20 fotos no total.')
    expect(state.uploads).toHaveLength(20)
  })
})

describe('workshop sizes', () => {
  // The sizes themselves are CSS (tokens.test.ts in packages/ui, and the Playwright phone project at 390 × 844);
  // these are the structures they rely on: the tier on the root, big real buttons, one primary.
  it('puts the enc-workshop tier on the screen and makes "Salvar recebimento" its one primary button', async () => {
    use()
    const { wrapper } = await openReceber()

    expect(wrapper.get('section.receber').classes()).toContain('enc-workshop')
    const primaries = wrapper.findAll('button.enc-button--primary')
    expect(primaries.map((b) => b.text())).toEqual(['Salvar recebimento'])
  })

  it('keeps the tier when the tab shows the screen inside Compras', async () => {
    signedInAs('admin', buyer)
    use()
    const { wrapper } = await openApp('/compras?tab=receber')
    await vi.waitFor(() => expect(wrapper.text()).toContain('C-0012'))

    expect(wrapper.get('[role=tabpanel] section.receber').classes()).toContain('enc-workshop')
  })

  it('shows every action as a labelled button or field, at 390 px as at any width', async () => {
    window.innerWidth = 390
    use()
    const { wrapper } = await openReceber()
    await button(row(wrapper, banco), 'Com defeito').trigger('click')

    const line = row(wrapper, banco)
    expect(line.findAll('button').map((b) => b.text())).toEqual(['Tudo certo', 'Com defeito'])
    expect(field(line, 'O que houve?').element.tagName).toBe('TEXTAREA')
    expect(field(line, 'Foto do item').attributes('type')).toBe('file')
    expect(field(wrapper, 'Foto da nota fiscal').attributes('type')).toBe('file')
    expect(wrapper.findAll('table')).toHaveLength(0)
  })
})
