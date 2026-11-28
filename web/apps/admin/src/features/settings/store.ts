import { ApiError, chooseStatusWorkflow, getSettings, listStatusWorkflows, type StatusWorkflow, type TenantSettings, updateSettings } from '@encomendo/api'
import { defineStore } from 'pinia'
import { ref, shallowRef } from 'vue'

import { api } from '../../api'
import { useForm } from '../../composables/useForm'
import { errorMessage } from '../../errorMessage'
import { type DefaultsDraft, defaultsChecks, toUpdateRequest } from './defaultsForm'

const conflictMessage = 'Outra pessoa alterou as configurações. Recarregue para ver a versão atual.'
const fixTheFields = 'Confira os campos destacados.'
const presetGoneMessage = 'Esta lista de status não está mais disponível. Recarregue para ver as listas atuais.'

/**
 * useSettingsStore backs Configurações: the status workflow presets and the
 * one in use, and the store defaults. Only the owner opens the screen
 * (`settings:write`); the API checks it again on every write. Both writes
 * send the settings `version` that was read, so someone saving first shows
 * as a conflict instead of being overwritten.
 */
export const useSettingsStore = defineStore('settings', () => {
  const settings = shallowRef<TenantSettings | null>(null)
  const workflows = shallowRef<StatusWorkflow[]>([])
  /** loadError is why the last read failed, when the screen offers "Tentar de novo". */
  const loadError = ref<string | null>(null)
  /** formKey changes when the defaults form must start over from the settings, after a read or a save. */
  const formKey = ref(0)

  /** choosingId is the preset whose "Usar esta lista" is being saved. */
  const choosingId = ref<string | null>(null)
  const workflowNotice = ref<string | null>(null)
  const workflowError = ref<string | null>(null)
  const workflowConflict = ref(false)

  const defaults = useForm()
  const defaultsNotice = ref<string | null>(null)
  const defaultsConflict = ref(false)

  /**
   * open reads the settings and the presets. It also forgets the messages of
   * an earlier visit, so a notice never greets the owner on returning.
   */
  async function open(): Promise<void> {
    loadError.value = null
    workflowNotice.value = null
    workflowError.value = null
    workflowConflict.value = false
    defaults.clear()
    defaultsNotice.value = null
    defaultsConflict.value = false
    try {
      const [read, presets] = await Promise.all([getSettings(api), listStatusWorkflows(api)])
      settings.value = read
      workflows.value = presets.items
      formKey.value += 1
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      loadError.value = errorMessage(failure)
    }
  }

  /** reload reads everything again ("Recarregar"), dropping the edits that lost the race. */
  async function reload(): Promise<void> {
    await open()
  }

  /**
   * chooseWorkflow switches the store's status list. New orders take it;
   * existing ones keep theirs. The settings answer replaces ours, so the next
   * save carries the new version.
   */
  async function chooseWorkflow(workflow: StatusWorkflow): Promise<void> {
    const current = settings.value
    if (current === null) return
    workflowNotice.value = null
    workflowError.value = null
    workflowConflict.value = false
    choosingId.value = workflow.id
    try {
      settings.value = await chooseStatusWorkflow(api, { workflowId: workflow.id, version: workflow.version, expectedVersion: current.version })
      workflowNotice.value = `Lista de status alterada. Os próximos pedidos usam ${workflow.name}.`
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      workflowConflict.value = failure.code === 'version_conflict' || failure.code === 'workflow_not_found'
      workflowError.value = errorMessage(failure, { version_conflict: conflictMessage, workflow_not_found: presetGoneMessage, invalid: fixTheFields })
    } finally {
      choosingId.value = null
    }
  }

  /** saveDefaults sends every field with the version read, and resolves whether it was saved. */
  async function saveDefaults(draft: DefaultsDraft): Promise<boolean> {
    const current = settings.value
    if (current === null) return false
    defaultsNotice.value = null
    defaultsConflict.value = false
    const saved = await defaults.attempt(defaultsChecks(draft), { byCode: { version_conflict: conflictMessage, invalid: fixTheFields } }, async () => {
      try {
        settings.value = await updateSettings(api, toUpdateRequest(draft, current.version))
      } catch (failure) {
        defaultsConflict.value = failure instanceof ApiError && failure.code === 'version_conflict'
        throw failure
      }
    })
    if (!saved) return false
    defaultsNotice.value = 'Configurações salvas.'
    formKey.value += 1
    return true
  }

  return {
    settings,
    workflows,
    loadError,
    formKey,
    choosingId,
    workflowNotice,
    workflowError,
    workflowConflict,
    saving: defaults.submitting,
    defaultsError: defaults.error,
    defaultsFieldErrors: defaults.fieldErrors,
    defaultsNotice,
    defaultsConflict,
    open,
    reload,
    chooseWorkflow,
    saveDefaults,
  }
})
