<script setup lang="ts">
/**
 * InvitationView opens an invitation link (`/convite#token=…`): it shows the
 * store and the email, then asks a new member for a name and a password, or
 * an existing account for its password. Joining loads the store with a full
 * page load, so nothing of another session stays in memory.
 */
import { BaseButton, BaseInput } from '@encomendo/ui'
import { storeToRefs } from 'pinia'
import { computed, nextTick, ref, useTemplateRef, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'

import { useFirstErrorFocus } from '../../../composables/useFirstErrorFocus'
import { loadPage } from '../../../pageLoad'
import AuthCard from '../components/AuthCard.vue'
import { invitationGone, useAuthStore } from '../store'

const auth = useAuthStore()
const { submitting, formError, fieldErrors, invitation, invitationState } = storeToRefs(auth)
const route = useRoute()
const router = useRouter()

// The token travels in the fragment, so it never reaches a server log (ADR 0011).
const token = computed(() => new URLSearchParams(route.hash.slice(1)).get('token') ?? '')
const name = ref('')
const password = ref('')
const heading = useTemplateRef<HTMLElement>('heading')
const focusFirstError = useFirstErrorFocus([
  ['name', useTemplateRef('nameField')],
  ['password', useTemplateRef('passwordField')],
])

// Pasting another link into this tab changes only the fragment, not the page.
watch(token, (current) => auth.openInvitation(current), { immediate: true })

async function retry(): Promise<void> {
  await auth.openInvitation(token.value)
  // The button just clicked is gone; give focus a place to land.
  await nextTick()
  heading.value?.focus()
}

async function accept(): Promise<void> {
  if (await auth.acceptInvitation(token.value, { name: name.value, password: password.value })) {
    loadPage(router.resolve({ name: 'hoje' }).href)
    return
  }
  await focusFirstError(fieldErrors.value)
}
</script>

<template>
  <AuthCard>
    <p
      v-if="invitationState === 'loading'"
      role="status"
    >
      Verificando o convite…
    </p>
    <template v-else-if="invitationState === 'ready' && invitation">
      <h1
        id="invitation-title"
        ref="heading"
        tabindex="-1"
      >
        Entrar na equipe da loja {{ invitation.tenant.name }}
      </h1>
      <p v-if="invitation.accountExists">
        Você já tem uma conta com {{ invitation.email }}. Entre com a sua senha para aceitar.
      </p>
      <p v-else>
        Convite para {{ invitation.email }}. Crie sua conta para aceitar.
      </p>
      <p
        v-if="formError"
        role="alert"
      >
        {{ formError }}
      </p>
      <form
        novalidate
        aria-labelledby="invitation-title"
        @submit.prevent="accept"
      >
        <BaseInput
          v-if="!invitation.accountExists"
          ref="nameField"
          v-model="name"
          label="Nome"
          autocomplete="name"
          required
          :error="fieldErrors.name"
        />
        <BaseInput
          ref="passwordField"
          v-model="password"
          label="Senha"
          type="password"
          :autocomplete="invitation.accountExists ? 'current-password' : 'new-password'"
          :hint="invitation.accountExists ? null : 'Pelo menos 8 caracteres.'"
          required
          :error="fieldErrors.password"
        />
        <BaseButton
          variant="primary"
          type="submit"
          :disabled="submitting"
        >
          Aceitar convite
        </BaseButton>
      </form>
      <p v-if="invitation.accountExists">
        <RouterLink :to="{ name: 'forgot-password' }">
          Esqueci a senha
        </RouterLink>
      </p>
    </template>
    <template v-else-if="invitationState === 'failed'">
      <h1>Convite</h1>
      <p role="alert">
        {{ formError }}
      </p>
      <BaseButton
        variant="primary"
        @click="retry"
      >
        Tentar de novo
      </BaseButton>
    </template>
    <template v-else-if="invitationState === 'missing'">
      <h1>Abra o convite pelo e-mail</h1>
      <p>Este endereço não traz o convite. Abra de novo o link que chegou no seu e-mail.</p>
    </template>
    <template v-else>
      <h1>Convite indisponível</h1>
      <p>{{ invitationGone }}</p>
    </template>
    <p>
      <RouterLink :to="{ name: 'login' }">
        Entrar
      </RouterLink>
    </p>
  </AuthCard>
</template>

<style scoped>
p {
  margin: 0;
}
</style>
