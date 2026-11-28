import type { Permission } from '@encomendo/api'
import { enableAutoUnmount, flushPromises } from '@vue/test-utils'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import { button, closeApps, field, openApp } from '../../testing/app'
import { authSession, problem } from '../../testing/fixtures'
import { couros, motoPecas, noticeFor, serve, toBuyItem } from './testkit'

const server = setupServer()
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => {
  server.resetHandlers()
  vi.restoreAllMocks()
})
afterAll(() => server.close())
enableAutoUnmount(afterEach)
afterEach(closeApps)

const buyer: Permission[] = ['order:read', 'catalog:read', 'catalog:write', 'purchase:write', 'settings:read']

beforeEach(() => {
  server.use(http.post('*/auth/refresh', () => HttpResponse.json(authSession({ role: 'admin', permissions: buyer }))))
  vi.spyOn(window, 'open').mockReturnValue(null)
})

/** use answers the reads of Compras; `extra` handlers go first, so a test can change one answer. */
function use(backend: Parameters<typeof serve>[0] = {}, ...extra: Parameters<typeof server.use>) {
  const { handlers } = serve(backend)
  server.use(...extra, ...handlers)
}

async function openCompras(path: string, waitFor: string) {
  const app = await openApp(path)
  await vi.waitFor(() => expect(app.wrapper.text()).toContain(waitFor))
  return app
}

describe('the message of "Ver mensagem"', () => {
  it('closes when the button is pressed again', async () => {
    use()
    const { wrapper } = await openCompras('/compras', 'Couros Paulista')
    const first = wrapper.findAll('article')[0]
    if (first === undefined) throw new Error('no card')

    await button(first, 'Ver mensagem').trigger('click')
    await vi.waitFor(() => expect(first.find('pre').exists()).toBe(true))
    await button(first, 'Ver mensagem').trigger('click')

    expect(first.find('pre').exists()).toBe(false)
  })

  it('says why the preview failed, in the words of the refusal', async () => {
    use({}, http.post('*/api/v1/admin/purchasing/supplier-orders/preview', () => problem(409, 'item_not_to_buy', 'x')))
    const { wrapper } = await openCompras('/compras', 'Couros Paulista')
    const first = wrapper.findAll('article')[0]
    if (first === undefined) throw new Error('no card')

    await button(first, 'Ver mensagem').trigger('click')

    await vi.waitFor(() => expect(first.get('[role=alert]').text()).toBe('Algum item marcado já não está em A comprar. A lista foi atualizada; confira e envie de novo.'))
  })

  it('closes with nothing to show when every box is taken off', async () => {
    use()
    const { wrapper } = await openCompras('/compras', 'Moto Peças')
    const moto = wrapper.findAll('article')[1]
    if (moto === undefined) throw new Error('no card')
    await button(moto, 'Ver mensagem').trigger('click')
    await vi.waitFor(() => expect(moto.find('pre').exists()).toBe(true))

    await field(moto, '4× Pastilha de freio').setValue(false)
    await flushPromises()

    expect(moto.find('pre').exists()).toBe(false)
  })
})

describe('reads that fail', () => {
  it('says so on "Enviadas"', async () => {
    use({}, http.get('*/api/v1/admin/purchasing/supplier-orders', () => new HttpResponse(null, { status: 500 })))

    const { wrapper } = await openCompras('/compras?tab=enviadas', 'O servidor não respondeu.')

    expect(wrapper.find('table').exists()).toBe(false)
  })

  it('says so on "Fornecedores"', async () => {
    use({}, http.get('*/api/v1/admin/suppliers', () => new HttpResponse(null, { status: 500 })))

    await openCompras('/compras?tab=fornecedores', 'O servidor não respondeu.')
  })

  it('lists the cards even when the notices cannot be read', async () => {
    use({}, http.get('*/api/v1/admin/purchasing/notices', () => new HttpResponse(null, { status: 500 })))

    const { wrapper } = await openCompras('/compras', 'Couros Paulista')

    expect(wrapper.find('[aria-label="Avisar o fornecedor"]').exists()).toBe(false)
  })

  it('lists the cards even when the settings cannot be read', async () => {
    use({}, http.get('*/api/v1/admin/settings', () => new HttpResponse(null, { status: 500 })))

    await openCompras('/compras', 'Couros Paulista')
  })

  it('says so in the supplier drawer when its items cannot be read', async () => {
    use({ suppliers: [{ id: couros.id, name: couros.name, whatsappPhone: couros.phone, version: 1 }] }, http.get('*/api/v1/admin/suppliers/:id/items', () => new HttpResponse(null, { status: 500 })))
    const { wrapper } = await openCompras('/compras?tab=fornecedores', 'Couros Paulista')

    await button(wrapper, 'Couros Paulista').trigger('click')

    await vi.waitFor(() => expect(wrapper.get('[role=dialog] [role=alert]').text()).toBe('O servidor não respondeu. Tente de novo em instantes.'))
  })

  it('says "Nenhum item usa este fornecedor." for a supplier no item uses', async () => {
    use({ suppliers: [{ id: couros.id, name: couros.name, whatsappPhone: couros.phone, version: 1 }] })
    const { wrapper } = await openCompras('/compras?tab=fornecedores', 'Couros Paulista')

    await button(wrapper, 'Couros Paulista').trigger('click')

    await vi.waitFor(() => expect(wrapper.get('[role=dialog]').text()).toContain('Nenhum item usa este fornecedor.'))
  })
})

