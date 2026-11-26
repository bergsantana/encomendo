<script setup lang="ts">
import { BaseButton, BaseInput } from '@encomendo/ui'
import { storeToRefs } from 'pinia'
import { onMounted, onUnmounted, ref, useTemplateRef } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'

import AuthCard from '../components/AuthCard.vue'
import { useFirstErrorFocus } from '../../../composables/useFirstErrorFocus'
import { safeRedirect } from '../../../router/redirect'
import { useAuthStore } from '../store'

const auth = useAuthStore()
const { submitting, formError, fieldErrors, notice, restoreFailed, retrying } = storeToRefs(auth)
const route = useRoute()
const router = useRouter()

const email = ref('')
const password = ref('')
const focusFirstError = useFirstErrorFocus([
  ['email', useTemplateRef('emailField')],
  ['password', useTemplateRef('passwordField')],
])

onMounted(() => auth.clearErrors())
// The notice (a password just changed) is for this visit only.
onUnmounted(() => auth.clearNotice())

function openDestination(): Promise<unknown> {
  return router.replace(safeRedirect(route.query.redirect) ?? { name: 'hoje' })
}

async function submit(): Promise<void> {
  if (await auth.logIn({ email: email.value, password: password.value })) {
    await openDestination()
    return
  }
  await focusFirstError(fieldErrors.value)
}

async function retry(): Promise<void> {
  if (await auth.retryRestore()) await openDestination()
}
</script>

<template>
  <AuthCard>
    <h1 id="login-title">
      Entrar
    </h1>
    <p
      v-if="notice"
      role="status"
    >
      {{ notice }}
    </p>
    <p
      v-if="formError"
      role="alert"
    >
      {{ formError }}
    </p>
    <p
      v-if="retrying"
      role="status"
    >
      Tentando de novo…
    </p>
    <!-- Not disabled while it waits: disabling the button just pressed would drop focus. -->
    <BaseButton
      v-if="restoreFailed || retrying"
      :aria-disabled="retrying ? 'true' : undefined"
      @click="retry"
    >
      Tentar de novo
    </BaseButton>
    <form
      novalidate
      aria-labelledby="login-title"
      @submit.prevent="submit"
    >
      <BaseInput
        ref="emailField"
        v-model="email"
        label="E-mail"
        type="email"
        autocomplete="username"
        required
        :error="fieldErrors.email"
      />
      <BaseInput
        ref="passwordField"
        v-model="password"
        label="Senha"
        type="password"
        autocomplete="current-password"
        required
        :error="fieldErrors.password"
      />
      <BaseButton
        variant="primary"
        type="submit"
        :disabled="submitting"
      >
        Entrar
      </BaseButton>
    </form>
    <p class="links">
      <RouterLink :to="{ name: 'forgot-password' }">
        Esqueci a senha
      </RouterLink>
      <RouterLink :to="{ name: 'signup' }">
        Criar conta
      </RouterLink>
    </p>
  </AuthCard>
</template>

<style scoped>
.links {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  gap: 8px;
  margin: 0;
}
</style>
