import {
  ApiError,
  changeMemberRole,
  type Invitation,
  type InvitableRole,
  inviteMember,
  listInvitations,
  listMembers,
  type Member,
  removeMember,
  revokeInvitation,
} from '@encomendo/api'
import { defineStore } from 'pinia'
import { ref } from 'vue'

import { api } from '../../api'
import { type FormMessages, useForm } from '../../composables/useForm'
import { usePagination } from '../../composables/usePagination'
import { errorMessage } from '../../errorMessage'
import { useSessionStore } from '../../stores/session'
import { roleLabel } from './roles'
import { invitationKey, type RowError } from './rows'

/** NewInvitation is what the invite form collects. */
export interface NewInvitation {
  email: string
  role: InvitableRole
}

// Teams are small; one page is usually the whole team.
const pageSize = 100

const fixTheFields = 'Confira os campos destacados.'
const notSent = 'O convite foi salvo, mas o e-mail não saiu. Use "Reenviar" para tentar de novo.'
const alreadyMember = 'Esta pessoa já faz parte da equipe.'
const lostPermission = 'Você não tem mais permissão para gerenciar a equipe.'

const inviteMessages: FormMessages = {
  byCode: { invalid: fixTheFields, already_member: fixTheFields, email_unavailable: notSent },
  fieldByCode: { already_member: ['email', alreadyMember] },
}
const memberMessages = { forbidden: lostPermission, not_found: 'Esta pessoa não faz mais parte da equipe.' }
const invitationMessages = {
  forbidden: lostPermission,
  not_found: 'Este convite não está mais pendente.',
  already_member: alreadyMember,
  email_unavailable: notSent,
}

// Codes meaning the row is gone or no longer what the screen shows: the
// lists are read again, so the message goes at the top, not in the row.
const staleCodes = new Set(['not_found', 'already_member'])

/**
 * useMembersStore backs the Equipe screen: the team, the pending
 * invitations, and the changes to both. Every call needs `members:manage`,
 * which the API checks against the caller's current role (ADR 0011).
 */
