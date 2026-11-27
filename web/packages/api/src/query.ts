/** PageRequest asks for one page of a list: after `cursor`, at most `limit` items (the API caps it at 100). */
export interface PageRequest {
  cursor?: string
  limit?: number
}

/**
 * queryString turns defined parameters into "?a=1&b=2", or "" when none is
 * set. A parameter left undefined is left out, so a caller never sends
 * "q=undefined"; `false` is kept, because "includeArchived=false" is an answer.
 */
export function queryString(params: Record<string, string | number | boolean | undefined>): string {
  const query = new URLSearchParams()
  for (const [name, value] of Object.entries(params)) {
    if (value !== undefined) query.set(name, String(value))
  }
  const text = query.toString()
  return text === '' ? '' : `?${text}`
}
