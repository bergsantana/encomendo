const focusable = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

/**
 * trapTab keeps Tab and Shift+Tab inside `container`: from the last element
 * it wraps to the first, and back. Call it from the container's keydown
 * handler; it does nothing for other keys. The container itself (focused
 * with tabindex -1 to read its title first) counts as "before the first".
 */
export function trapTab(event: KeyboardEvent, container: HTMLElement): void {
  if (event.key !== 'Tab') return
  const inside = [...container.querySelectorAll<HTMLElement>(focusable)]
  const first = inside[0]
  const last = inside[inside.length - 1]
  if (first === undefined || last === undefined) return
  const active = document.activeElement
  if (event.shiftKey && (active === first || active === container)) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && active === last) {
    event.preventDefault()
    first.focus()
  }
}
