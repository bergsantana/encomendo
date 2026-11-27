import {
  ApiError,
  archiveKit,
  createKit,
  getKit,
  type ItemSummary,
  type Kit,
  type KitSummary,
  listItems,
  listKits,
  type Reference,
  replaceKit,
  restoreKit,
  setKitPublished,
} from '@encomendo/api'
import { computed, ref, shallowRef } from 'vue'

import { api } from '../../api'
import { useForm } from '../../composables/useForm'
import { errorMessage } from '../../errorMessage'
import { type KitDraft, kitChecks, kitField, toKitInput } from './kitForm'
import { usePhotos } from './photos'
import type { useStoreSettings } from './storeSettings'
import type { useTaxonomy } from './taxonomy'

const pageSize = 50
const fixTheFields = 'Confira os campos destacados.'
const conflictMessage = 'Outra pessoa alterou este kit. Recarregue para ver a versão atual.'

/** archivedMessage names the archived items a kit write was refused for (`item_archived`), once each. */
function archivedMessage(items: Reference[]): string {
  const names = new Map(items.map((item) => [item.id, item.name]))
  if (names.size === 0) return 'Um item do kit está arquivado e não pode entrar no kit.'
  return [...names.values()].map((name) => `${name} está arquivado e não pode entrar no kit.`).join(' ')
}

/**
 * useKits is the Kits tab of Catálogo: the kit list, the kit editor, its
 * photos, and the store's items the editor chooses from. The catalog store
 * uses it and shares the categories, fields and settings with the Itens tab,
 * so there is one copy of each. Reading needs `catalog:read` and every write
 * `catalog:write`; the API checks both.
 *
 * Totals, discounts, margins and days are the API's (ADR 0025): this keeps
 * the kit as the API returned it and never works any of them out.
 */
