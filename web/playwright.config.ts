import { defineConfig, devices } from '@playwright/test'

/**
 * End-to-end flows. Milestone 1 runs a smoke test against the built apps
 * served by `vite preview`; from milestone 2 the flows run against the full
 * docker compose stack (set E2E_BASE_URL to skip the preview servers).
 */
const external = process.env.E2E_BASE_URL
const phoneSpecs = /phone.*\.spec\.ts$/

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['github'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: external ?? 'http://localhost:4173',
    trace: 'retain-on-failure',
    locale: 'pt-BR',
  },
  projects: [
    { name: 'chromium', testIgnore: phoneSpecs, use: { ...devices['Desktop Chrome'] } },
    // The phone layouts (ADR 0029): a 390 x 844 Chromium with touch, as the Android WebView renders them.
    // Only the phone*.spec.ts files run here; the desktop specs keep the desktop project.
    { name: 'phone', testMatch: phoneSpecs, use: { ...devices['Pixel 7'], viewport: { width: 390, height: 844 } } },
  ],
  webServer: external
    ? undefined
    : [{ command: 'pnpm --filter @encomendo/admin preview --strictPort', url: 'http://localhost:4173', reuseExistingServer: !process.env.CI }],
})
