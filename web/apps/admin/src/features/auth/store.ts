import {
  acceptInvitation as joinStore,
  ApiError,
  confirmPasswordReset,
  createTenant,
  type InvitationPreview,
  logIn as requestSession,
  previewInvitation,
  requestPasswordReset,
  signUp as requestAccount,
  type Tenant,
} from '@encomendo/api'
import { defineStore } from 'pinia'
import { computed, ref, shallowRef } from 'vue'

import { api } from '../../api'
import { authClient } from '../../authClient'
import { type FieldCheck, type FormMessages, useForm } from '../../composables/useForm'
import { errorMessage, storeSuspendedMessage } from '../../errorMessage'
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

/** NewPassword is what the second step of "Esqueci a senha" collects. */
export interface NewPassword {
  code: string
  password: string
}

/** Joining is what accepting an invitation collects; the name only for a new account. */
export interface Joining {
  name: string
  password: string
}

/**
 * InvitationState is where the invitation screen stands. `missing` is a link
 * without its token, as after logging in again from that page (the token
 * never travels in a redirect); `unavailable` is a token that no longer works.
 */
export type InvitationState = 'loading' | 'ready' | 'missing' | 'unavailable' | 'failed'

const fixTheFields = 'Confira os campos destacados.'
/** invitationGone explains an invitation that is unknown, expired, revoked or used: the API does not say which. */
export const invitationGone = 'Este convite não vale mais. Peça um novo a quem convidou você.'

const loginMessages: FormMessages = { byCode: { invalid_credentials: 'E-mail ou senha incorretos.', invalid: fixTheFields } }
const resetRequestMessages: FormMessages = { byCode: { invalid: fixTheFields } }
const signUpMessages: FormMessages = {
  byCode: { invalid: fixTheFields, email_taken: fixTheFields },
  fieldByCode: { email_taken: ['email', 'Já existe uma conta com este e-mail.'] },
}
const storeMessages: FormMessages = {
  byCode: { invalid: fixTheFields, slug_taken: fixTheFields },
  fieldByCode: { slug_taken: ['slug', 'Este link já está em uso. Escolha outro.'] },
}
const switchMessages: FormMessages = { byCode: { forbidden: 'Você não faz mais parte desta loja.' } }
const resetMessages: FormMessages = {
  byCode: { invalid: fixTheFields, invalid_code: fixTheFields },
  fieldByCode: { invalid_code: ['code', 'Código errado ou vencido. Confira o e-mail ou peça um novo código.'] },
}
const joinMessages: FormMessages = {
  byCode: { invalid: fixTheFields, invalid_credentials: fixTheFields, already_member: 'Você já faz parte desta loja.', not_found: invitationGone },
  fieldByCode: { invalid_credentials: ['password', 'Senha incorreta.'] },
}

const sixDigits = (value: string) => /^\d{6}$/.test(value)

/**
 * useAuthStore backs the screens that start, move and end a session: login,
 * sign-up, the store picker, "Esqueci a senha", accepting an invitation and
 * logout. It checks each form, makes the call, and keeps the pt-BR messages.
 */
