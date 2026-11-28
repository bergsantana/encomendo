import type { HttpClient } from './http'
import { type PageRequest, queryString } from './query'
import type { components } from './types/schema'

type Schemas = components['schemas']

/** Category groups catalog items ("Categorias"). */
export type Category = Schemas['Category']
/** CategoryList is every category, in the store's order. */
export type CategoryList = Schemas['CategoryList']
/** CategoryInput is the body of POST /api/v1/admin/catalog/categories. */
export type CategoryInput = Schemas['CategoryInput']
/** UpdateCategoryRequest renames a category, moves it, or both. */
export type UpdateCategoryRequest = Schemas['UpdateCategoryRequest']
/** Attribute is an item field ("Serve em") with its ordered values. */
export type Attribute = Schemas['Attribute']
/** AttributeList is every item field, in the store's order. */
export type AttributeList = Schemas['AttributeList']
/** AttributeInput is the body of creating or replacing an item field. */
export type AttributeInput = Schemas['AttributeInput']
/** Item is one catalog item. Owner-only `cost`, `productionCost` and `margin` are absent for other roles (ADR 0025). */
export type Item = Schemas['Item']
/** ItemSummary is one row of the item list. */
export type ItemSummary = Schemas['ItemSummary']
/** ItemPage is one page of items, by name. */
export type ItemPage = Schemas['ItemPage']
/**
 * ItemInput is the body of creating an item. Its cost fields (`productionCostCents`,
 * a supplier's `costCents`) are optional on purpose: leave them out to keep the
 * stored cost, send `null` to clear it. Only the owner may send them at all;
 * for any other role even `null` is `forbidden`.
 */
export type ItemInput = Schemas['ItemInput']
/** ReplaceItemRequest is ItemInput plus the `version` that was read. */
export type ReplaceItemRequest = Schemas['ReplaceItemRequest']
/** Kit is a ready kit ("Kit pronto") with its items, totals and days. Owner-only `cost` and `margin` follow ADR 0025. */
export type Kit = Schemas['Kit']
/** KitSummary is one row of the kit list. */
export type KitSummary = Schemas['KitSummary']
/** KitPage is one page of kits, by name. */
export type KitPage = Schemas['KitPage']
/** KitInput is the body of creating a kit. */
export type KitInput = Schemas['KitInput']
/** ReplaceKitRequest is KitInput plus the `version` that was read. */
export type ReplaceKitRequest = Schemas['ReplaceKitRequest']
/** SetPublishedRequest shows or hides an item or kit on the store page ("No portal"). */
export type SetPublishedRequest = Schemas['SetPublishedRequest']
/** VersionRequest is the body of archive and restore: the `version` that was read. */
export type VersionRequest = Schemas['VersionRequest']
/** Photo is one photo of an item or kit, with a short-lived URL. */
export type Photo = Schemas['Photo']
/** PhotoList is the photos of an item or kit in their order; the first is the cover. */
export type PhotoList = Schemas['PhotoList']
/** PhotoUploadRequest asks where to upload one photo. */
export type PhotoUploadRequest = Schemas['PhotoUploadRequest']
/** PhotoUpload says where and how to upload one photo. */
export type PhotoUpload = Schemas['PhotoUpload']
/** OrderPhotosRequest lists every photo of the owner exactly once, in the new order. */
export type OrderPhotosRequest = Schemas['OrderPhotosRequest']

/** ItemListRequest asks for one page of items. */
export interface ItemListRequest extends PageRequest {
  /** q keeps only items whose name contains it, ignoring case. */
  q?: string
  categoryId?: string
  /** includeArchived also lists archived items ("Mostrar arquivados"). */
  includeArchived?: boolean
}

/** KitListRequest asks for one page of kits. Kits have no category. */
export interface KitListRequest extends PageRequest {
  q?: string
  includeArchived?: boolean
}

const catalog = '/api/v1/admin/catalog'

