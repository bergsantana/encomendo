import { ApiError } from '@encomendo/api'
import { describe, expect, it } from 'vitest'

import { errorMessage, storeSuspendedMessage } from './errorMessage'

const error = (status: number, code?: string, retryAfterSeconds?: number) =>
  new ApiError(status, { title: 'Título do servidor', ...(code === undefined ? {} : { code }) }, retryAfterSeconds)

describe('errorMessage', () => {
  it('prefers the text the screen gives for a code', () => {
    expect(errorMessage(error(401, 'invalid_credentials'), { invalid_credentials: 'E-mail ou senha incorretos.' })).toBe(
      'E-mail ou senha incorretos.',
    )
  })

  it.each([
    [1, '1 segundo'],
    [45, '45 segundos'],
    [60, '1 minuto'],
    [61, '2 minutos'],
    [undefined, '1 minuto'],
  ])('tells how long to wait after a 429 with Retry-After %s', (seconds, wait) => {
    expect(errorMessage(error(429, 'rate_limited', seconds))).toBe(`Muitas tentativas. Tente de novo em ${wait}.`)
  })

  it('falls back to the pt-BR title the API sent', () => {
    expect(errorMessage(error(409, 'slug_taken'))).toBe('Título do servidor')
  })

  it('words a suspended store the same on every screen, whatever title the server sent', () => {
    expect(errorMessage(error(403, 'tenant_suspended'))).toBe('Esta loja está suspensa. Fale com o suporte do Encomendo.')
    expect(storeSuspendedMessage).toBe('Esta loja está suspensa. Fale com o suporte do Encomendo.')
  })
})
