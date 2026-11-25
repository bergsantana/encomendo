/**
 * Which site a host name points to. Each store lives at its own subdomain
 * ({slug}.encomendo.com.br); the client area lives at conta.encomendo.com.br.
 * The tenant is resolved by the backend from the same Host header, so this
 * only decides which screen to boot; it is never trusted for data access.
 */
export type Site =
  | { kind: 'store'; slug: string }
  | { kind: 'account' }
  | { kind: 'unknown' }

/** Subdomains that can never be a store slug. */
const reserved = new Set(['www', 'api', 'admin', 'app', 'conta', 'mail', 'static'])

const slugPattern = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/

/** resolveSite reads the site from a host such as "altarpm.encomendo.com.br:443". */
export function resolveSite(host: string, baseDomain: string): Site {
  const hostname = host.toLowerCase().replace(/:\d+$/, '')
  const suffix = `.${baseDomain.toLowerCase()}`
  if (!hostname.endsWith(suffix)) return { kind: 'unknown' }

  const label = hostname.slice(0, -suffix.length)
  if (label === 'conta') return { kind: 'account' }
  if (label.includes('.') || reserved.has(label) || !slugPattern.test(label)) return { kind: 'unknown' }
  return { kind: 'store', slug: label }
}
