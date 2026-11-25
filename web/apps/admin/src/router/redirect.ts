/**
 * safeRedirect returns the login screen's `redirect` query when it is a path
 * inside this app, without any fragment, and null otherwise, so a crafted
 * login link can never send the member to another site after they type their
 * password.
 */
export function safeRedirect(value: unknown): string | null {
  if (typeof value !== 'string' || !value.startsWith('/')) return null
  // "//host" and "/\host" are read by browsers as another host.
  if (value.startsWith('//') || value.startsWith('/\\')) return null
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
