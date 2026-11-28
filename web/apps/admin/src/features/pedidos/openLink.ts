/** whatsappOrigin is the only place an opened link may lead: the API's links are wa.me links. */
const whatsappOrigin = 'https://wa.me/'

/**
 * openWhatsapp opens a `wa.me` link the API returned, in a new tab, without
 * letting the page it opens reach this one. The client never builds such a
 * link: the API makes it, with the message in it (docs/product, "WhatsApp
 * messages"). Anything that is not an `https://wa.me/` link is ignored, so a
 * wrong answer can never send the person somewhere else. The Android shell
 * opens it outside the app (ADR 0029).
 */
export function openWhatsapp(url: string): void {
  if (!url.startsWith(whatsappOrigin)) return
  window.open(url, '_blank', 'noopener')
}
