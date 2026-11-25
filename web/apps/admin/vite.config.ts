import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vitest/config'

// In dev the API runs on the local stack (make dev-up); proxy it so the app
// stays same-origin, as it is behind nginx and in production.
const apiTarget = process.env.API_URL ?? 'http://localhost:8081'
const proxy = Object.fromEntries(['/api', '/auth', '/healthz', '/readyz'].map((path) => [path, apiTarget]))

export default defineConfig({
  plugins: [vue()],
  server: { port: 5173, proxy },
  preview: { port: 4173 },
  test: {
    environment: 'jsdom',
    coverage: {
      provider: 'v8',
      include: ['src/**/*.{ts,vue}'],
      exclude: ['src/main.ts'],
      thresholds: { lines: 80, functions: 80, branches: 80, statements: 80 },
    },
  },
})
