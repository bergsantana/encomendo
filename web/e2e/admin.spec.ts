import { expect, test } from '@playwright/test'

test('admin opens on Hoje with the main menu', async ({ page }) => {
  await page.goto('/')

  await expect(page).toHaveURL(/\/hoje$/)
  await expect(page.getByRole('heading', { name: 'Hoje' })).toBeVisible()
  await expect(page.getByRole('navigation', { name: 'Menu principal' })).toContainText('Hoje')
  // Without a backend the status line must still render, in pt-BR.
  await expect(page.getByRole('status')).toBeVisible()
})
