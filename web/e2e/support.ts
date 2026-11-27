import { expect, type APIRequestContext, type Page } from '@playwright/test'

// Helpers the real-stack flows share: sign-up, the first store, log-out and
// the Mailpit inbox. See admin.spec.ts for how to run them.
export const mailpitUrl = process.env.E2E_MAILPIT_URL ?? ''
export const password = 'segredo-forte-1'

/** unique makes an email and a store link that no earlier run or parallel test used. */
export function unique(label: string): { email: string; slug: string } {
  const id = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 8)}`
  return { email: `${label}-${id}@e2e.test`, slug: `${label}-${id}` }
}

export async function signUp(page: Page, name: string, email: string): Promise<void> {
  await page.goto('/criar-conta')
  await page.getByLabel('Nome', { exact: true }).fill(name)
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha').fill(password)
  await page.getByRole('button', { name: 'Criar conta' }).click()
  await expect(page.getByRole('heading', { name: 'Crie sua loja' })).toBeVisible()
}

export async function createStore(page: Page, name: string, slug: string): Promise<void> {
  await page.getByLabel('Nome da loja').fill(name)
  await page.getByLabel('Link da loja').fill(slug)
  await page.getByRole('button', { name: 'Criar loja' }).click()
  await expect(page).toHaveURL(/\/hoje$/)
  await expect(page.getByRole('heading', { name: 'Hoje' })).toBeVisible()
}

export async function logOut(page: Page): Promise<void> {
  await page.getByRole('navigation', { name: 'Menu principal' }).getByRole('button', { name: 'Sair' }).click()
  await expect(page.getByRole('heading', { name: 'Entrar' })).toBeVisible()
}

/**
 * latestEmail polls Mailpit until `count` messages for `to` have arrived
 * and returns the text body of the newest one. Mailpit lists newest first.
 */
export async function latestEmail(request: APIRequestContext, to: string, count = 1): Promise<string> {
  let text = ''
  await expect
    .poll(
      async () => {
        const found = await request.get(`${mailpitUrl}/api/v1/search`, { params: { query: `to:${to}` } })
        expect(found.ok(), `Mailpit at ${mailpitUrl} answered ${found.status()}`).toBe(true)
        const messages = ((await found.json()) as { messages?: { ID: string }[] }).messages ?? []
        if (messages.length < count) return false
        const message = await request.get(`${mailpitUrl}/api/v1/message/${messages[0].ID}`)
        text = ((await message.json()) as { Text: string }).Text
        return true
      },
      // Reset codes go out off the request path: give slow CI time.
      { message: `${count} email(s) to ${to} in Mailpit`, timeout: 15_000 },
    )
    .toBe(true)
  return text
}