/** id escapes one path segment. */
function id(value: string): string {
  return encodeURIComponent(value)
}

// Reading needs `catalog:read` (every role) and writing `catalog:write`
// (owner, admin). The API re-checks both on every call.

// Categories

/** listCategories calls GET /api/v1/admin/catalog/categories. */
export function listCategories(client: HttpClient): Promise<CategoryList> {
  return client.get<CategoryList>(`${catalog}/categories`)
}

/** createCategory calls POST /api/v1/admin/catalog/categories. A name already used throws `name_taken`. */
export function createCategory(client: HttpClient, body: CategoryInput): Promise<Category> {
  return client.post<Category>(`${catalog}/categories`, body)
}

/** updateCategory calls PATCH /api/v1/admin/catalog/categories/{categoryId}: send `name`, `position` or both. */
export function updateCategory(client: HttpClient, categoryId: string, body: UpdateCategoryRequest): Promise<Category> {
  return client.patch<Category>(`${catalog}/categories/${id(categoryId)}`, body)
}

/** deleteCategory calls DELETE /api/v1/admin/catalog/categories/{categoryId}. A category an item uses throws `category_in_use`. */
export async function deleteCategory(client: HttpClient, categoryId: string): Promise<void> {
  await client.delete<unknown>(`${catalog}/categories/${id(categoryId)}`)
}

// Attributes (item fields)

/** listAttributes calls GET /api/v1/admin/catalog/attributes. */
export function listAttributes(client: HttpClient): Promise<AttributeList> {
  return client.get<AttributeList>(`${catalog}/attributes`)
}

/** createAttribute calls POST /api/v1/admin/catalog/attributes. */
export function createAttribute(client: HttpClient, body: AttributeInput): Promise<Attribute> {
  return client.post<Attribute>(`${catalog}/attributes`, body)
}

/**
 * replaceAttribute calls PUT /api/v1/admin/catalog/attributes/{attributeId}.
 * A value sent with its `id` keeps it; one left out is removed from every
 * item and kit that used it, so ask first, using `usageCount`.
 */
export function replaceAttribute(client: HttpClient, attributeId: string, body: AttributeInput): Promise<Attribute> {
  return client.put<Attribute>(`${catalog}/attributes/${id(attributeId)}`, body)
}

/** deleteAttribute calls DELETE /api/v1/admin/catalog/attributes/{attributeId}; its values leave every item and kit. */
export async function deleteAttribute(client: HttpClient, attributeId: string): Promise<void> {
  await client.delete<unknown>(`${catalog}/attributes/${id(attributeId)}`)
}

// Items

/** listItems calls GET /api/v1/admin/catalog/items. */
export function listItems(client: HttpClient, request: ItemListRequest = {}): Promise<ItemPage> {
  const query = queryString({
    cursor: request.cursor,
    limit: request.limit,
    q: request.q,
    categoryId: request.categoryId,
    includeArchived: request.includeArchived,
  })
  return client.get<ItemPage>(`${catalog}/items${query}`)
}

/**
 * createItem calls POST /api/v1/admin/catalog/items. Leave every cost field
 * out unless the signed-in role is owner: any other role sending one gets
 * `forbidden`.
 */
export function createItem(client: HttpClient, body: ItemInput): Promise<Item> {
  return client.post<Item>(`${catalog}/items`, body)
}

/** getItem calls GET /api/v1/admin/catalog/items/{itemId}. */
export function getItem(client: HttpClient, itemId: string): Promise<Item> {
  return client.get<Item>(`${catalog}/items/${id(itemId)}`)
}

/**
 * replaceItem calls PUT /api/v1/admin/catalog/items/{itemId}. A cost field
 * left out keeps the stored cost, so an admin saves without ever sending
 * one; `null` clears it. Someone saving first throws `version_conflict`.
 */
