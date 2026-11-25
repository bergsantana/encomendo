import { ApiError, createTenant, logIn as requestSession, signUp as requestAccount, type Tenant } from '@encomendo/api'
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import { api } from '../../api'
import { errorMessage, issuesByField } from '../../errorMessage'
import { lastStore } from '../../lastStore'
import { useSessionStore } from '../../stores/session'

/** Credentials are what the login form collects. */
export interface Credentials {
  email: string
  password: string
}

/** Account is what the sign-up form collects. */
export interface Account {
  name: string
  email: string
  password: string
}

/** NewStore is what the create-store form collects. */
export interface NewStore {
  name: string
  slug: string
}

const fixTheFields = 'Confira os campos destacados.'

// The text above each form for the problem codes it expects.
const loginMessages = { invalid_credentials: 'E-mail ou senha incorretos.', invalid: fixTheFields }
const fieldMessages = { invalid: fixTheFields, email_taken: fixTheFields, slug_taken: fixTheFields }
const switchMessages = { forbidden: 'Você não faz mais parte desta loja.' }

// Codes that are about one field, and what that field says.
const fieldByCode: Partial<Record<string, [field: string, message: string]>> = {
  email_taken: ['email', 'Já existe uma conta com este e-mail.'],
  slug_taken: ['slug', 'Este link já está em uso. Escolha outro.'],
}

/**
 * useAuthStore backs the screens that start, move and end a session: login,
 * sign-up, the store picker and logout. It checks each form, makes the call,
 * and keeps the pt-BR errors to show above the form and next to its fields.
 */
export const useAuthStore = defineStore('auth', () => {
  const session = useSessionStore()
  const submitting = ref(false)
  const attemptError = ref<string | null>(null)
  const fieldErrors = ref<Record<string, string>>({})
  const loggingOut = ref(false)
  const logOutError = ref<string | null>(null)
  let created: Tenant | null = null

  /** formError is the message above the form: the last attempt's, or why the session could not be restored. */
  const formError = computed(() => {
    if (attemptError.value !== null) return attemptError.value
    return session.restoreError === null ? null : errorMessage(session.restoreError)
  })

  /** logIn resolves true once signed in, opening the store this device used last. */
  function logIn(credentials: Credentials): Promise<boolean> {
    const email = credentials.email.trim()
    const { password } = credentials
    const missing = required([
      ['email', email, 'Informe seu e-mail.'],
      ['password', password, 'Informe sua senha.'],
    ])
    return attempt(missing, loginMessages, () =>
      session.begin((client) => requestSession(client, { email, password, tenantId: lastStore() })),
    )
  }

  /** signUp resolves true once the account exists and is signed in, still without a store. */
  function signUp(account: Account): Promise<boolean> {
    const name = account.name.trim()
    const email = account.email.trim()
    const { password } = account
    const missing = required([
      ['name', name, 'Informe seu nome.'],
      ['email', email, 'Informe seu e-mail.'],
      ['password', password, 'Informe uma senha.'],
    ])
    return attempt(missing, fieldMessages, () => session.begin((client) => requestAccount(client, { name, email, password })))
  }

  /** chooseStore resolves true once the session is in that store. */
  function chooseStore(tenantId: string): Promise<boolean> {
    return attempt({}, switchMessages, () => session.switchTo(tenantId))
  }

  /** createStore resolves true once the store exists, owned by the member, and the session is in it. */
  function createStore(store: NewStore): Promise<boolean> {
    const name = store.name.trim()
    const slug = store.slug.trim().toLowerCase()
    const missing = required([
      ['name', name, 'Informe o nome da loja.'],
      ['slug', slug, 'Informe o link da loja.'],
    ])
    return attempt(missing, fieldMessages, async () => {
      // When the store was created but opening it failed, the retry only
      // opens it: creating it again would say its own link is taken.
      if (created?.slug !== slug) created = await createTenant(api, { name, slug })
      await session.switchTo(created.id)
      created = null
    })
  }

  /**
   * attempt runs one form's call: it stops at missing fields, and turns a
   * refusal into the message above the form and the errors next to fields.
   */
  async function attempt(missing: Record<string, string>, messages: Partial<Record<string, string>>, call: () => Promise<unknown>): Promise<boolean> {
    clearErrors()
    fieldErrors.value = missing
    if (Object.keys(missing).length > 0) {
      attemptError.value = fixTheFields
      return false
    }
    submitting.value = true
    try {
      await call()
      return true
    } catch (error) {
      if (!(error instanceof ApiError)) throw error
      fieldErrors.value = { ...issuesByField(error), ...fieldError(error.code) }
      attemptError.value = errorMessage(error, messages)
      return false
    } finally {
      submitting.value = false
    }
  }

  /** clearErrors forgets the last attempt's errors, e.g. when a form opens again. */
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

  return { submitting, formError, fieldErrors, loggingOut, logOutError, logIn, signUp, chooseStore, createStore, clearErrors, logOut }
})

/** required lists the empty fields with what each should say. Passwords are not trimmed: spaces are valid in them. */
function required(fields: [field: string, value: string, message: string][]): Record<string, string> {
  return Object.fromEntries(fields.filter(([, value]) => value === '').map(([field, , message]) => [field, message]))
}

function fieldError(code: string): Record<string, string> {
  const known = fieldByCode[code]
  return known === undefined ? {} : { [known[0]]: known[1] }
}
