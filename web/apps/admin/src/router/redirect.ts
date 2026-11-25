// Any origin works as the base: only whether the redirect stays on it matters.
const probeOrigin = 'https://admin.invalid'

/**
 * safeRedirect returns the login screen's `redirect` query when it is a path
 * inside this app, without any fragment, and null otherwise, so a crafted
 * login link can never send the member to another site after they type their
 * password. It is also safe to hand to a full page load, which, unlike the
 * router, follows whatever URL the browser makes of it.
 */
export function safeRedirect(value: unknown): string | null {
  if (typeof value !== 'string' || !value.startsWith('/')) return null
  // URL parsers drop tabs and line breaks and read "\" as "/", so
  // "/\t/evil.example" or "/\evil.example" would name another host.
  if (hasControlOrBackslash(value)) return null
  const url = new URL(value, probeOrigin)
  if (url.origin !== probeOrigin) return null
  return pathWithoutFragment(value)
}

/**
 * pathWithoutFragment drops the "#…" part of a route's full path. Links such
 * as invitations keep their token in the fragment so it never reaches a
 * server; copied into a query string it would land in access logs and
 * Referer headers. vue-router encodes "#" everywhere else, so the first one
 * starts the fragment.
 */
export function pathWithoutFragment(fullPath: string): string {
  const hash = fullPath.indexOf('#')
  return hash === -1 ? fullPath : fullPath.slice(0, hash)
}

function hasControlOrBackslash(value: string): boolean {
  for (const char of value) {
    const code = char.charCodeAt(0)
    if (code < 0x20 || code === 0x7f || char === '\\') return true
  }
  return false
}
