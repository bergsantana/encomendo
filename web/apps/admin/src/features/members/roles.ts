import type { InvitableRole, Role } from '@encomendo/api'
import type { SelectOption } from '@encomendo/ui'

// Product wording, the same in every store and niche (ADR 0017,
// docs/product/design-document.md, "Roles"). Code never branches on it.
const labels: Record<Role, string> = {
  owner: 'Dono',
  admin: 'Admin',
  staff: 'Funcionário',
  viewer: 'Visualizador',
}

/** roleLabel is how the apps call a role. */
export function roleLabel(role: Role): string {
  return labels[role]
}

const invitable: readonly InvitableRole[] = ['admin', 'staff', 'viewer']

/** invitableRoleOptions are the roles a member can be given or invited with: every one but owner. */
export const invitableRoleOptions: readonly SelectOption[] = invitable.map((role) => ({ value: role, label: labels[role] }))

/** isInvitableRole narrows a value from a select back to a role the API accepts. */
export function isInvitableRole(value: string): value is InvitableRole {
  return (invitable as readonly string[]).includes(value)
}
