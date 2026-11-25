import { describe, expect, it } from 'vitest'

import { pathWithoutFragment, safeRedirect } from './redirect'

describe('safeRedirect', () => {
  it.each(['/hoje', '/financeiro?mes=2026-11', '/pedidos/P-0231'])('keeps the in-app path %s', (path) => {
    expect(safeRedirect(path)).toBe(path)
  })

  it('drops a fragment, which may carry a token', () => {
    expect(safeRedirect('/convite?x=1#token=secret')).toBe('/convite?x=1')
  })

  it.each([
    ['another site', 'https://evil.example/entrar'],
    ['a protocol-relative URL', '//evil.example'],
    ['a backslash trick', '/\\evil.example'],
    // URL parsers drop tabs and line breaks, so these become "//evil.example" when loaded.
    ['a tab before a second slash', '/\t/evil.example/entrar'],
    ['a line feed before a second slash', '/\n/evil.example'],
    ['a carriage return before a backslash', '/\r\\evil.example'],
    ['a tab before a backslash', '/\t\\evil.example'],
    ['any other control character', '/hoje\u0000'],
    ['a backslash anywhere', '/hoje\\x'],
    ['a script URL', 'javascript:alert(1)'],
    ['a relative path', 'hoje'],
    ['an empty value', ''],
    ['a repeated query value', ['/hoje', '/financeiro']],
    ['no value', undefined],
  ])('refuses %s', (_, value) => {
    expect(safeRedirect(value)).toBeNull()
  })
})

describe('pathWithoutFragment', () => {
  it.each([
    ['/hoje', '/hoje'],
    ['/hoje?mes=2026-11', '/hoje?mes=2026-11'],
    ['/convite#token=secret', '/convite'],
  ])('turns %s into %s', (fullPath, path) => {
    expect(pathWithoutFragment(fullPath)).toBe(path)
  })
})
