<script setup lang="ts">
/**
 * MemberRow is one person on the team: name, email and role, with the role
 * to change and a removal. Picking a role only drafts it; "Alterar papel"
 * saves it, so arrow keys on the choice never save a role step by step
 * (WCAG 3.2.2). The member's own role change and every removal ask first.
 * The owner's row has neither control: the owner's role never changes and
 * the owner is never removed (ADR 0011).
 */
import type { InvitableRole, Member } from '@encomendo/api'
import { BaseButton, BaseSelect } from '@encomendo/ui'
import { computed, nextTick, ref, useId, useTemplateRef, watch } from 'vue'

import { invitableRoleOptions, isInvitableRole, roleLabel } from '../roles'

const props = withDefaults(
  defineProps<{
    member: Member
    /** isMe marks the signed-in member's own row. */
    isMe: boolean
    /** busy waits while a change to this member is saved. */
    busy: boolean
    /** error is why the last change to this member failed. */
    error?: string | null
  }>(),
  { error: null },
)

const emit = defineEmits<{
  changeRole: [role: InvitableRole]
  remove: []
}>()

const draft = ref<string>(props.member.role)
const asking = ref<'role' | 'remove' | null>(null)
const row = useTemplateRef<HTMLElement>('row')
const actions = useTemplateRef<HTMLElement>('actions')
const questionId = useId()

// A saved or re-read role replaces the draft.
watch(
  () => props.member.role,
  (role) => {
    draft.value = role
  },
)

const changed = computed(() => isInvitableRole(draft.value) && draft.value !== props.member.role)

function saveRole(): void {
  if (!isInvitableRole(draft.value)) return
  if (props.isMe) {
    void ask('role')
    return
  }
  emit('changeRole', draft.value)
}

function confirmRole(): void {
  asking.value = null
  if (isInvitableRole(draft.value)) emit('changeRole', draft.value)
}

function confirmRemove(): void {
  asking.value = null
  emit('remove')
}

// The button just clicked disappears, so focus moves to what replaces it:
// "Cancelar", the safe answer, which reads the question out with it.
async function ask(what: 'role' | 'remove'): Promise<void> {
  asking.value = what
  await nextTick()
  actions.value?.querySelector('button')?.focus()
}

// Cancelling puts things back as they were, focus included: the role's
// choice for a role question, "Remover" for a removal.
async function cancel(): Promise<void> {
  const was = asking.value
  asking.value = null
  if (was === 'role') draft.value = props.member.role
  await nextTick()
  const back = was === 'role' ? row.value?.querySelector('select') : actions.value?.querySelector('[data-action="remove"]')
  if (back instanceof HTMLElement) back.focus()
}
</script>

<template>
  <tr ref="row">
    <td>
      {{ member.name }}<span v-if="isMe"> (você)</span>
    </td>
    <td>{{ member.email }}</td>
    <td>
      <span v-if="member.role === 'owner'">{{ roleLabel('owner') }}</span>
      <BaseSelect
        v-else
        v-model="draft"
        :label="`Papel de ${member.name}`"
        hide-label
        :options="invitableRoleOptions"
      />
    </td>
    <td
      ref="actions"
      class="actions"
    >
      <template v-if="member.role !== 'owner'">
        <span
          v-if="asking === 'role'"
          class="confirm"
        >
          <span :id="questionId">Você deixará de gerenciar a equipe.</span>
          <BaseButton
            :disabled="busy"
            :aria-describedby="questionId"
            @click="cancel"
          >
            Cancelar
          </BaseButton>
          <BaseButton
            :disabled="busy"
            :aria-describedby="questionId"
            @click="confirmRole"
          >
            Confirmar
          </BaseButton>
        </span>
        <span
          v-else-if="asking === 'remove'"
          class="confirm"
        >
          <span :id="questionId">{{ isMe ? 'Você sairá da equipe e perderá o acesso a esta loja.' : `Remover ${member.name} da equipe?` }}</span>
          <BaseButton
            :disabled="busy"
            :aria-describedby="questionId"
            @click="cancel"
          >
            Cancelar
          </BaseButton>
          <BaseButton
            :disabled="busy"
            :aria-describedby="questionId"
            @click="confirmRemove"
          >
            Confirmar remoção
          </BaseButton>
        </span>
        <template v-else>
          <BaseButton
            v-if="changed"
            :disabled="busy"
            @click="saveRole"
          >
            Alterar papel<span class="enc-visually-hidden"> de {{ member.name }}</span>
          </BaseButton>
          <BaseButton
            data-action="remove"
            :disabled="busy"
            @click="ask('remove')"
          >
            Remover<span class="enc-visually-hidden">{{ ` ${member.name}` }}</span>
          </BaseButton>
        </template>
      </template>
      <p
        v-if="error"
        role="alert"
        class="row-error"
      >
        {{ error }}
      </p>
    </td>
  </tr>
</template>

<style scoped>
td {
  padding: 8px 12px;
  border-bottom: 1px solid var(--enc-hairline);
  vertical-align: middle;
}
.actions {
  text-align: right;
}
.actions > :deep(button) + :deep(button) {
  margin-left: 8px;
}
.confirm {
  display: inline-flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
}
.row-error {
  margin: 4px 0 0;
  font-weight: 600;
}
</style>
