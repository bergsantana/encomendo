import { ApiError, PhotoFileError, PhotoUploadError } from '@encomendo/api'

import { errorMessage } from '../../errorMessage'

const notSent = 'A foto não foi enviada. Tente de novo.'

/** photoOwnerWord is what the photos belong to, in the words of the screens. */
export const photoOwnerWord = { item: 'item', kit: 'kit' } as const

/**
 * photoMessage is what a failed photo upload or change says, or null when the
 * person cancelled it. `subject` is what the photos belong to: an item or a
 * kit, each holding up to 8 (docs/product/design-document.md, Catálogo).
 */
export function photoMessage(failure: unknown, subject: 'item' | 'kit' = 'item'): string | null {
  if (failure instanceof PhotoFileError) return failure.code === 'photo_too_large' ? 'Esta foto passa de 10 MB.' : 'Use uma foto JPEG, PNG ou WebP.'
  if (failure instanceof PhotoUploadError) return notSent
  if (failure instanceof DOMException && failure.name === 'AbortError') return null
  if (!(failure instanceof ApiError)) throw failure
  return errorMessage(failure, {
    photo_limit: `Cada ${photoOwnerWord[subject]} pode ter até 8 fotos.`,
    photo_invalid: 'Use uma foto JPEG, PNG ou WebP de até 10 MB.',
    photo_not_uploaded: notSent,
  })
}
