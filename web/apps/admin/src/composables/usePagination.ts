import { computed, type ComputedRef, type Ref, ref, shallowRef } from 'vue'

/** CursorPage is one page of a list the API pages with `?cursor=` (docs/architecture.md section 6). */
export interface CursorPage<T> {
  items: T[]
  /** nextCursor is absent on the last page. */
  nextCursor?: string
}

/** Pagination is a list read page by page, as "Carregar mais" shows it. */
export interface Pagination<T> {
  items: Ref<T[]>
  loading: Ref<boolean>
  hasMore: ComputedRef<boolean>
  /** reload reads the first page again, replacing what was loaded. */
  reload(): Promise<void>
  /** loadMore appends the next page, if there is one. */
  loadMore(): Promise<void>
}

/**
 * usePagination keeps a cursor-paged list: the items loaded so far, whether
 * there are more, and whether a page is on its way. Errors are thrown to the
 * caller, which decides what to say.
 */
export function usePagination<T>(fetchPage: (cursor?: string) => Promise<CursorPage<T>>): Pagination<T> {
  const items = shallowRef<T[]>([])
  const loading = ref(false)
  const cursor = ref<string | undefined>(undefined)
  const hasMore = computed(() => cursor.value !== undefined)

  async function load(from: string | undefined, keep: T[]): Promise<void> {
    loading.value = true
    try {
      const page = await fetchPage(from)
      items.value = [...keep, ...page.items]
      cursor.value = page.nextCursor
    } finally {
      loading.value = false
    }
  }

  return {
    items,
    loading,
    hasMore,
    reload: () => load(undefined, []),
    loadMore: () => (cursor.value === undefined ? Promise.resolve() : load(cursor.value, items.value)),
  }
}
