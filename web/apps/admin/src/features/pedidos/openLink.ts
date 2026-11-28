/**
 * openWhatsapp opens a `wa.me` link the API returned, in a new tab, without
 * letting the page it opens reach this one. The client never builds such a
 * link: the API makes it, with the message in it (docs/product, "WhatsApp
 * messages"). The Android shell opens it outside the app (ADR 0029).
 */
export function openWhatsapp(url: string): void {
  window.open(url, '_blank', 'noopener')
}