export function replaceItem(client: HttpClient, itemId: string, body: ReplaceItemRequest): Promise<Item> {
  return client.put<Item>(`${catalog}/items/${id(itemId)}`, body)
}

/** setItemPublished calls PUT /api/v1/admin/catalog/items/{itemId}/published ("No portal"). */
export function setItemPublished(client: HttpClient, itemId: string, body: SetPublishedRequest): Promise<Item> {
  return client.put<Item>(`${catalog}/items/${id(itemId)}/published`, body)
}

/** archiveItem calls POST /api/v1/admin/catalog/items/{itemId}/archive. A kit holding the item throws `item_in_kit`. */
export function archiveItem(client: HttpClient, itemId: string, body: VersionRequest): Promise<Item> {
  return client.post<Item>(`${catalog}/items/${id(itemId)}/archive`, body)
}

/** restoreItem calls POST /api/v1/admin/catalog/items/{itemId}/restore; the item comes back unpublished. */
export function restoreItem(client: HttpClient, itemId: string, body: VersionRequest): Promise<Item> {
  return client.post<Item>(`${catalog}/items/${id(itemId)}/restore`, body)
}

/**
 * confirmItemPhoto calls POST /api/v1/admin/catalog/items/{itemId}/photos.
 * The API answers 201 for a new photo and 200 when the key was already
 * confirmed; both resolve the photo. Prefer uploadPhoto.
 */
export function confirmItemPhoto(client: HttpClient, itemId: string, key: string): Promise<Photo> {
  return client.post<Photo>(`${catalog}/items/${id(itemId)}/photos`, { key })
}

/** orderItemPhotos calls PUT /api/v1/admin/catalog/items/{itemId}/photos/order; the first photo is the cover. */
export function orderItemPhotos(client: HttpClient, itemId: string, photoIds: string[]): Promise<PhotoList> {
  return client.put<PhotoList>(`${catalog}/items/${id(itemId)}/photos/order`, { photoIds })
}

/** deleteItemPhoto calls DELETE /api/v1/admin/catalog/items/{itemId}/photos/{photoId}. */
export async function deleteItemPhoto(client: HttpClient, itemId: string, photoId: string): Promise<void> {
  await client.delete<unknown>(`${catalog}/items/${id(itemId)}/photos/${id(photoId)}`)
}

// Kits

/** listKits calls GET /api/v1/admin/catalog/kits. */
export function listKits(client: HttpClient, request: KitListRequest = {}): Promise<KitPage> {
  const query = queryString({
    cursor: request.cursor,
    limit: request.limit,
    q: request.q,
    includeArchived: request.includeArchived,
  })
  return client.get<KitPage>(`${catalog}/kits${query}`)
}

/** createKit calls POST /api/v1/admin/catalog/kits. An archived item in it throws `item_archived`. */
export function createKit(client: HttpClient, body: KitInput): Promise<Kit> {
  return client.post<Kit>(`${catalog}/kits`, body)
}

/** getKit calls GET /api/v1/admin/catalog/kits/{kitId}. */
export function getKit(client: HttpClient, kitId: string): Promise<Kit> {
  return client.get<Kit>(`${catalog}/kits/${id(kitId)}`)
}

/** replaceKit calls PUT /api/v1/admin/catalog/kits/{kitId}. Someone saving first throws `version_conflict`. */
export function replaceKit(client: HttpClient, kitId: string, body: ReplaceKitRequest): Promise<Kit> {
  return client.put<Kit>(`${catalog}/kits/${id(kitId)}`, body)
}

/** setKitPublished calls PUT /api/v1/admin/catalog/kits/{kitId}/published ("No portal"). */
export function setKitPublished(client: HttpClient, kitId: string, body: SetPublishedRequest): Promise<Kit> {
  return client.put<Kit>(`${catalog}/kits/${id(kitId)}/published`, body)
}

