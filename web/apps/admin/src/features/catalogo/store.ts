import {
  ApiError,
  archiveItem,
  createItem,
  createSupplier,
  deleteItemPhoto,
  getItem,
  getSettings,
  type Item,
  type ItemSummary,
  listItems,
  listSuppliers,
  orderItemPhotos,
  type Photo,
  PhotoFileError,
  PhotoUploadError,
  type Reference,
  replaceItem,
  restoreItem,
  setItemPublished,
  type Supplier,
  uploadPhoto,
} from '@encomendo/api'
import { defineStore } from 'pinia'
import { computed, reactive, ref, shallowRef } from 'vue'

import { api } from '../../api'
import { useForm } from '../../composables/useForm'
import { errorMessage } from '../../errorMessage'
import { useSessionStore } from '../../stores/session'
import { formField, type ItemDraft, itemChecks, toItemInput } from './itemForm'
import { isSupplierPhone, supplierPhone } from './phone'
import { useTaxonomy } from './taxonomy'

/** ItemFilters are the search box, the category filter and "Mostrar arquivados". */
export interface ItemFilters {
  q: string
  /** categoryId is '' for "Todas as categorias". */
  categoryId: string
  includeArchived: boolean
}

const pageSize = 50
const fixTheFields = 'Confira os campos destacados.'
const conflictMessage = 'Outra pessoa alterou este item. Recarregue para ver a versão atual.'

const photoMessages = {
  tooLarge: 'Esta foto passa de 10 MB.',
  wrongType: 'Use uma foto JPEG, PNG ou WebP.',
  invalid: 'Use uma foto JPEG, PNG ou WebP de até 10 MB.',
  limit: 'Cada item pode ter até 8 fotos.',
  notSent: 'A foto não foi enviada. Tente de novo.',
}

/** inKitMessage names the kits holding an item that was to be archived (`item_in_kit`). */
function inKitMessage(kits: Reference[]): string {
  const names = kits.map((kit) => kit.name)
  if (names.length === 0) return 'Este item está em um kit. Tire-o do kit antes de arquivar.'
  if (names.length === 1) return `Este item está no kit ${names[0]}. Tire-o do kit antes de arquivar.`
  const last = names[names.length - 1]
  return `Este item está nos kits ${names.slice(0, -1).join(', ')} e ${last}. Tire-o dos kits antes de arquivar.`
}

/** photoMessage is what a failed photo upload or change says, or null when the person cancelled it. */
function photoMessage(failure: unknown): string | null {
  if (failure instanceof PhotoFileError) return failure.code === 'photo_too_large' ? photoMessages.tooLarge : photoMessages.wrongType
  if (failure instanceof PhotoUploadError) return photoMessages.notSent
  if (failure instanceof DOMException && failure.name === 'AbortError') return null
  if (!(failure instanceof ApiError)) throw failure
  return errorMessage(failure, { photo_limit: photoMessages.limit, photo_invalid: photoMessages.invalid, photo_not_uploaded: photoMessages.notSent })
}

/**
 * useCatalogStore backs the Catálogo screen, Itens tab: the item list and its
 * filters, the item drawer, the supplier created inside it, the photos, and
 * the categories and fields dialogs. Reading needs `catalog:read` and every
 * write `catalog:write`; the API checks both. Costs and margins are the
 * owner's (ADR 0025): the store never sends a cost key for another role, and
 * shows what the API sent.
 */
