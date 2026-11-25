<script setup lang="ts">
/**
 * InvitationsTable lists the invitations not yet accepted: who, with which
 * role, until when the link works, and the actions to send it again (a new
 * link) or cancel it. Rows are keyed by email in lower case, since sending
 * again can give the invitation a new id.
 */
import type { Invitation } from '@encomendo/api'
import { BaseButton } from '@encomendo/ui'

import { formatDate } from '../../../format'
import { roleLabel } from '../roles'
import { invitationKey, type RowError } from '../rows'

withDefaults(
  defineProps<{
    invitations: readonly Invitation[]
    /** busy is the key of the row being saved, so its buttons wait. */
    busy: string | null
    /** rowError is why the last change to a row failed. */
    rowError?: RowError | null
  }>(),
  { rowError: null },
)

const emit = defineEmits<{
  resend: [invitation: Invitation]
  revoke: [invitation: Invitation]
}>()
</script>

<template>
  <table>
    <thead>
      <tr>
        <th scope="col">
          E-mail
        </th>
        <th scope="col">
          Papel
        </th>
        <th scope="col">
          Vale até
        </th>
        <th scope="col">
          <span class="enc-visually-hidden">Ações</span>
        </th>
      </tr>
    </thead>
    <tbody>
      <tr
        v-for="invitation in invitations"
        :key="invitationKey(invitation)"
      >
        <td>{{ invitation.email }}</td>
        <td>{{ roleLabel(invitation.role) }}</td>
        <td class="enc-number">
          {{ formatDate(invitation.expiresAt) }}
        </td>
        <td class="actions">
          <BaseButton
            :disabled="busy === invitationKey(invitation)"
            @click="emit('resend', invitation)"
          >
            Reenviar<span class="enc-visually-hidden"> convite para {{ invitation.email }}</span>
          </BaseButton>
          <BaseButton
            :disabled="busy === invitationKey(invitation)"
            @click="emit('revoke', invitation)"
          >
            Cancelar convite<span class="enc-visually-hidden"> para {{ invitation.email }}</span>
          </BaseButton>
          <p
            v-if="rowError?.key === invitationKey(invitation)"
            role="alert"
            class="row-error"
          >
            {{ rowError.message }}
          </p>
        </td>
      </tr>
    </tbody>
  </table>
</template>

<style scoped>
table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--enc-table-size, 13px);
}
th {
  padding: 8px 12px;
  border-bottom: 1px solid var(--enc-hairline);
  text-align: left;
  color: var(--enc-ink-secondary);
}
td {
  padding: 8px 12px;
  border-bottom: 1px solid var(--enc-hairline);
}
.actions {
  text-align: right;
}
.actions > :deep(button) + :deep(button) {
  margin-left: 8px;
}
.row-error {
  margin: 4px 0 0;
  font-weight: 600;
}
</style>