export function useKits(deps: { taxonomy: ReturnType<typeof useTaxonomy>; settings: ReturnType<typeof useStoreSettings> }) {
  const kits = shallowRef<KitSummary[]>([])
  const nextCursor = ref<string | undefined>(undefined)
  const loading = ref(false)
  /** loaded is true once a list read succeeded, so the screen tells "loading" from "empty". */
  const loaded = ref(false)
  /** loadFailed is true after a failed open, when the tab offers "Tentar de novo". */
  const loadFailed = ref(false)
  const includeArchived = ref(false)
  let listRead = 0

  /** catalogItems are every item of the store, archived ones too, so a kit row can name and place its item. */
  const catalogItems = shallowRef<ItemSummary[]>([])

  const publishingId = ref<string | null>(null)
  /** notice is the last change that went through, for a status line. */
  const notice = ref<string | null>(null)
  /** error is why the last read or switch failed, for the alert above the table. */
  const error = ref<string | null>(null)

  /** editing is the kit the editor shows, or null for a new one; `editorOpen` says whether it is there at all. */
  const editing = shallowRef<Kit | null>(null)
  const editorOpen = ref(false)
  const editorLoading = ref(false)
  /** conflict is true after `version_conflict`, when the editor offers "Recarregar". */
  const conflict = ref(false)
  const form = useForm()
  const photos = usePhotos('kit', () => editing.value?.id ?? null)

  async function guard<T>(call: () => Promise<T>): Promise<T | undefined> {
    error.value = null
    try {
      return await call()
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      error.value = errorMessage(failure, { version_conflict: conflictMessage })
      return undefined
    }
  }

  async function readKits(cursor: string | undefined): Promise<void> {
    const read = ++listRead
    loading.value = true
    try {
      const page = await listKits(api, { cursor, limit: pageSize, includeArchived: includeArchived.value ? true : undefined })
      // A newer read started meanwhile: its answer is the one to show.
      if (read !== listRead) return
      kits.value = cursor === undefined ? page.items : [...kits.value, ...page.items]
      nextCursor.value = page.nextCursor
      loaded.value = true
    } finally {
      if (read === listRead) loading.value = false
    }
  }

  async function readCatalogItems(): Promise<void> {
    const all: ItemSummary[] = []
    let cursor: string | undefined
    do {
      const page = await listItems(api, { cursor, limit: 100, includeArchived: true })
      all.push(...page.items)
      cursor = page.nextCursor
    } while (cursor !== undefined)
    catalogItems.value = all
  }

  /**
   * open reads the list and everything the editor chooses from, all at once,
   * so one read failing does not keep the others from showing.
   */
  async function open(): Promise<void> {
    notice.value = null
    const read = await guard(() =>
      Promise.all([readKits(undefined), readCatalogItems(), deps.taxonomy.loadCategories(), deps.taxonomy.loadAttributes(), deps.settings.load()]),
    )
    loadFailed.value = read === undefined
  }

  /** search reads the first page again with the current filter. */
  async function search(): Promise<void> {
    await guard(() => readKits(undefined))
  }

  /** setIncludeArchived switches "Mostrar arquivados" and reads the list again. */
  async function setIncludeArchived(value: boolean): Promise<void> {
    includeArchived.value = value
    await search()
  }

  /** loadMore appends the next page ("Carregar mais"). */
  async function loadMore(): Promise<void> {
    if (nextCursor.value !== undefined) await guard(() => readKits(nextCursor.value))
  }

  /** setPublished switches "No portal" for a row, and shows what the server saved. */
  async function setPublished(kit: KitSummary, published: boolean): Promise<void> {
    notice.value = null
    publishingId.value = kit.id
    const saved = await guard(() => setKitPublished(api, kit.id, { published, version: kit.version })).finally(() => {
      publishingId.value = null
    })
    if (saved === undefined) {
      // A stale version or a lost permission: read the row again, and keep saying why.
      const why = error.value
      await search()
      error.value ??= why
      return
    }
    kits.value = kits.value.map((row) => (row.id === kit.id ? { ...row, published: saved.published, version: saved.version } : row))
  }

  function resetEditor(): void {
    form.clear()
    conflict.value = false
    photos.clearError()
  }

  function showKit(kit: Kit | null): void {
    editing.value = kit
    photos.set(kit?.photos ?? [])
    resetEditor()
    editorOpen.value = true
  }

  /** openNew opens the editor for a new kit. */
  function openNew(): void {
    notice.value = null
    showKit(null)
  }

  /** openEdit reads the kit and opens the editor on it. */
  async function openEdit(summary: KitSummary): Promise<void> {
    notice.value = null
    editorLoading.value = true
    try {
      const kit = await guard(() => getKit(api, summary.id))
      if (kit !== undefined) showKit(kit)
    } finally {
      editorLoading.value = false
    }
  }

  /** closeEditor closes it, dropping unsaved edits and any photo still going up. */
  function closeEditor(): void {
    photos.abort()
    editorOpen.value = false
    editing.value = null
    resetEditor()
  }

  /** reload reads the open kit again ("Recarregar"), dropping the edits that lost the race. */
  async function reload(): Promise<void> {
    const current = editing.value
    if (current === null) return
    try {
      showKit(await getKit(api, current.id))
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      form.error.value = errorMessage(failure)
    }
  }

  /** refusal turns a failed write on the open kit into the message above the form. */
  function refusal(failure: unknown): void {
    if (!(failure instanceof ApiError)) throw failure
    conflict.value = failure.code === 'version_conflict'
    form.error.value = errorMessage(failure, {
      version_conflict: conflictMessage,
      item_archived: archivedMessage(failure.references),
      invalid: fixTheFields,
    })
  }

  /**
   * save creates or replaces the kit from the form and resolves whether it
   * was saved. The editor stays open on the kit the API returned, so the
   * totals and days it shows are the new ones and photos can be added to a
   * kit that was just created.
   */
  async function save(draft: KitDraft): Promise<boolean> {
    conflict.value = false
    notice.value = null
    const checks = kitChecks(draft).map((check): [string, string, string] => [check.field, check.ok ? 'ok' : '', check.message])
    const current = editing.value
    const result: { saved: Kit | null; refused: string | null } = { saved: null, refused: null }
    const ok = await form.attempt(checks, { byCode: { version_conflict: conflictMessage, invalid: fixTheFields } }, async () => {
      const body = toKitInput(draft)
      try {
        result.saved = current === null ? await createKit(api, body) : await replaceKit(api, current.id, { ...body, version: current.version })
      } catch (failure) {
        conflict.value = failure instanceof ApiError && failure.code === 'version_conflict'
        if (failure instanceof ApiError && failure.code === 'item_archived') result.refused = archivedMessage(failure.references)
        throw failure
      }
    })
    form.fieldErrors.value = Object.fromEntries(Object.entries(form.fieldErrors.value).map(([field, message]) => [kitField(field), message]))
    if (result.refused !== null) form.error.value = result.refused
    if (!ok || result.saved === null) return false
    showKit(result.saved)
    notice.value = 'Kit salvo.'
    await search()
    return true
  }

  async function change(call: (kit: Kit) => Promise<Kit>, message: (kit: Kit) => string): Promise<void> {
    const current = editing.value
    if (current === null) return
    resetEditor()
    try {
      await call(current)
    } catch (failure) {
      refusal(failure)
      return
    }
    notice.value = message(current)
    closeEditor()
    await search()
  }

  /** archive archives the open kit; it leaves the store page with it. */
  function archive(): Promise<void> {
    return change(
      (kit) => archiveKit(api, kit.id, { version: kit.version }),
      (kit) => `${kit.name} foi arquivado.`,
    )
  }

  /** restore brings the open archived kit back; it returns unpublished, and an archived item in it refuses. */
  function restore(): Promise<void> {
    return change(
      (kit) => restoreKit(api, kit.id, { version: kit.version }),
      (kit) => `${kit.name} voltou ao catálogo.`,
    )
  }

  return {
    kits,
    hasMore: computed(() => nextCursor.value !== undefined),
    loading,
    loaded,
    loadFailed,
    includeArchived,
    catalogItems,
    publishingId,
    notice,
    error,
    editing,
    editorOpen,
    editorLoading,
    conflict,
    saving: form.submitting,
    formError: form.error,
    fieldErrors: form.fieldErrors,
    photos,
    open,
    search,
    setIncludeArchived,
    loadMore,
    setPublished,
    openNew,
    openEdit,
    closeEditor,
    reload,
    save,
    archive,
    restore,
  }
}