export const useCatalogStore = defineStore('catalog', () => {
  const session = useSessionStore()
  const taxonomy = useTaxonomy()

  const items = shallowRef<ItemSummary[]>([])
  const nextCursor = ref<string | undefined>(undefined)
  const loading = ref(false)
  /** loaded is true once the first read ended, so the screen tells "loading" from "empty". */
  const loaded = ref(false)
  const filters = reactive<ItemFilters>({ q: '', categoryId: '', includeArchived: false })
  let listRead = 0

  const suppliers = shallowRef<Supplier[]>([])
  const stockEnabled = ref(false)
  const productionEnabled = ref(false)
  const currency = ref('BRL')

  /** notice is the last change that went through, for a status line. */
  const notice = ref<string | null>(null)
  /** error is why the last read or switch failed, for the alert above the table. */
  const error = ref<string | null>(null)

  /** editing is the item the drawer shows, or null for a new one; `drawerOpen` says whether it is there at all. */
  const editing = shallowRef<Item | null>(null)
  const drawerOpen = ref(false)
  const drawerLoading = ref(false)
  /** drawerKey changes when the drawer's item is read again, so the form starts over from it. */
  const drawerKey = ref(0)
  const photos = shallowRef<Photo[]>([])
  const photoError = ref<string | null>(null)
  const photoBusy = ref(false)
  /** conflict is true after `version_conflict`, when the drawer offers "Recarregar". */
  const conflict = ref(false)
  const itemForm = useForm()
  const supplierForm = useForm()
  let photoUploads: AbortController | null = null

  /** sendsCosts says whether the signed-in role may write costs: the owner only (ADR 0025). */
  const sendsCosts = computed(() => session.role === 'owner')

  /**
   * guard runs a read or switch and turns a refusal into the alert above the
   * table. Programming errors are not caught.
   */
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

  async function readItems(cursor: string | undefined): Promise<void> {
    const read = ++listRead
    loading.value = true
    try {
      const page = await listItems(api, {
        cursor,
        limit: pageSize,
        q: filters.q.trim() === '' ? undefined : filters.q.trim(),
        categoryId: filters.categoryId === '' ? undefined : filters.categoryId,
        includeArchived: filters.includeArchived ? true : undefined,
      })
      // A newer search started meanwhile: its answer is the one to show.
      if (read !== listRead) return
      items.value = cursor === undefined ? page.items : [...items.value, ...page.items]
      nextCursor.value = page.nextCursor
      loaded.value = true
    } finally {
      if (read === listRead) loading.value = false
    }
  }

  async function readSuppliers(): Promise<void> {
    const all: Supplier[] = []
    let cursor: string | undefined
    do {
      const page = await listSuppliers(api, { cursor, limit: 100 })
      all.push(...page.items)
      cursor = page.nextCursor
    } while (cursor !== undefined)
    suppliers.value = all
  }

  async function readSettings(): Promise<void> {
    const settings = await getSettings(api)
    stockEnabled.value = settings.stockEnabled
    productionEnabled.value = settings.productionEnabled
    currency.value = settings.currency
  }

  /** open reads the list and everything the drawer and dialogs choose from. */
  async function open(): Promise<void> {
    notice.value = null
    await guard(() => Promise.all([readItems(undefined), taxonomy.loadCategories(), taxonomy.loadAttributes(), readSuppliers(), readSettings()]))
    loaded.value = true
  }

  /** search reads the first page again with the current filters. */
  async function search(): Promise<void> {
    await guard(() => readItems(undefined))
  }

  /** loadMore appends the next page ("Carregar mais"). */
  async function loadMore(): Promise<void> {
    if (nextCursor.value !== undefined) await guard(() => readItems(nextCursor.value))
  }

  /** setPublished switches "No portal" for a row, and shows what the server saved. */
  async function setPublished(item: ItemSummary, published: boolean): Promise<void> {
    notice.value = null
    const saved = await guard(() => setItemPublished(api, item.id, { published, version: item.version }))
    if (saved === undefined) {
      // A stale version or a lost permission: read the row again, and keep saying why.
      const why = error.value
      await search()
      error.value ??= why
      return
    }
    items.value = items.value.map((row) => (row.id === item.id ? { ...row, published: saved.published, version: saved.version } : row))
  }

  function resetDrawer(): void {
    itemForm.clear()
    supplierForm.clear()
    conflict.value = false
    photoError.value = null
  }

  function showItem(item: Item | null): void {
    editing.value = item
    photos.value = item?.photos ?? []
    drawerKey.value += 1
    resetDrawer()
    drawerOpen.value = true
  }

  /** openNew opens the drawer for a new item. */
  function openNew(): void {
    notice.value = null
    showItem(null)
  }

  /** openEdit reads the item and opens the drawer on it. */
  async function openEdit(summary: ItemSummary): Promise<void> {
    notice.value = null
    drawerLoading.value = true
    try {
      const item = await guard(() => getItem(api, summary.id))
      if (item !== undefined) showItem(item)
    } finally {
      drawerLoading.value = false
    }
  }

  /** closeDrawer closes it, dropping unsaved edits and any photo still going up. */
  function closeDrawer(): void {
    photoUploads?.abort()
    photoUploads = null
    drawerOpen.value = false
    editing.value = null
    resetDrawer()
  }

  /** reloadItem reads the open item again ("Recarregar"), dropping the edits that lost the race. */
  async function reloadItem(): Promise<void> {
    const current = editing.value
    if (current === null) return
    try {
      showItem(await getItem(api, current.id))
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      itemForm.error.value = errorMessage(failure)
    }
  }

  /** refusal turns a failed write on the open item into the message above the form. */
  function refusal(failure: unknown): void {
    if (!(failure instanceof ApiError)) throw failure
    conflict.value = failure.code === 'version_conflict'
    itemForm.error.value = errorMessage(failure, {
      version_conflict: conflictMessage,
      item_in_kit: inKitMessage(failure.references),
      invalid: fixTheFields,
    })
  }

  /** saveItem creates or replaces the item from the form, and resolves whether it was saved. */
  async function saveItem(draft: ItemDraft): Promise<boolean> {
    conflict.value = false
    const sends = sendsCosts.value
    const checks = itemChecks(draft, sends).map((check): [string, string, string] => [check.field, check.ok ? 'ok' : '', check.message])
    const current = editing.value
    const saved = await itemForm.attempt(checks, { byCode: { version_conflict: conflictMessage, invalid: fixTheFields } }, async () => {
      const body = toItemInput(draft, sends)
      try {
        if (current === null) await createItem(api, body)
        else await replaceItem(api, current.id, { ...body, version: current.version })
      } catch (failure) {
        conflict.value = failure instanceof ApiError && failure.code === 'version_conflict'
        throw failure
      }
    })
    itemForm.fieldErrors.value = Object.fromEntries(Object.entries(itemForm.fieldErrors.value).map(([field, message]) => [formField(field), message]))
    if (!saved) return false
    notice.value = 'Item salvo.'
    closeDrawer()
    await search()
    return true
  }

  /** archive archives the open item; one that a kit holds is refused, naming the kit. */
  async function archive(): Promise<void> {
    const current = editing.value
    if (current === null) return
    resetDrawer()
    try {
      await archiveItem(api, current.id, { version: current.version })
    } catch (failure) {
      refusal(failure)
      return
    }
    notice.value = `${current.name} foi arquivado.`
    closeDrawer()
    await search()
  }

  /** restore brings the open archived item back; it returns unpublished. */
  async function restore(): Promise<void> {
    const current = editing.value
    if (current === null) return
    resetDrawer()
    try {
      await restoreItem(api, current.id, { version: current.version })
    } catch (failure) {
      refusal(failure)
      return
    }
    notice.value = `${current.name} voltou ao catálogo.`
    closeDrawer()
    await search()
  }

  /**
   * addSupplier creates a supplier from "Novo fornecedor" and resolves it, or
   * null when the fields need fixing. The number is typed with its DDD and
   * sent as the digits the API takes.
   */
  async function addSupplier(name: string, whatsapp: string): Promise<Supplier | null> {
    let created: Supplier | null = null
    await supplierForm.attempt(
      [
        ['name', name.trim(), 'Informe o nome do fornecedor.'],
        ['whatsappPhone', whatsapp.trim(), 'Informe o WhatsApp do fornecedor.'],
        ['whatsappPhone', isSupplierPhone(whatsapp) || whatsapp.trim() === '' ? 'ok' : '', 'Confira o número: use DDD e 8 ou 9 dígitos.'],
      ],
      { byCode: { invalid: fixTheFields } },
      async () => {
        created = await createSupplier(api, { name: name.trim(), whatsappPhone: supplierPhone(whatsapp) })
        suppliers.value = [...suppliers.value, created].sort((a, b) => a.name.localeCompare(b.name, 'pt-BR'))
      },
    )
    return created
  }

  /** addPhotos uploads the chosen files in turn to the open item, stopping at the first one refused. */
  async function addPhotos(files: File[]): Promise<void> {
    const current = editing.value
    if (current === null) return
    photoError.value = null
    photoUploads = new AbortController()
    photoBusy.value = true
    try {
      for (const file of files) {
        const photo = await uploadPhoto(api, { ownerType: 'item', ownerId: current.id, file, signal: photoUploads.signal })
        photos.value = [...photos.value, photo]
      }
    } catch (failure) {
      photoError.value = photoMessage(failure)
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
      photos.value = (await orderItemPhotos(api, current.id, ids)).items
    } catch (failure) {
      photoError.value = photoMessage(failure)
    }
  }

  /** removePhoto deletes a photo of the open item. */
  async function removePhoto(photo: Photo): Promise<void> {
    const current = editing.value
    if (current === null) return
    photoError.value = null
    try {
      await deleteItemPhoto(api, current.id, photo.id)
      photos.value = photos.value.filter((other) => other.id !== photo.id)
    } catch (failure) {
      photoError.value = photoMessage(failure)
    }
  }

  return {
    items,
    hasMore: computed(() => nextCursor.value !== undefined),
    loading,
    loaded,
    filters,
    suppliers,
    stockEnabled,
    productionEnabled,
    currency,
    sendsCosts,
    notice,
    error,
    editing,
    drawerOpen,
    drawerLoading,
    drawerKey,
    photos,
    photoError,
    photoBusy,
    conflict,
    saving: itemForm.submitting,
    formError: itemForm.error,
    fieldErrors: itemForm.fieldErrors,
    addingSupplier: supplierForm.submitting,
    supplierError: supplierForm.error,
    supplierFieldErrors: supplierForm.fieldErrors,
    clearSupplierForm: supplierForm.clear,
    categories: taxonomy.categories,
    attributes: taxonomy.attributes,
    categoryError: taxonomy.categoryError,
    categoryFormError: taxonomy.categoryForm.error,
    categoryFieldErrors: taxonomy.categoryForm.fieldErrors,
    categorySaving: taxonomy.categoryForm.submitting,
    clearCategoryForm: taxonomy.categoryForm.clear,
    attributeFormError: taxonomy.attributeForm.error,
    attributeFieldErrors: taxonomy.attributeForm.fieldErrors,
    attributeSaving: taxonomy.attributeForm.submitting,
    attributeFormId: taxonomy.attributeFormId,
    clearAttributeForm: taxonomy.attributeForm.clear,
    addCategory: taxonomy.addCategory,
    renameCategory: taxonomy.renameCategory,
    moveCategory: taxonomy.moveCategory,
    removeCategory: taxonomy.removeCategory,
    saveAttribute: taxonomy.saveAttribute,
    removeAttribute: taxonomy.removeAttribute,
    open,
    search,
    loadMore,
    setPublished,
    openNew,
    openEdit,
    closeDrawer,
    reloadItem,
    saveItem,
    archive,
    restore,
    addSupplier,
    addPhotos,
    makeCover,
    removePhoto,
  }
})
