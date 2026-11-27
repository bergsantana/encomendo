<script setup lang="ts">
/**
 * InviteForm collects an email and a role to invite someone to the team. It
 * keeps what is typed; the screen sends it and, once it went through, calls
 * `clear`. Every role but owner can be invited; Funcionário is the default.
 */
import type { InvitableRole } from '@encomendo/api'
import { BaseButton, BaseInput, BaseSelect } from '@encomendo/ui'
import { computed, nextTick, ref, useTemplateRef } from 'vue'

import { invitableRoleOptions, isInvitableRole } from '../roles'

withDefaults(
  defineProps<{
    submitting: boolean
    /** error is the pt-BR message above the form, if the last attempt failed. */
    error?: string | null
    fieldErrors: Record<string, string>
  }>(),
  { error: null },
)

const emit = defineEmits<{ submit: [invitation: { email: string; role: InvitableRole }] }>()

const email = ref('')
const chosen = ref<InvitableRole>('staff')
const emailField = useTemplateRef('emailField')

const role = computed({
  get: () => chosen.value,
  set: (value: string) => {
    if (isInvitableRole(value)) chosen.value = value
  },
})

defineExpose({
  /** clear empties the form for the next invitation, keeping the role. */
  clear: (): void => {
    email.value = ''
  },
  /** focusEmail moves the keyboard to the email, the field a refusal is about. */
  focusEmail: async (): Promise<void> => {
    await nextTick()
    emailField.value?.focus()
  },
})
</script>

<template>
  <form
    novalidate
    aria-labelledby="invite-title"
    class="invite"
    @submit.prevent="emit('submit', { email, role: chosen })"
  >
    <h2 id="invite-title">
      Convidar para a equipe
    </h2>
    <p
      v-if="error"
      role="alert"
    >
      {{ error }}
    </p>
    <div class="fields">
      <BaseInput
        ref="emailField"
        v-model="email"
        label="E-mail"
        type="email"
        autocomplete="off"
        required
        :error="fieldErrors.email"
      />
      <BaseSelect
        v-model="role"
        label="Papel"
        :options="invitableRoleOptions"
      />
      <BaseButton
        variant="primary"
        type="submit"
        :disabled="submitting"
      >
        Enviar convite
      </BaseButton>
    </div>
  </form>
</template>

<style scoped>
.invite {
  display: flex;
  flex-direction: column;
  gap: 12px;
  max-width: 640px;
}
h2 {
  margin: 0;
  font-size: 16px;
}
p {
  margin: 0;
  font-weight: 600;
}
.fields {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: 12px;
}

/* Phone: one field per row, the button last and full width. */
@media (max-width: 640px) {
  .fields {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