export const useAuthStore = defineStore('auth', () => {
  const session = useSessionStore()
  const form = useForm()
  const loggingOut = ref(false)
  const logOutError = ref<string | null>(null)
  /** notice is good news for the login screen, such as a password just changed. */
  const notice = ref<string | null>(null)
  /** resetEmail is where the reset code went, while the member types it in. */
  const resetEmail = ref<string | null>(null)
  const resetNotice = ref<string | null>(null)
  const invitation = shallowRef<InvitationPreview | null>(null)
  const invitationState = ref<InvitationState>('loading')
  let created: Tenant | null = null

  /** formError is the message above the form: the last attempt's, or why the session could not be restored. */
  const formError = computed(() => {
    if (form.error.value !== null) return form.error.value
    return session.restoreError === null ? null : errorMessage(session.restoreError)
  })

  /**
   * restoreFailed says the server could not answer on load, so the session in
   * the cookie may still be good and is worth asking for again. Not after a
   * 429: its message asks to wait, and every retry would count against it.
   */
  const restoreFailed = computed(
    () => form.error.value === null && session.restoreError !== null && session.restoreError.code !== 'rate_limited',
  )
  const retrying = ref(false)

  /**
   * retryRestore asks the server again for the session in the refresh
   * cookie, and resolves whether the tab is now signed in. Logging in again
   * instead would start a new login and leave the one in the cookie unused.
   * A click while it waits does nothing.
   */
  async function retryRestore(): Promise<boolean> {
    if (retrying.value) return false
    retrying.value = true
    try {
      await session.restore()
      return session.isSignedIn
    } finally {
      retrying.value = false
    }
  }

  /** logIn resolves true once signed in, opening the store this device used last. */
  function logIn(credentials: Credentials): Promise<boolean> {
    const email = credentials.email.trim()
    const { password } = credentials
    notice.value = null
    const checks: FieldCheck[] = [
      ['email', email, 'Informe seu e-mail.'],
      ['password', password, 'Informe sua senha.'],
    ]
    return form.attempt(checks, loginMessages, () =>
      session.begin((client) => requestSession(client, { email, password, tenantId: lastStore() })),
    )
  }

  /** signUp resolves true once the account exists and is signed in, still without a store. */
  function signUp(account: Account): Promise<boolean> {
    const name = account.name.trim()
    const email = account.email.trim()
    const { password } = account
    const checks: FieldCheck[] = [
      ['name', name, 'Informe seu nome.'],
      ['email', email, 'Informe seu e-mail.'],
      ['password', password, 'Informe uma senha.'],
    ]
    return form.attempt(checks, signUpMessages, () => session.begin((client) => requestAccount(client, { name, email, password })))
  }

  /**
   * chooseStore resolves true once the session has moved to that store. A
   * suspended store is never entered: it says so without calling the API
   * (ADR 0022). A switch the API still refuses as suspended says the same.
   */
  function chooseStore(tenantId: string): Promise<boolean> {
    if (isSuspended(tenantId)) {
      showSuspended()
      return Promise.resolve(false)
    }
    return form.attempt([], switchMessages, () => session.switchTo(tenantId))
  }

  function isSuspended(tenantId: string): boolean {
    return session.memberships.some((membership) => membership.tenant.id === tenantId && membership.status === 'suspended')
  }

  /** showSuspended puts the suspended-store message above the store picker, where the member lands from a store that was suspended. */
  function showSuspended(): void {
    form.clear()
    form.error.value = storeSuspendedMessage
  }

  /** createStore resolves true once the store exists, owned by the member, and the session has moved to it. */
  function createStore(store: NewStore): Promise<boolean> {
    const name = store.name.trim()
    const slug = store.slug.trim().toLowerCase()
    const checks: FieldCheck[] = [
      ['name', name, 'Informe o nome da loja.'],
      ['slug', slug, 'Informe o link da loja.'],
    ]
    return form.attempt(checks, storeMessages, async () => {
      // When the store was created but opening it failed, the retry only
      // opens it: creating it again would say its own link is taken.
      if (created?.slug !== slug) created = await createTenant(api, { name, slug })
      await session.switchTo(created.id)
      created = null
    })
  }

  /**
   * requestReset asks for a code by email. The screen says the same whether
   * or not the account exists, as the API does (ADR 0011).
   */
  async function requestReset(email: string): Promise<boolean> {
    const address = email.trim()
    const sent = await form.attempt([['email', address, 'Informe seu e-mail.']], resetRequestMessages, () => requestPasswordReset(authClient, address))
    if (sent) {
      resetEmail.value = address
      resetNotice.value = `Se houver uma conta com ${address}, enviamos um código de 6 dígitos. Ele vale por 10 minutos.`
    }
    return sent
  }

  /** resendReset asks for a new code for the same email; the earlier one stops working. */
  async function resendReset(): Promise<void> {
    const address = resetEmail.value
    if (address === null) return
    if (await form.attempt([], resetRequestMessages, () => requestPasswordReset(authClient, address))) {
      resetNotice.value = `Se houver uma conta com ${address}, enviamos um novo código. O anterior deixou de valer.`
    }
  }

  /** restartReset goes back to asking for the email. */
  function restartReset(): void {
    resetEmail.value = null
    resetNotice.value = null
    form.clear()
  }

  /** confirmReset sets the new password; the login screen then says so. Every session of the account ends. */
  async function confirmReset(input: NewPassword): Promise<boolean> {
    const email = resetEmail.value
    if (email === null) return false
    // A code copied from the email may come with spaces, such as "482 913".
    const code = input.code.replace(/\s/g, '')
    const { password } = input
    const checks: FieldCheck[] = [
      ['code', code, 'Informe os 6 dígitos do código.', sixDigits],
      ['password', password, 'Informe a nova senha.'],
    ]
    const changed = await form.attempt(checks, resetMessages, () => confirmPasswordReset(authClient, { email, code, password }))
    if (changed) {
      restartReset()
      notice.value = 'Senha alterada. Entre com a nova senha.'
      session.announceAllEnded()
    }
    return changed
  }

  /** clearNotice forgets the login screen's good news once the member moves on. */
  function clearNotice(): void {
    notice.value = null
  }

  /** openInvitation reads what an invitation link offers. */
  async function openInvitation(token: string): Promise<void> {
    invitation.value = null
    form.clear()
    if (token === '') {
      invitationState.value = 'missing'
      return
    }
    invitationState.value = 'loading'
    try {
      invitation.value = await previewInvitation(authClient, token)
      invitationState.value = 'ready'
    } catch (error) {
      if (!(error instanceof ApiError)) throw error
      invitationState.value = error.code === 'not_found' ? 'unavailable' : 'failed'
      if (invitationState.value === 'failed') form.error.value = errorMessage(error)
    }
  }

  /** acceptInvitation joins the store with a new account, or with the existing account's password, and signs in there. */
  function acceptInvitation(token: string, input: Joining): Promise<boolean> {
    const exists = invitation.value?.accountExists ?? false
    const name = input.name.trim()
    const { password } = input
    const checks: FieldCheck[] = exists
      ? [['password', password, 'Informe sua senha.']]
      : [
          ['name', name, 'Informe seu nome.'],
          ['password', password, 'Informe uma senha.'],
        ]
    const body = exists ? { token, password } : { token, name, password }
    return form.attempt(checks, joinMessages, async () => {
      try {
        await session.begin((client) => joinStore(client, body))
      } catch (error) {
        // It ran out, or was used or revoked, since the preview: stop offering the form.
        if (error instanceof ApiError && error.code === 'not_found') invitationState.value = 'unavailable'
        throw error
      }
    })
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

  return {
    submitting: form.submitting,
    fieldErrors: form.fieldErrors,
    formError,
    restoreFailed,
    retrying,
    retryRestore,
    notice,
    resetEmail,
    resetNotice,
    invitation,
    invitationState,
    loggingOut,
    logOutError,
    logIn,
    signUp,
    chooseStore,
    showSuspended,
    createStore,
    requestReset,
    resendReset,
    restartReset,
    confirmReset,
    clearNotice,
    openInvitation,
    acceptInvitation,
    clearErrors: form.clear,
    logOut,
  }
})
