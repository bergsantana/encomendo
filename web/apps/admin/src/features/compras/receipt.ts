import type { ReceiptCheck, ReceiptEntry, ReceiptRequest, ToReceive, ToReceiveLine } from '@encomendo/api'

/** itemPhotoLimit is how many photos one received item can carry ("Cada item pode ter até 2 fotos."). */
export const itemPhotoLimit = 2
/** invoicePhotoLimit is how many photos of the nota fiscal one receipt can carry. */
export const invoicePhotoLimit = 4
/** totalPhotoLimit is the most photos one receipt holds, invoice photos included (the API refuses more with `invalid`). */
export const totalPhotoLimit = 20

/** ReceiptPhoto is one photo of the receipt being made: `key` is null while it goes up, then the key the receipt names. */
export interface ReceiptPhoto {
  id: number
  key: string | null
}

/** EntryMark is what the person said about one line: its check, the defect note and the item's photos. */
export interface EntryMark {
  check: ReceiptCheck
  note: string
  photos: ReceiptPhoto[]
}

/** invoiceTarget is the photo target of "Foto da nota fiscal"; an item's target is its entry key. */
export const invoiceTarget = 'invoice'

/** entryKey identifies a line of Receber itens: its `lineId`, or its `orderItemId` for an item bought at the counter. */
export function entryKey(line: ToReceiveLine): string {
  return line.lineId ?? line.orderItemId
}

/** receivableLines is every line of Receber itens in screen order: the pedidos de compra, then the items bought at the counter. */
export function receivableLines(answer: ToReceive | null): ToReceiveLine[] {
  if (answer === null) return []
  return [...answer.groups.flatMap((group) => group.lines), ...answer.toBuy]
}

/** BuiltReceipt is the body of "Salvar recebimento", or why it cannot be sent yet. */
export interface BuiltReceipt {
  /** body is null when nothing is checked or a defect has no note. */
  body: ReceiptRequest | null
  /** missingNote holds the entry keys of "Com defeito" lines whose "O que houve?" is empty. */
  missingNote: string[]
}

function entryOf(line: ToReceiveLine, mark: EntryMark): ReceiptEntry {
  const photoKeys = mark.photos.flatMap((photo) => (photo.key === null ? [] : [photo.key]))
  const note = mark.check === 'defective' ? mark.note.trim() : ''
  const rest = { check: mark.check, ...(note === '' ? {} : { note }), ...(photoKeys.length === 0 ? {} : { photoKeys }) }
  return line.lineId === undefined ? { orderItemId: line.orderItemId, ...rest } : { lineId: line.lineId, ...rest }
}

/**
 * buildReceipt turns the checks into the request. A defect needs its note
 * before any call ("Conte o que houve com {item}."); the note of "Tudo certo"
 * is dropped, as the API does. Only checked lines are sent.
 */
export function buildReceipt(lines: readonly ToReceiveLine[], marks: Readonly<Record<string, EntryMark>>, invoice: readonly ReceiptPhoto[]): BuiltReceipt {
  const entries: ReceiptEntry[] = []
  const missingNote: string[] = []
  for (const line of lines) {
    const mark = marks[entryKey(line)]
    if (mark === undefined) continue
    if (mark.check === 'defective' && mark.note.trim() === '') missingNote.push(entryKey(line))
    entries.push(entryOf(line, mark))
  }
  if (entries.length === 0 || missingNote.length > 0) return { body: null, missingNote }
  const invoicePhotoKeys = invoice.flatMap((photo) => (photo.key === null ? [] : [photo.key]))
  return { body: { entries, ...(invoicePhotoKeys.length === 0 ? {} : { invoicePhotoKeys }) }, missingNote }
}

/** photoCount is how many photos the receipt holds now, those still going up included. */
export function photoCount(marks: Readonly<Record<string, EntryMark>>, invoice: readonly ReceiptPhoto[]): number {
  return Object.values(marks).reduce((sum, mark) => sum + mark.photos.length, invoice.length)
}