export const useMembersStore = defineStore('members', () => {
  const session = useSessionStore()
  const members = usePagination((cursor) => listMembers(api, { cursor, limit: pageSize }))
  const invitations = usePagination((cursor) => listInvitations(api, { cursor, limit: pageSize }))
  const invite = useForm()
  /** notice is the last change that went through, for a status line. */
  const notice = ref<string | null>(null)
  /** error is why the last read, or a change to a row now gone, failed: for the alert at the top. */
  const error = ref<string | null>(null)
  /** rowError is why the last change to a row still on screen failed: for that row. */
  const rowError = ref<RowError | null>(null)
  /** busy is the key of the row being saved, so its buttons wait. */
  const busy = ref<string | null>(null)

  /** open reads the team and the pending invitations. */
  async function open(): Promise<void> {
    invite.clear()
    await run(memberMessages, reloadLists)
  }

  /**
   * changeRole gives a member another role and resolves whether it went
   * through. Changing one's own role refreshes the session, so the
   * permissions shown follow at once.
   */
  async function changeRole(member: Member, role: InvitableRole): Promise<boolean> {
    const changed = await run(memberMessages, () => changeMemberRole(api, member.userId, role), member.userId)
    if (changed === undefined) return false
    members.items.value = members.items.value.map((item) => (item.userId === member.userId ? changed : item))
    notice.value = `${member.name} agora é ${roleLabel(changed.role)}.`
    await refreshIfMe(member)
    return true
  }

  /** remove takes a member off the team; they keep their account. Removing oneself ends one's access to the store. */
  async function remove(member: Member): Promise<boolean> {
    const removed = await run(
      memberMessages,
      async () => {
        await removeMember(api, member.userId)
        return true
      },
      member.userId,
    )
    if (removed === undefined) return false
    members.items.value = members.items.value.filter((item) => item.userId !== member.userId)
    notice.value = `${member.name} saiu da equipe.`
    await refreshIfMe(member)
    return true
  }

  /** sendInvitation emails an invitation and lists it first, in place of a pending one for the same email. */
  async function sendInvitation(input: NewInvitation): Promise<boolean> {
    const email = input.email.trim()
    clearMessages()
    const sent = await invite.attempt([['email', email, 'Informe o e-mail.']], inviteMessages, async () => {
      try {
        listInvitation(await inviteMember(api, { email, role: input.role }))
      } catch (failure) {
        // The invitation exists even though the email did not go out: list it.
        if (failure instanceof ApiError && failure.code === 'email_unavailable') await invitations.reload().catch(() => undefined)
        throw failure
      }
    })
    if (sent) notice.value = `Convite enviado para ${email}.`
    return sent
  }

  /** resend replaces a pending invitation with a new link and a new expiry, and resolves whether it went through. */
  async function resend(invitation: Invitation): Promise<boolean> {
    const key = invitationKey(invitation)
    const renewed = await run(invitationMessages, () => inviteMember(api, { email: invitation.email, role: invitation.role }), key)
    if (renewed === undefined) return false
    listInvitation(renewed)
    notice.value = `Convite reenviado para ${invitation.email}.`
    return true
  }

  /** revoke cancels a pending invitation; its link stops working. */
  async function revoke(invitation: Invitation): Promise<boolean> {
    const revoked = await run(
      invitationMessages,
      async () => {
        await revokeInvitation(api, invitation.id)
        return true
      },
      invitationKey(invitation),
    )
    if (revoked === undefined) return false
    invitations.items.value = invitations.items.value.filter((item) => item.id !== invitation.id)
    notice.value = `Convite para ${invitation.email} cancelado.`
    return true
  }

  function listInvitation(invitation: Invitation): void {
    const key = invitationKey(invitation)
    invitations.items.value = [invitation, ...invitations.items.value.filter((item) => invitationKey(item) !== key)]
  }

  /**
   * run makes one read or change and turns a refusal into pt-BR: in the row
   * when the row is still there, at the top otherwise. A stale screen is read
   * again; a `forbidden` refreshes the session, so a role lost meanwhile
   * shows (the screen then leaves for Hoje). An unsent email still re-reads
   * the invitations, whose link was replaced.
   */
  async function run<T>(messages: Partial<Record<string, string>>, call: () => Promise<T>, key: string | null = null): Promise<T | undefined> {
    clearMessages()
    busy.value = key
    try {
      return await call()
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      const message = errorMessage(failure, messages)
      const stale = staleCodes.has(failure.code)
      if (stale || key === null) error.value = message
      else rowError.value = { key, message }
      if (stale || failure.code === 'email_unavailable') await reloadLists().catch(() => undefined)
      if (failure.code === 'forbidden') await session.refresh().catch(() => undefined)
      return undefined
    } finally {
      busy.value = null
    }
  }

  async function reloadLists(): Promise<void> {
    await Promise.all([members.reload(), invitations.reload()])
  }

  function clearMessages(): void {
    notice.value = null
    error.value = null
    rowError.value = null
  }

  async function refreshIfMe(member: Member): Promise<void> {
    if (member.userId === session.user?.id) await session.refresh().catch(() => undefined)
  }

  return {
    members: members.items,
    membersLoading: members.loading,
    hasMoreMembers: members.hasMore,
    loadMoreMembers: () => run(memberMessages, members.loadMore),
    invitations: invitations.items,
    hasMoreInvitations: invitations.hasMore,
    loadMoreInvitations: () => run(invitationMessages, invitations.loadMore),
    inviting: invite.submitting,
    inviteError: invite.error,
    inviteFieldErrors: invite.fieldErrors,
    notice,
    error,
    rowError,
    busy,
    open,
    changeRole,
    remove,
    sendInvitation,
    resend,
    revoke,
  }
})
