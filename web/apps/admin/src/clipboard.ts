/**
 * copyText puts text on the clipboard ("Copiar mensagem") and resolves
 * whether it worked. The Clipboard API exists only in secure contexts, and a
 * browser may refuse it, so a caller must say so when this resolves false
 * instead of pretending the text was copied.
 */
export async function copyText(text: string): Promise<boolean> {
  if (typeof navigator.clipboard?.writeText !== 'function') return false
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    return false
  }
}
