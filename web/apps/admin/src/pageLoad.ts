/**
 * loadPage leaves the single-page app with a full page load that replaces the
 * current history entry. The app ends a session this way so every store's
 * data is dropped and nothing one session loaded is left on screen for the
 * next, not even one Back press away.
 */
export function loadPage(url: string): void {
  window.location.replace(url)
}

/** PageHost is the part of `window` the back/forward cache rule needs. */
export interface PageHost {
  addEventListener(type: 'pageshow', listener: (event: Event) => void): void
  location: { reload(): void }
}

/**
 * reloadWhenRestoredFromCache reloads the app whenever the browser restores
 * it from the back/forward cache. A cached page comes back exactly as it was
 * left, stores and screens included, so after a logout Back would show the
 * previous member's data on a shared computer. A reload starts from the
 * refresh cookie instead.
 */
export function reloadWhenRestoredFromCache(host: PageHost = window): void {
  host.addEventListener('pageshow', (event) => {
    // `persisted` is set on the pageshow of a page restored from the cache.
    if ('persisted' in event && event.persisted === true) host.location.reload()
  })
}
