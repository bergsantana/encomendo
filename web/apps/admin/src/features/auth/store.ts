import { ApiError, logIn as requestSession } from '@encomendo/api'
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import { errorMessage, issuesByField } from '../../errorMessage'
import { useSessionStore } from '../../stores/session'

/** Credentials are what the login form collects. */
export interface Credentials {
  email: string
  password: string
}

const fixTheFields = 'Confira os campos destacados.'
const loginMessages = {
  invalid_credentials: 'E-mail ou senha incorretos.',
  invalid: fixTheFields,
}

/**
 * useAuthStore backs the login screen and the logout button: it checks the
 * form, starts and ends the session, and keeps the pt-BR errors to show.
 */
export const useAuthStore = defineStore('auth', () => {
  const session = useSessionStore()
  const submitting = ref(false)
  const attemptError = ref<string | null>(null)
  const fieldErrors = ref<Record<string, string>>({})
  const loggingOut = ref(false)
  const logOutError = ref<string | null>(null)

  /** formError is the message above the form: the last attempt's, or why the session could not be restored. */
  const formError = computed(() => {
    if (attemptError.value !== null) return attemptError.value
    return session.restoreError === null ? null : errorMessage(session.restoreError)
  })

  /** logIn resolves true once signed in; otherwise formError and fieldErrors say what to fix. */
  async function logIn(credentials: Credentials): Promise<boolean> {
    const email = credentials.email.trim()
    clearErrors()
    fieldErrors.value = missingFields(email, credentials.password)
    if (Object.keys(fieldErrors.value).length > 0) {
      attemptError.value = fixTheFields
      return false
    }

    submitting.value = true
    try {
      await session.begin((client) => requestSession(client, { email, password: credentials.password }))
      return true
    } catch (error) {
      if (!(error instanceof ApiError)) throw error
      fieldErrors.value = issuesByField(error)
      attemptError.value = errorMessage(error, loginMessages)
      return false
    } finally {
      submitting.value = false
    }
  }

  /** clearErrors forgets the last attempt's errors, e.g. when the form opens again. */
  function clearErrors(): void {
    attemptError.value = null
    fieldErrors.value = {}
  }

  /**
   * logOut ends the session on the server and here. When the server cannot
   * be reached the member stays signed in and logOutError says so.
   */
  async function logOut(): Promise<void> {
    loggingOut.value = true
    logOutError.value = null
    try {
      await session.logOut()
    } catch (error) {
      if (!(error instanceof ApiError)) throw error
      logOutError.value = `Não foi possível sair. ${errorMessage(error)}`
    } finally {
      loggingOut.value = false
    }
  }

  return { submitting, formError, fieldErrors, loggingOut, logOutError, logIn, clearErrors, logOut }
})

// The password is not trimmed: spaces are valid in it.
function missingFields(email: string, password: string): Record<string, string> {
  const missing: Record<string, string> = {}
  if (email === '') missing.email = 'Informe seu e-mail.'
  if (password === '') missing.password = 'Informe sua senha.'
  return missing
}
