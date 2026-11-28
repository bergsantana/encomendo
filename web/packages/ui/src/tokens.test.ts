// @vitest-environment node
import { readFileSync } from 'node:fs'

import { describe, expect, it } from 'vitest'

const tokens = readFileSync(new URL('./tokens.css', import.meta.url), 'utf8')

/** rule returns the declarations of the first rule whose selector is exactly `selector`. */
function rule(selector: string): string {
  const found = new RegExp(`(?:^|\\})\\s*${selector.replace(/[.[\]]/g, '\\$&')}\\s*\\{([^}]*)\\}`, 'm').exec(tokens)
  if (found?.[1] === undefined) throw new Error(`no rule for ${selector}`)
  return found[1]
}

describe('workshop sizes (docs/product, Receber itens)', () => {
  it('sets 18 px text and 56 px targets on .enc-workshop', () => {
    const declarations = rule('.enc-workshop')
    expect(declarations).toContain('font-size: 18px')
    expect(declarations).toContain('--enc-body-size: 18px')
    expect(declarations).toContain('--enc-control-height: 56px')
  })

  it('makes the primary button 64 px, and only under .enc-workshop', () => {
    expect(rule('.enc-workshop .enc-button.enc-button--primary')).toContain('height: var(--enc-primary-height)')
    expect(rule('.enc-workshop')).toContain('--enc-primary-height: 64px')
  })

  it('keeps the admin and store tiers as they were', () => {
    expect(rule('.enc-density-admin')).toContain('--enc-control-height: 32px')
    expect(rule('.enc-density-store')).toContain('--enc-control-height: 48px')
  })
})