/** archiveKit calls POST /api/v1/admin/catalog/kits/{kitId}/archive. */
export function archiveKit(client: HttpClient, kitId: string, body: VersionRequest): Promise<Kit> {
  return client.post<Kit>(`${catalog}/kits/${id(kitId)}/archive`, body)
}

/** restoreKit calls POST /api/v1/admin/catalog/kits/{kitId}/restore; the kit comes back unpublished. */
export function restoreKit(client: HttpClient, kitId: string, body: VersionRequest): Promise<Kit> {
  return client.post<Kit>(`${catalog}/kits/${id(kitId)}/restore`, body)
}

/** confirmKitPhoto calls POST /api/v1/admin/catalog/kits/{kitId}/photos; 200 and 201 both resolve the photo. Prefer uploadPhoto. */
export function confirmKitPhoto(client: HttpClient, kitId: string, key: string): Promise<Photo> {
  return client.post<Photo>(`${catalog}/kits/${id(kitId)}/photos`, { key })
}

/** orderKitPhotos calls PUT /api/v1/admin/catalog/kits/{kitId}/photos/order; the first photo is the cover. */
export function orderKitPhotos(client: HttpClient, kitId: string, photoIds: string[]): Promise<PhotoList> {
  return client.put<PhotoList>(`${catalog}/kits/${id(kitId)}/photos/order`, { photoIds })
}

/** deleteKitPhoto calls DELETE /api/v1/admin/catalog/kits/{kitId}/photos/{photoId}. */
export async function deleteKitPhoto(client: HttpClient, kitId: string, photoId: string): Promise<void> {
  await client.delete<unknown>(`${catalog}/kits/${id(kitId)}/photos/${id(photoId)}`)
}

// Photo uploads (ADR 0024)

/**
 * requestPhotoUpload calls POST /api/v1/admin/catalog/photo-uploads. The
 * server picks the key; a store at its photo limit throws `photo_limit`.
 */
export function requestPhotoUpload(client: HttpClient, body: PhotoUploadRequest): Promise<PhotoUpload> {
  return client.post<PhotoUpload>(`${catalog}/photo-uploads`, body)
}

/**
 * PhotoUploadError means the file never reached storage: the storage host
 * was unreachable, refused the `PUT` or the signed URL had expired. Nothing
 * was confirmed, so the caller may simply try again. The UI shows its own
 * pt-BR wording; this class carries none.
 */
export class PhotoUploadError extends Error {
  /** code is stable, like ApiError.code, so callers branch on it. */
  readonly code = 'photo_upload_failed'
  /** status is the storage host's HTTP status, or 0 when it could not be reached. */
  readonly status: number

  constructor(status: number) {
    super(`photo upload failed (${status})`)
    this.name = 'PhotoUploadError'
    this.status = status
  }
}

/** PhotoFileCode says why a file was refused before any request was made. */
export type PhotoFileCode = 'photo_type_unsupported' | 'photo_too_large'

/**
 * PhotoFileError means the file itself cannot be uploaded: it is not JPEG,
 * PNG or WebP (`photo_type_unsupported`) or it is over 10 MB
 * (`photo_too_large`). uploadPhoto throws it before making any request, so
 * nothing was requested, stored or confirmed. The UI shows its own pt-BR
 * wording; this class carries none.
 */
export class PhotoFileError extends Error {
  /** code is stable, like ApiError.code, so callers branch on it. */
  readonly code: PhotoFileCode

  constructor(code: PhotoFileCode) {
    super(`photo file refused (${code})`)
    this.name = 'PhotoFileError'
    this.code = code
  }
}

/** PhotoUploadInput names the photo's owner and the file to upload. */
export interface PhotoUploadInput {
  ownerType: PhotoUploadRequest['ownerType']
  ownerId: string
  file: Blob
  /** signal aborts the storage `PUT`, e.g. when the dialog closes. The abort is rethrown as is, not as PhotoUploadError. */
  signal?: AbortSignal
  /** storageTimeoutMs bounds the storage `PUT`. Defaults to 2 minutes. */
  storageTimeoutMs?: number
}

