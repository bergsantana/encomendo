import type { AuthSession, TenantSummary } from '@encomendo/api'
import { HttpResponse } from 'msw'

/** altaRpm is the store most tests run in. */
export const altaRpm: TenantSummary = { id: '0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d00', name: 'Alta RPM', slug: 'altarpm' }

/** authSession builds an AuthSession as the API returns it; override only what a test is about. */
export function authSession(overrides: Partial<AuthSession> = {}): AuthSession {
  return {
    accessToken: 'access-1',
    tokenType: 'Bearer',
    expiresIn: 900,
    user: { id: '0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d9e', name: 'Ana Souza', email: 'ana@altarpm.com.br' },
    tenant: altaRpm,
    role: 'owner',
    permissions: ['order:read', 'order:write'],
    memberships: [{ tenant: altaRpm, role: 'owner' }],
    ...overrides,
  }
}

/** problem answers like the API does when it refuses a call. */
export function problem(status: number, code: string, title: string, headers: Record<string, string> = {}): HttpResponse<string> {
  return new HttpResponse(JSON.stringify({ type: 'about:blank', title, status, code }), {
    status,
    headers: { 'Content-Type': 'application/problem+json', ...headers },
  })
}

/** gate returns a promise a mock handler can wait on, and the function that opens it. */
export function gate(): { opened: Promise<void>; open: () => void } {
  let open = () => {}
  const opened = new Promise<void>((resolve) => (open = resolve))
  return { opened, open }
}
