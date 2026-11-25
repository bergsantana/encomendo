import { describe, expect, it, vi } from 'vitest'

import { usePagination } from './usePagination'

type Page = { items: string[]; nextCursor?: string }

function pages(...answers: Page[]) {
  const fetchPage = vi.fn(async (cursor?: string) => {
    void cursor
    const answer = answers.shift()
    if (answer === undefined) throw new Error('no more pages in this test')
    return answer
  })
  return fetchPage
}

describe('usePagination', () => {
  it('loads the first page and says whether there is more', async () => {
    const fetchPage = pages({ items: ['a', 'b'], nextCursor: 'c1' })
    const list = usePagination(fetchPage)

    await list.reload()

    expect(list.items.value).toEqual(['a', 'b'])
    expect(list.hasMore.value).toBe(true)
    expect(fetchPage).toHaveBeenCalledWith(undefined)
  })

  it('appends the next page from the cursor, until the last one', async () => {
    const fetchPage = pages({ items: ['a'], nextCursor: 'c1' }, { items: ['b'] })
    const list = usePagination(fetchPage)
    await list.reload()

    await list.loadMore()

    expect(list.items.value).toEqual(['a', 'b'])
    expect(list.hasMore.value).toBe(false)
    expect(fetchPage).toHaveBeenLastCalledWith('c1')
  })

  it('starts over on reload', async () => {
    const list = usePagination(pages({ items: ['a'], nextCursor: 'c1' }, { items: ['z'] }))
    await list.reload()

    await list.reload()

    expect(list.items.value).toEqual(['z'])
  })

  it('does nothing more on the last page', async () => {
    const fetchPage = pages({ items: ['a'] })
    const list = usePagination(fetchPage)
    await list.reload()

    await list.loadMore()

    expect(fetchPage).toHaveBeenCalledTimes(1)
  })

  it('says it is loading while a page is on its way', async () => {
    let answer: (page: Page) => void = () => undefined
    const list = usePagination(() => new Promise<Page>((resolve) => (answer = resolve)))

    const loading = list.reload()
    expect(list.loading.value).toBe(true)
    answer({ items: [] })
    await loading

    expect(list.loading.value).toBe(false)
  })

  it('stops loading when a page fails, and lets the error through', async () => {
    const list = usePagination(async () => {
      throw new Error('offline')
    })

    await expect(list.reload()).rejects.toThrow('offline')

    expect(list.loading.value).toBe(false)
  })
})
