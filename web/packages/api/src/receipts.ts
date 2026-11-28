import { checkPhotoFile, defaultStorageTimeoutMs, putToStorage } from './catalog'
import type { HttpClient } from './http'
import { idempotencyHeaders } from './idempotency'
import type { components } from './types/schema'

type Schemas = components['schemas']

/** ToReceive is Receber itens: open lines by pedido de compra, then the items still in "A comprar". */
export type ToReceive = Schemas['ToReceive']
/** ToReceiveGroup is one pedido de compra of Receber itens ("C-0012 · {fornecedor}"). */
export type ToReceiveGroup = Schemas['ToReceiveGroup']
/** ToReceiveLine is one line to receive; it carries no cost. A counter item has no `lineId`. */
export type ToReceiveLine = Schemas['ToReceiveLine']
/** ReceiptRequest is "Salvar recebimento": the entries with their checks, and the invoice photos. */
export type ReceiptRequest = Schemas['ReceiptRequest']
/** ReceiptEntry is one checked line, or one item bought at the counter. */
export type ReceiptEntry = Schemas['ReceiptEntry']
/** Receipt is the saved receipt, with the orders it completed and the items back in "A comprar". */
export type Receipt = Schemas['Receipt']
/** ReceiptCheck is "Tudo certo" (`ok`) or "Com defeito" (`defective`). */
export type ReceiptCheck = Schemas['Check']

// Receiving needs `receiving:write` (owner, admin, staff).

const purchasing = '/api/v1/admin/purchasing'

/** toReceive calls GET /api/v1/admin/purchasing/to-receive: what can be received, by pedido de compra. */
export function toReceive(client: HttpClient): Promise<ToReceive> {
  return client.get<ToReceive>(`${purchasing}/to-receive`)
}

/** ReceiptPhotoInput is the file of a receiving photo, with the optional limits of its storage `PUT`. */
export interface ReceiptPhotoInput {
  file: Blob
  /** signal aborts the storage `PUT`. The abort is rethrown as is, not as PhotoUploadError. */
  signal?: AbortSignal
  /** storageTimeoutMs bounds the storage `PUT`. Defaults to 2 minutes. */
  storageTimeoutMs?: number
}

/**
 * uploadReceiptPhoto stores one receiving photo (an item's or the invoice's)
 * and resolves its key, which "Salvar recebimento" names. It requests an
 * upload, then sends the file to storage with exactly the signed headers, as
 * `uploadPhoto` does; there is no confirm step, the receipt is the
 * confirmation. A file that is not JPEG, PNG or WebP, or is over 10 MB, throws
 * PhotoFileError before any request; a failed storage `PUT` throws
 * PhotoUploadError; the API's refusals throw ApiError.
 */
export async function uploadReceiptPhoto(client: HttpClient, input: ReceiptPhotoInput): Promise<string> {
  const contentType = checkPhotoFile(input.file)
  const upload = await client.post<Schemas['PhotoUpload']>(`${purchasing}/receipt-photo-uploads`, { contentType })
  await putToStorage(upload, input.file, input.signal, input.storageTimeoutMs ?? defaultStorageTimeoutMs)
  return upload.key
}

/**
 * recordReceipt calls POST /api/v1/admin/purchasing/receipts ("Salvar
 * recebimento"). `idempotencyKey` is made once per tap of the button
 * (`newIdempotencyKey`) and sent again, unchanged, by "Tentar de novo": the
 * retry then answers the same receipt instead of receiving twice. Refusals
 * are `line_closed`, `order_closed`, `version_conflict`, `invalid` and
 * `idempotency_key_reused`.
 */
export function recordReceipt(client: HttpClient, body: ReceiptRequest, idempotencyKey: string): Promise<Receipt> {
  return client.post<Receipt>(`${purchasing}/receipts`, body, { headers: idempotencyHeaders(idempotencyKey) })
}
