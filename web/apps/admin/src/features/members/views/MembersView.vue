<script setup lang="ts">
/**
 * MembersView is the Equipe screen: who is on the store's team and with
 * which role, the pending invitations, and the invite form. It needs
 * `members:manage`; when a change takes that away from the member (their
 * own role), they are sent to Hoje.
 */
import type { Invitation, InvitableRole, Member } from '@encomendo/api'
import { BaseButton } from '@encomendo/ui'
import { storeToRefs } from 'pinia'
import { nextTick, onMounted, useTemplateRef, watch } from 'vue'
import { useRouter } from 'vue-router'

import { usePermission } from '../../../composables/usePermission'
import { useSessionStore } from '../../../stores/session'
import InvitationsTable from '../components/InvitationsTable.vue'
import InviteForm from '../components/InviteForm.vue'
import MemberRow from '../components/MemberRow.vue'
import { useMembersStore } from '../store'

const store = useMembersStore()
const {
  members,
  membersLoading,
  hasMoreMembers,
  invitations,
  hasMoreInvitations,
  inviting,
  inviteError,
  inviteFieldErrors,
  notice,
  error,
  rowError,
  busy,
} = storeToRefs(store)
const { user } = storeToRefs(useSessionStore())
const canManage = usePermission('members:manage')
const router = useRouter()
const inviteForm = useTemplateRef('inviteForm')
const status = useTemplateRef<HTMLElement>('status')

onMounted(() => store.open())

watch(canManage, (allowed) => {
  if (!allowed) void router.replace({ name: 'hoje' })
})

async function invite(invitation: { email: string; role: InvitableRole }): Promise<void> {
  if (await store.sendInvitation(invitation)) inviteForm.value?.clear()
  else if (inviteFieldErrors.value.email !== undefined) await inviteForm.value?.focusEmail()
}

// The button just clicked is gone or waits (a removed row, a saved role, a
// sent invitation): focus lands on what happened.
async function focusStatus(): Promise<void> {
  await nextTick()
  status.value?.focus()
}

async function changeRole(member: Member, role: InvitableRole): Promise<void> {
  if (await store.changeRole(member, role)) await focusStatus()
}

async function remove(member: Member): Promise<void> {
  if (await store.remove(member)) await focusStatus()
}

async function resend(invitation: Invitation): Promise<void> {
  if (await store.resend(invitation)) await focusStatus()
}

async function revoke(invitation: Invitation): Promise<void> {
  if (await store.revoke(invitation)) await focusStatus()
}
</script>

<template>
  <section class="members">
    <h1>Equipe</h1>
    <p
      v-if="notice"
      ref="status"
      role="status"
      tabindex="-1"
    >
      {{ notice }}
    </p>
    <p
      v-if="error"
      role="alert"
    >
      {{ error }}
    </p>
    <BaseButton
      v-if="error && members.length === 0 && !membersLoading"
      @click="store.open()"
    >
      Tentar de novo
    </BaseButton>

    <table
      v-if="members.length > 0"
      class="enc-cards"
    >
      <thead>
        <tr>
          <th scope="col">
            Nome
          </th>
          <th scope="col">
            E-mail
          </th>
          <th scope="col">
            Papel
          </th>
          <th scope="col">
            <span class="enc-visually-hidden">Ações</span>
          </th>
        </tr>
      </thead>
      <tbody>
        <MemberRow
          v-for="member in members"
          :key="member.userId"
          :member="member"
          :is-me="member.userId === user?.id"
          :busy="busy === member.userId"
          :error="rowError?.key === member.userId ? rowError.message : null"
          @change-role="(role) => changeRole(member, role)"
          @remove="remove(member)"
        />
      </tbody>
    </table>
    <p
      v-else-if="membersLoading"
      class="loading"
    >
      Carregando a equipe…
    </p>
    <BaseButton
      v-if="hasMoreMembers"
      @click="store.loadMoreMembers()"
    >
      Carregar mais
    </BaseButton>

    <InviteForm
      ref="inviteForm"
      :submitting="inviting"
      :error="inviteError"
      :field-errors="inviteFieldErrors"
      @submit="invite"
    />

    <template v-if="invitations.length > 0">
      <h2>Convites pendentes</h2>
      <InvitationsTable
        :invitations="invitations"
        :busy="busy"
        :row-error="rowError"
        @resend="resend"
        @revoke="revoke"
      />
      <BaseButton
        v-if="hasMoreInvitations"
        @click="store.loadMoreInvitations()"
      >
        Carregar mais convites
      </BaseButton>
    </template>
  </section>
</template>

<style scoped>
.members {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 16px;
}
h1,
h2 {
  margin: 0;
}
h2 {
  font-size: 16px;
}
p {
  margin: 0;
}
[role='alert'],
[role='status'] {
  font-weight: 600;
}
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
.loading {
  color: var(--enc-ink-secondary);
}

/* Phone: the page is one column of full-width pieces; the tables are cards (enc-cards). */
@media (max-width: 640px) {
  .members {
    align-items: stretch;
  }
}
</style>
