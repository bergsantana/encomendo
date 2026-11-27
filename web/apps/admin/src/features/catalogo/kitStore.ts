import {
  ApiError,
  archiveKit,
  type Attribute,
  type Category,
  createKit,
  deleteKitPhoto,
  getKit,
  getSettings,
  type ItemSummary,
  type Kit,
  type KitSummary,
  listAttributes,
  listCategories,
  listItems,
  listKits,
  orderKitPhotos,
  type Photo,
  type Reference,
  replaceKit,
  restoreKit,
  setKitPublished,
  uploadPhoto,
} from '@encomendo/api'
import { defineStore } from 'pinia'
import { computed, ref, shallowRef } from 'vue'

import { api } from '../../api'
import { useForm } from '../../composables/useForm'
import { errorMessage } from '../../errorMessage'
import { type KitDraft, kitChecks, kitField, toKitInput } from './kitForm'
import { photoMessage } from './photoMessage'

const pageSize = 50
const fixTheFields = 'Confira os campos destacados.'
const conflictMessage = 'Outra pessoa alterou este kit. Recarregue para ver a versão atual.'

/** archivedMessage names the archived items a kit write was refused for (`item_archived`). */
function archivedMessage(items: Reference[]): string {
  if (items.length === 0) return 'Um item do kit está arquivado e não pode entrar no kit.'
  return items.map((item) => `${item.name} está arquivado e não pode entrar no kit.`).join(' ')
}

/**
 * useKitsStore backs the Kits tab of Catálogo: the kit list, the kit editor,
 * its photos, and what the editor chooses from (the store's items, fields and
 * default days). It is the second half of the Catálogo feature, apart from
 * `useCatalogStore` only to keep each store readable. Reading needs
 * `catalog:read` and every write `catalog:write`; the API checks both.
 *
 * Totals, discounts, margins and days are the API's (ADR 0025): the store
 * keeps the kit as the API returned it and never works any of them out.
 */
export const useKitsStore = defineStore('catalogKits', () => {
  const kits = shallowRef<KitSummary[]>([])
  const nextCursor = ref<string | undefined>(undefined)
  const loading = ref(false)
  /** loaded is true once the first read ended, so the screen tells "loading" from "empty". */
  const loaded = ref(false)
  const includeArchived = ref(false)
  let listRead = 0

  /** catalogItems are every item of the store, archived ones too, so a kit row can name and place its item. */
  const catalogItems = shallowRef<ItemSummary[]>([])
  const categories = shallowRef<Category[]>([])
  const attributes = shallowRef<Attribute[]>([])
  /** defaultAssemblyDays and defaultSafetyMarginDays are the store's, shown as "Padrão da loja". */
  const defaultAssemblyDays = ref<number | null>(null)
  const defaultSafetyMarginDays = ref<number | null>(null)

  const publishingId = ref<string | null>(null)
  /** notice is the last change that went through, for a status line. */
  const notice = ref<string | null>(null)
  /** error is why the last read or switch failed, for the alert above the table. */
  const error = ref<string | null>(null)

  /** editing is the kit the editor shows, or null for a new one; `editorOpen` says whether it is there at all. */
  const editing = shallowRef<Kit | null>(null)
  const editorOpen = ref(false)
  const editorLoading = ref(false)
  const photos = shallowRef<Photo[]>([])
  const photoError = ref<string | null>(null)
  const photoBusy = ref(false)
  /** conflict is true after `version_conflict`, when the editor offers "Recarregar". */
  const conflict = ref(false)
  const form = useForm()
  let photoUploads: AbortController | null = null

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

  async function readSettings(): Promise<void> {
    const settings = await getSettings(api)
    defaultAssemblyDays.value = settings.assemblyDays
    defaultSafetyMarginDays.value = settings.safetyMarginDays
  }

  /** open reads the list and everything the editor chooses from. */
  async function open(): Promise<void> {
    notice.value = null
    await guard(async () => {
      categories.value = (await listCategories(api)).items
      attributes.value = (await listAttributes(api)).items
      await Promise.all([readKits(undefined), readCatalogItems(), readSettings()])
    })
    loaded.value = true
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
    photoError.value = null
  }

  function showKit(kit: Kit | null): void {
    editing.value = kit
    photos.value = kit?.photos ?? []
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
    photoUploads?.abort()
    photoUploads = null
    editorOpen.value = false
    editing.value = null
    resetEditor()
  }

  /** reloadKit reads the open kit again ("Recarregar"), dropping the edits that lost the race. */
  async function reloadKit(): Promise<void> {
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
   * saveKit creates or replaces the kit from the form and resolves whether it
   * was saved. The editor stays open on the kit the API returned, so the
   * totals and days it shows are the new ones and photos can be added to a
   * kit that was just created.
   */
  async function saveKit(draft: KitDraft): Promise<boolean> {
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

  /** addPhotos uploads the chosen files in turn to the open kit, stopping at the first one refused. */
  async function addPhotos(files: File[]): Promise<void> {
    const current = editing.value
    if (current === null) return
    photoError.value = null
    photoUploads = new AbortController()
    photoBusy.value = true
    try {
      for (const file of files) {
        const photo = await uploadPhoto(api, { ownerType: 'kit', ownerId: current.id, file, signal: photoUploads.signal })
        photos.value = [...photos.value, photo]
      }
    } catch (failure) {
      photoError.value = photoMessage(failure, 'kit')
    } finally {
      photoBusy.value = false
    }
  }

  /** makeCover puts a photo first, keeping the others in their order. */
  async function makeCover(photo: Photo): Promise<void> {
    const current = editing.value
    if (current === null) return
    photoError.value = null
    try {
      const ids = [photo.id, ...photos.value.filter((other) => other.id !== photo.id).map((other) => other.id)]
      photos.value = (await orderKitPhotos(api, current.id, ids)).items
    } catch (failure) {
      photoError.value = photoMessage(failure, 'kit')
    }
  }

  /** removePhoto deletes a photo of the open kit. */
  async function removePhoto(photo: Photo): Promise<void> {
    const current = editing.value
    if (current === null) return
    photoError.value = null
    try {
      await deleteKitPhoto(api, current.id, photo.id)
      photos.value = photos.value.filter((other) => other.id !== photo.id)
    } catch (failure) {
      photoError.value = photoMessage(failure, 'kit')
    }
  }

  return {
    kits,
    hasMore: computed(() => nextCursor.value !== undefined),
    loading,
    loaded,
    includeArchived,
    catalogItems,
    categories,
    attributes,
    defaultAssemblyDays,
    defaultSafetyMarginDays,
    publishingId,
    notice,
    error,
    editing,
    editorOpen,
    editorLoading,
    photos,
    photoError,
    photoBusy,
    conflict,
    saving: form.submitting,
    formError: form.error,
    fieldErrors: form.fieldErrors,
    open,
    search,
    setIncludeArchived,
    loadMore,
    setPublished,
    openNew,
    openEdit,
    closeEditor,
    reloadKit,
    saveKit,
    archive,
    restore,
    addPhotos,
    makeCover,
    removePhoto,
  }
})
