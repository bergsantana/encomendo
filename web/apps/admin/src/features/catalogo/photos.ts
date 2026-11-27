import { deleteItemPhoto, deleteKitPhoto, orderItemPhotos, orderKitPhotos, type Photo, uploadPhoto } from '@encomendo/api'
import { ref, shallowRef } from 'vue'

import { api } from '../../api'
import { photoMessage } from './photoMessage'

/**
 * usePhotos keeps the photos of the item or kit that is open: upload, "Usar
 * como capa" and "Remover foto". They change on the server at once, apart
 * from "Salvar". `ownerId` says which item or kit is open, or null when none
 * is; the catalog store uses it once for items and once for kits.
 */
export function usePhotos(subject: 'item' | 'kit', ownerId: () => string | null) {
  const photos = shallowRef<Photo[]>([])
  /** error is why the last upload or change was refused, for the line above the photos. */
  const error = ref<string | null>(null)
  const busy = ref(false)
  let uploads: AbortController | null = null
  const order = subject === 'item' ? orderItemPhotos : orderKitPhotos
  const erase = subject === 'item' ? deleteItemPhoto : deleteKitPhoto

  /** set shows the photos of the owner that was just read. */
  function set(list: Photo[]): void {
    photos.value = list
  }

  /** clearError drops the message of the last refusal. */
  function clearError(): void {
    error.value = null
  }

  /** abort stops any photo still going up, when the form closes. */
  function abort(): void {
    uploads?.abort()
    uploads = null
  }

  /** add uploads the chosen files in turn, stopping at the first one refused. */
  async function add(files: File[]): Promise<void> {
    const id = ownerId()
    if (id === null) return
    error.value = null
    uploads = new AbortController()
    busy.value = true
    try {
      for (const file of files) {
        const photo = await uploadPhoto(api, { ownerType: subject, ownerId: id, file, signal: uploads.signal })
        photos.value = [...photos.value, photo]
      }
    } catch (failure) {
      error.value = photoMessage(failure, subject)
    } finally {
      busy.value = false
    }
  }

  /** makeCover puts a photo first, keeping the others in their order. */
  async function makeCover(photo: Photo): Promise<void> {
    const id = ownerId()
    if (id === null) return
    error.value = null
    try {
      const ids = [photo.id, ...photos.value.filter((other) => other.id !== photo.id).map((other) => other.id)]
      photos.value = (await order(api, id, ids)).items
    } catch (failure) {
      error.value = photoMessage(failure, subject)
    }
  }

  /** remove deletes one photo. */
  async function remove(photo: Photo): Promise<void> {
    const id = ownerId()
    if (id === null) return
    error.value = null
    try {
      await erase(api, id, photo.id)
      photos.value = photos.value.filter((other) => other.id !== photo.id)
    } catch (failure) {
      error.value = photoMessage(failure, subject)
    }
  }

  return { photos, error, busy, set, clearError, abort, add, makeCover, remove }
}
