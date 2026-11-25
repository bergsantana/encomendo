import { ApiError } from '@encomendo/api'
import { describe, expect, it, vi } from 'vitest'

import { useForm } from './useForm'

const refusal = (status: number, code: string, errors: { field: string; message: string }[] = []) =>
  new ApiError(status, { title: 'Título do servidor', code, errors })

describe('useForm', () => {
  it('stops at fields that are empty or invalid, without calling the API', async () => {
    const form = useForm()
    const call = vi.fn(async () => undefined)

    const done = await form.attempt(
      [
        ['email', '', 'Informe seu e-mail.'],
        ['code', '12a', 'Informe os 6 dígitos do código.', (value) => /^\d{6}$/.test(value)],
        ['name', 'Ana', 'Informe seu nome.'],
      ],
      {},
      call,
    )

    expect(done).toBe(false)
    expect(call).not.toHaveBeenCalled()
    expect(form.error.value).toBe('Confira os campos destacados.')
    expect(form.fieldErrors.value).toEqual({ email: 'Informe seu e-mail.', code: 'Informe os 6 dígitos do código.' })
  })

  it('runs the call and reports success', async () => {
    const form = useForm()

    await expect(form.attempt([['email', 'ana@altarpm.com.br', 'Informe seu e-mail.']], {}, async () => undefined)).resolves.toBe(true)

    expect(form.error.value).toBeNull()
    expect(form.submitting.value).toBe(false)
  })

  it('shows a refusal above the form and its field issues next to the fields', async () => {
    const form = useForm()

    await form.attempt([], { byCode: { invalid: 'Confira os campos destacados.' } }, async () => {
      throw refusal(400, 'invalid', [{ field: 'password', message: 'Use pelo menos 8 caracteres.' }])
    })

    expect(form.error.value).toBe('Confira os campos destacados.')
    expect(form.fieldErrors.value).toEqual({ password: 'Use pelo menos 8 caracteres.' })
  })

  it('puts a code that is about one field next to that field', async () => {
    const form = useForm()

    await form.attempt([], { fieldByCode: { invalid_code: ['code', 'Código errado ou vencido.'] } }, async () => {
      throw refusal(400, 'invalid_code')
    })

    expect(form.fieldErrors.value).toEqual({ code: 'Código errado ou vencido.' })
  })

  it('lets programming errors through', async () => {
    const form = useForm()

    await expect(
      form.attempt([], {}, async () => {
        throw new TypeError('bug')
      }),
    ).rejects.toThrow('bug')
    expect(form.submitting.value).toBe(false)
  })

  it('forgets the last attempt when cleared', async () => {
    const form = useForm()
    await form.attempt([['email', '', 'Informe seu e-mail.']], {}, async () => undefined)

    form.clear()

    expect(form.error.value).toBeNull()
    expect(form.fieldErrors.value).toEqual({})
  })
})
