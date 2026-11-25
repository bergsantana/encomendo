import type { Invitation } from '@encomendo/api'

/** RowError is why the last change to one row failed, shown in that row. */
export interface RowError {
  /** key is the member's userId, or the invitation's `invitationKey`. */
  key: string
  message: string
}

/**
 * invitationKey names an invitation's row by its email, compared without
 * regard to case as the API does: "Reenviar" replaces the invitation, so its
 * id can change while the row stays.
 */
export function invitationKey(invitation: Pick<Invitation, 'email'>): string {
  return invitation.email.toLowerCase()
}
