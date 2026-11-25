import { nextTick, type ShallowRef } from 'vue'

/** Focusable is anything a form can move the keyboard to, such as BaseInput. */
export interface Focusable {
  focus(): void
}

/**
 * useFirstErrorFocus returns a function that moves focus to the first field,
 * in form order, that has an error. Keyboard and screen reader users then
 * land where they need to act and hear the field's error with it.
 */
export function useFirstErrorFocus(
  fields: [name: string, field: Readonly<ShallowRef<Focusable | null>>][],
): (errors: Record<string, string>) => Promise<void> {
  return async (errors) => {
    await nextTick()
    fields.find(([name]) => errors[name] !== undefined)?.[1].value?.focus()
  }
}
