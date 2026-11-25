import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vitest/config'

const apiTarget = process.env.API_URL ?? 'http://localhost:8081'
const proxy = Object.fromEntries(['/api', '/auth'].map((path) => [path, apiTarget]))

export default defineConfig({
  plugins: [vue()],
  // Open http://altarpm.localhost:5174 to see a store; *.localhost resolves to 127.0.0.1.
  server: { host: true, port: 5174, allowedHosts: ['.localhost'], proxy },
  preview: { port: 4174 },
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