const photoTypes: readonly string[] = ['image/jpeg', 'image/png', 'image/webp']

// The server is the gate at the margin (ADR 0024); this only spares a
// pointless upload of a file that is clearly too big.
const maxPhotoBytes = 10 * 1024 * 1024

/** defaultStorageTimeoutMs is how long the storage `PUT` may take when the caller names no limit. */
export const defaultStorageTimeoutMs = 120_000

function isPhotoType(type: string): type is PhotoUploadRequest['contentType'] {
  return photoTypes.includes(type)
}

/**
 * checkPhotoFile returns the file's content type when it is a JPEG, PNG or
 * WebP of at most 10 MB, and throws PhotoFileError before any request is made
 * otherwise. Receiving photos share it (receipts.ts).
 */
export function checkPhotoFile(file: Blob): PhotoUploadRequest['contentType'] {
  const contentType = file.type
  if (!isPhotoType(contentType)) throw new PhotoFileError('photo_type_unsupported')
  if (file.size > maxPhotoBytes) throw new PhotoFileError('photo_too_large')
  return contentType
}

/**
 * uploadPhoto stores one photo of an item or kit: it requests an upload,
 * sends the file to storage and confirms it, and resolves the recorded
 * Photo. A file that is not JPEG, PNG or WebP, or is over 10 MB, throws
 * PhotoFileError before any request is made. The API's own refusals throw
 * ApiError with their code (`photo_invalid`, `photo_not_uploaded`,
 * `photo_limit`); a failed or timed-out storage `PUT` throws
 * PhotoUploadError and confirms nothing; a caller's abort is rethrown as is.
 */
export async function uploadPhoto(client: HttpClient, input: PhotoUploadInput): Promise<Photo> {
  const contentType = checkPhotoFile(input.file)

  const upload = await requestPhotoUpload(client, { ownerType: input.ownerType, ownerId: input.ownerId, contentType })
  await putToStorage(upload, input.file, input.signal, input.storageTimeoutMs ?? defaultStorageTimeoutMs)

  return input.ownerType === 'item'
    ? confirmItemPhoto(client, input.ownerId, upload.key)
    : confirmKitPhoto(client, input.ownerId, upload.key)
}

/** storageSignal combines the caller's signal with the storage timeout where AbortSignal.any exists. */
function storageSignal(callerSignal: AbortSignal | undefined, timeoutMs: number): AbortSignal {
  const timeout = AbortSignal.timeout(timeoutMs)
  if (callerSignal === undefined) return timeout
  return typeof AbortSignal.any === 'function' ? AbortSignal.any([callerSignal, timeout]) : callerSignal
}

/**
 * putToStorage sends the file to the signed storage URL.
 *
 * This is the one place in packages/api that calls fetch for a URL that is
 * not the API, so it deliberately bypasses http.ts: that client would add the
 * bearer token and the cookie credentials, handing the access token to the
 * storage host, and the signed URL rejects an extra Authorization header.
 * The request carries exactly the headers the API signed and no credentials
 * (ADR 0024). A caller's abort is rethrown untouched; a timeout or network
 * failure becomes PhotoUploadError.
 */
export async function putToStorage(upload: PhotoUpload, file: Blob, callerSignal: AbortSignal | undefined, timeoutMs: number): Promise<void> {
  let response: Response
  try {
    // Looked up on every call so a test mock or polyfill installed later applies.
    response = await globalThis.fetch(upload.uploadUrl, {
      method: upload.method,
      headers: upload.headers,
      body: file,
      credentials: 'omit',
      signal: storageSignal(callerSignal, timeoutMs),
    })
  } catch (error) {
    if (callerSignal?.aborted) throw callerSignal.reason ?? error
    throw new PhotoUploadError(0)
  }
  if (!response.ok) throw new PhotoUploadError(response.status)
}
