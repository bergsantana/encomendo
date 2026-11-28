import { afterEach, describe, expect, it, vi } from 'vitest'

import { openWhatsapp } from './openLink'

afterEach(() => vi.restoreAllMocks())

describe('openWhatsapp', () => {
  it('opens a wa.me link in a new tab, with noopener', () => {
    const open = vi.spyOn(window, 'open').mockReturnValue(null)

    openWhatsapp('https://wa.me/5592991234567?text=Ol%C3%A1')

    expect(open).toHaveBeenCalledOnce()
    expect(open).toHaveBeenCalledWith('https://wa.me/5592991234567?text=Ol%C3%A1', '_blank', 'noopener')
  })

  it.each(['', 'https://evil.example/', 'http://wa.me/5592991234567', 'https://wa.me.evil.example/x', 'javascript:alert(1)', '//wa.me/55', 'wa.me/55'])('ignores %s', (url) => {
    const open = vi.spyOn(window, 'open').mockReturnValue(null)

    openWhatsapp(url)

    expect(open).not.toHaveBeenCalled()
  })
})
