import type { ApiError } from '@encomendo/api'

/**
 * storeSuspendedMessage is what staff read whenever the API answers
 * `tenant_suspended`: in the store picker and on any screen inside the store
 * (ADR 0022, "Esta loja está suspensa" in the product design document).
 */
export const storeSuspendedMessage = 'Esta loja está suspensa. Fale com o suporte do Encomendo.'

/**
 * errorMessage turns a failed call into pt-BR text for the member. `byCode`
 * holds the screen's own wording for the codes it expects, such as
 * `invalid_credentials` on login; anything else gets a general message or
 * the pt-BR title the API sent.
 */
export function errorMessage(error: ApiError, byCode: Partial<Record<string, string>> = {}): string {
  const own = byCode[error.code]
  if (own !== undefined) return own
  // One wording everywhere, whatever title the server sent.
  if (error.code === 'tenant_suspended') return storeSuspendedMessage
  if (error.status === 0) return 'Sem conexão com o servidor. Confira a internet e tente de novo.'
  if (error.code === 'rate_limited') return `Muitas tentativas. Tente de novo em ${waitFor(error.retryAfterSeconds)}.`
  if (error.status >= 500) return 'O servidor não respondeu. Tente de novo em instantes.'
  return error.title
}

/** issuesByField indexes a 400's field issues by field, keeping the first message of each. */
export function issuesByField(error: ApiError): Record<string, string> {
  const issues: Record<string, string> = {}
  for (const issue of error.fieldIssues) issues[issue.field] ??= issue.message
  return issues
}

// Every 429 carries Retry-After (ADR 0013); a minute is a safe guess if one ever does not.
function waitFor(seconds = 60): string {
  if (seconds < 60) return seconds === 1 ? '1 segundo' : `${seconds} segundos`
  const minutes = Math.ceil(seconds / 60)
  return minutes === 1 ? '1 minuto' : `${minutes} minutos`
}
