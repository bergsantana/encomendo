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
const { submitting, formError, fieldErrors, notice } = storeToRefs(auth)
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

async function submit(): Promise<void> {
  if (await auth.logIn({ email: email.value, password: password.value })) {
    await router.replace(safeRedirect(route.query.redirect) ?? { name: 'hoje' })
    return
  }
  await focusFirstError(fieldErrors.value)
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
