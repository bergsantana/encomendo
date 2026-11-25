import { ApiError } from '@encomendo/api'
import { type Ref, ref } from 'vue'

import { errorMessage, issuesByField } from '../errorMessage'

/**
 * FieldCheck is one field to check before calling the API: its name, its
 * value, the pt-BR message when it is wrong, and what makes it right
 * (filled in, by default).
 */
export type FieldCheck = [field: string, value: string, message: string, isValid?: (value: string) => boolean]

/** FormMessages is what a form says for the problem codes it expects. */
export interface FormMessages {
  /** Text above the form, by code. */
  byCode?: Partial<Record<string, string>>
  /** Codes that are about one field, and what that field says. */
  fieldByCode?: Partial<Record<string, [field: string, message: string]>>
}

/** FormState is one form's submission state and errors. */
export interface FormState {
  submitting: Ref<boolean>
  /** error is the pt-BR message above the form, if the last attempt failed. */
  error: Ref<string | null>
  /** fieldErrors holds the pt-BR message next to each field to fix. */
  fieldErrors: Ref<Record<string, string>>
  attempt(checks: FieldCheck[], messages: FormMessages, call: () => Promise<unknown>): Promise<boolean>
  clear(): void
}

const fixTheFields = 'Confira os campos destacados.'

/**
 * useForm runs a form's submit: it stops at fields to fix, calls the API,
 * and turns a refusal into the message above the form and the messages next
 * to the fields. Programming errors are not caught.
 */
export function useForm(): FormState {
  const submitting = ref(false)
  const error = ref<string | null>(null)
  const fieldErrors = ref<Record<string, string>>({})

  function clear(): void {
    error.value = null
    fieldErrors.value = {}
  }

  async function attempt(checks: FieldCheck[], messages: FormMessages, call: () => Promise<unknown>): Promise<boolean> {
    clear()
    fieldErrors.value = wrongFields(checks)
    if (Object.keys(fieldErrors.value).length > 0) {
      error.value = fixTheFields
      return false
    }
    submitting.value = true
    try {
      await call()
      return true
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      const field = messages.fieldByCode?.[failure.code]
      fieldErrors.value = { ...issuesByField(failure), ...(field === undefined ? {} : { [field[0]]: field[1] }) }
      error.value = errorMessage(failure, messages.byCode)
      return false
    } finally {
      submitting.value = false
    }
  }

  return { submitting, error, fieldErrors, attempt, clear }
}

function wrongFields(checks: FieldCheck[]): Record<string, string> {
  const wrong = checks.filter(([, value, , isValid = (v: string) => v !== '']) => !isValid(value))
  return Object.fromEntries(wrong.map(([field, , message]) => [field, message]))
}
