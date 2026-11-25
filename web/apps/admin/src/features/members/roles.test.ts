import { describe, expect, it } from 'vitest'

import { invitableRoleOptions, roleLabel } from './roles'

describe('roleLabel', () => {
  it.each([
    ['owner', 'Dono'],
    ['admin', 'Admin'],
    ['staff', 'Funcionário'],
    ['viewer', 'Visualizador'],
  ] as const)('shows %s as %s (ADR 0017)', (role, label) => {
    expect(roleLabel(role)).toBe(label)
  })
})

describe('invitableRoleOptions', () => {
  it('offers every role but owner, most powerful first', () => {
    expect(invitableRoleOptions).toEqual([
      { value: 'admin', label: 'Admin' },
      { value: 'staff', label: 'Funcionário' },
      { value: 'viewer', label: 'Visualizador' },
    ])
  })
})
