import {
  ApiError,
  type Attribute,
  type AttributeInput,
  type Category,
  createAttribute,
  createCategory,
  deleteAttribute,
  deleteCategory,
  listAttributes,
  listCategories,
  replaceAttribute,
  updateCategory,
} from '@encomendo/api'
import { ref, shallowRef } from 'vue'

import { api } from '../../api'
import { useForm } from '../../composables/useForm'
import { errorMessage } from '../../errorMessage'

/** AttributeDraft is a "Campo" as its card holds it while it is edited. */
export interface AttributeDraft {
  name: string
  values: { id?: string; label: string }[]
}

const categoryMessages = {
  category_in_use: 'Esta categoria tem itens. Mude os itens de categoria antes de excluir.',
  not_found: 'Esta categoria não existe mais.',
}
const categoryNameMessages = { byCode: { invalid: 'Confira os campos destacados.' }, fieldByCode: { name_taken: ['name', 'Já existe uma categoria com este nome.'] as [string, string] } }
const attributeMessages = {
  byCode: { invalid: 'Confira os campos destacados.' },
  fieldByCode: { name_taken: ['name', 'Já existe um campo com este nome.'] as [string, string] },
}

/**
 * useTaxonomy keeps what items are sorted and described by: the store's
 * categories ("Categorias") and item fields ("Campos"), with the dialogs'
 * changes to them. The catalog store uses it; it has no state of its own to
 * share. Reads are open to every role, writes need `catalog:write`; the API
 * checks both.
 */
export function useTaxonomy() {
  const categories = shallowRef<Category[]>([])
  const attributes = shallowRef<Attribute[]>([])
  /** categoryError is why the last move or deletion of a category was refused. */
  const categoryError = ref<string | null>(null)
  const categoryForm = useForm()
  const attributeForm = useForm()
  /** attributeFormId is the card the last save came from: an attribute id, or '' for a new one. */
  const attributeFormId = ref<string | null>(null)

  async function loadCategories(): Promise<void> {
    categories.value = (await listCategories(api)).items
  }

  async function loadAttributes(): Promise<void> {
    attributes.value = (await listAttributes(api)).items
  }

  /** addCategory creates a category at the end of the list. */
  async function addCategory(name: string): Promise<boolean> {
    categoryError.value = null
    return categoryForm.attempt([['name', name.trim(), 'Informe o nome da categoria.']], categoryNameMessages, async () => {
      await createCategory(api, { name: name.trim() })
      await loadCategories()
    })
  }

  /** renameCategory gives a category another name. */
  async function renameCategory(category: Category, name: string): Promise<boolean> {
    categoryError.value = null
    return categoryForm.attempt([['name', name.trim(), 'Informe o nome da categoria.']], categoryNameMessages, async () => {
      await updateCategory(api, category.id, { name: name.trim() })
      await loadCategories()
    })
  }

  /** moveCategory puts a category at a place in the store's order; the others shift. */
  async function moveCategory(category: Category, position: number): Promise<void> {
    await refuse(async () => {
      await updateCategory(api, category.id, { position })
      await loadCategories()
    })
  }

  /** removeCategory deletes a category no item uses; one in use is refused with `category_in_use`. */
  async function removeCategory(category: Category): Promise<void> {
    await refuse(async () => {
      await deleteCategory(api, category.id)
      await loadCategories()
    })
  }

  async function refuse(call: () => Promise<void>): Promise<void> {
    categoryError.value = null
    categoryForm.clear()
    try {
      await call()
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      categoryError.value = errorMessage(failure, categoryMessages)
      if (failure.code === 'not_found') await loadCategories().catch(() => undefined)
    }
  }

  /**
   * saveAttribute creates a field (no `attribute`) or replaces one. A value
   * left out of an existing field leaves every item and kit that used it, so
   * the card asks first.
   */
  async function saveAttribute(attribute: Attribute | null, draft: AttributeDraft): Promise<boolean> {
    attributeFormId.value = attribute?.id ?? ''
    const checks: [string, string, string][] = [['name', draft.name.trim(), 'Informe o nome do campo.']]
    const seen = new Set<string>()
    draft.values.forEach((value, index) => {
      const label = value.label.trim()
      const duplicate = label !== '' && seen.has(label.toLowerCase())
      seen.add(label.toLowerCase())
      // A repeated value is reported as an empty one: the check fails with the repeat's own message.
      checks.push([`values.${index}`, duplicate ? '' : label, duplicate ? 'Este valor já existe neste campo.' : 'Informe o valor.'])
    })
    const body: AttributeInput = {
      name: draft.name.trim(),
      values: draft.values.map((value) => ({ ...(value.id === undefined ? {} : { id: value.id }), label: value.label.trim() })),
    }
    const saved = await attributeForm.attempt(checks, attributeMessages, async () => {
      if (attribute === null) await createAttribute(api, body)
      else await replaceAttribute(api, attribute.id, body)
      await loadAttributes()
    })
    // The server names a value as `values[1].label`; the card shows it under the value's field.
    attributeForm.fieldErrors.value = Object.fromEntries(
      Object.entries(attributeForm.fieldErrors.value).map(([field, message]) => [field.replace(/^values\.(\d+)\.label$/, 'values.$1'), message]),
    )
    return saved
  }

  /** removeAttribute deletes a field; its values leave every item and kit. */
  async function removeAttribute(attribute: Attribute): Promise<void> {
    attributeFormId.value = attribute.id
    attributeForm.clear()
    try {
      await deleteAttribute(api, attribute.id)
      await loadAttributes()
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      attributeForm.error.value = errorMessage(failure, { not_found: 'Este campo não existe mais.' })
      if (failure.code === 'not_found') await loadAttributes().catch(() => undefined)
    }
  }

  return {
    categories,
    attributes,
    categoryError,
    categoryForm,
    attributeForm,
    attributeFormId,
    loadCategories,
    loadAttributes,
    addCategory,
    renameCategory,
    moveCategory,
    removeCategory,
    saveAttribute,
    removeAttribute,
  }
}