describe('Fornecedores paging and search', () => {
  it('pages with "Carregar mais" and says when a search finds nothing', async () => {
    use({}, http.get('*/api/v1/admin/suppliers', ({ request }) => {
      const url = new URL(request.url)
      if (url.searchParams.get('q') === 'zzz') return HttpResponse.json({ items: [] })
      return url.searchParams.get('cursor') === null
        ? HttpResponse.json({ items: [{ id: couros.id, name: couros.name, whatsappPhone: couros.phone, version: 1 }], nextCursor: 'n1' })
        : HttpResponse.json({ items: [{ id: motoPecas.id, name: motoPecas.name, whatsappPhone: motoPecas.phone, version: 1 }] })
    }))
    const { wrapper } = await openCompras('/compras?tab=fornecedores', 'Couros Paulista')

    await button(wrapper, 'Carregar mais').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Moto Peças'))
    expect(wrapper.findAll('button').map((b) => b.text())).not.toContain('Carregar mais')

    await field(wrapper, 'Buscar fornecedor').setValue('zzz')
    await wrapper.get('form[role=search]').trigger('submit')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Nenhum fornecedor encontrado.'))
  })

  it('searches after a pause in typing', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      const queries: (string | null)[] = []
      use({}, http.get('*/api/v1/admin/suppliers', ({ request }) => {
        queries.push(new URL(request.url).searchParams.get('q'))
        return HttpResponse.json({ items: [] })
      }))
      const { wrapper } = await openCompras('/compras?tab=fornecedores', 'Buscar fornecedor')

      await field(wrapper, 'Buscar fornecedor').setValue('mo')
      await vi.advanceTimersByTimeAsync(400)
      await flushPromises()

      expect(queries.at(-1)).toBe('mo')
    } finally {
      vi.useRealTimers()
    }
  })
})

describe('a pedido de compra opened from another one', () => {
  it('reads the new one when the address changes', async () => {
    const other = '0190f3c4-5e1a-7b2c-8d3e-000000005009'
    use({}, http.get('*/api/v1/admin/purchasing/supplier-orders/:id', ({ params }) => HttpResponse.json({ ...(params.id === other ? { number: 'C-0009' } : { number: 'C-0012' }), id: params.id, supplierId: couros.id, supplierName: couros.name, sentAt: '2026-11-27T15:00:00Z', promisedDate: null, received: 0, total: 0, state: 'sent', sentBy: { id: other, name: 'Ana' }, whatsappUrl: 'https://wa.me/1', lines: [], canUndo: true, version: 1 })))
    const { wrapper, router } = await openCompras(`/compras/pedidos/${toBuyItem(1).orderId}`, 'C-0012')

    await router.push(`/compras/pedidos/${other}`)
    await vi.waitFor(() => expect(wrapper.get('h1').text()).toBe('Pedido de compra C-0009'))
  })
})

describe('a notice resolved while the list was stale', () => {
  it('reads the notices again after the refusal', async () => {
    let reads = 0
    use(
      {},
      http.get('*/api/v1/admin/purchasing/notices', () => {
        reads += 1
        return HttpResponse.json({ items: reads === 1 ? [noticeFor(1)] : [] })
      }),
      http.post('*/api/v1/admin/purchasing/notices/:lineId/resolve', () => problem(409, 'line_closed', 'x')),
    )
    const { wrapper } = await openCompras('/compras', 'O pedido P-0231 foi cancelado.')

    await button(wrapper, 'Marcar como resolvido').trigger('click')

    await vi.waitFor(() => expect(wrapper.text()).not.toContain('O pedido P-0231 foi cancelado.'))
    expect(reads).toBe(2)
  })
})
