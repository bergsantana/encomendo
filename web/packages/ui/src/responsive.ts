/**
 * Phone layouts (ADR 0029). The admin has two layouts: desktop, and phone at
 * 640 px and narrower (a 390 px phone, and anything a thumb drives). Layout
 * is decided in CSS with `@media (max-width: 640px)`, so a screen never
 * renders twice; this file is for the few places that also need to know in
 * script, such as moving focus into a drawer.
 */
import { onBeforeUnmount, ref, type Ref } from 'vue'

/** phoneMaxWidth is the widest viewport, in px, that gets the phone layout. Keep it equal to the `640px` in tokens.css. */
export const phoneMaxWidth = 640

/** phoneQuery is the media query behind the phone layout, for `matchMedia`. */
export const phoneQuery = `(max-width: ${phoneMaxWidth}px)`

/**
 * useMediaQuery follows a media query as a ref. It is false where
 * `matchMedia` does not exist (tests, server rendering) and stops listening
 * when the component goes away.
 */
export function useMediaQuery(query: string): Ref<boolean> {
  const matches = ref(false)
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return matches
  const list = window.matchMedia(query)
  matches.value = list.matches
  const update = (event: MediaQueryListEvent): void => {
    matches.value = event.matches
  }
  list.addEventListener('change', update)
  onBeforeUnmount(() => list.removeEventListener('change', update))
  return matches
}

/** useIsPhone is true while the viewport gets the phone layout. Use it only for behaviour; layout belongs in CSS. */
export function useIsPhone(): Ref<boolean> {
  return useMediaQuery(phoneQuery)
}
