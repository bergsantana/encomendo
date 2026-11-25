<script setup lang="ts">
/**
 * ForgotPasswordView is "Esqueci a senha": the email first, then the 6-digit
 * code from the email and the new password. It reads the same whether or not
 * the email has an account, as the API does (ADR 0011).
 */
import { BaseButton, BaseInput } from '@encomendo/ui'
import { storeToRefs } from 'pinia'
import { nextTick, onMounted, ref, useTemplateRef } from 'vue'
import { RouterLink, useRouter } from 'vue-router'

import { useFirstErrorFocus } from '../../../composables/useFirstErrorFocus'
import AuthCard from '../components/AuthCard.vue'
import { useAuthStore } from '../store'

const auth = useAuthStore()
const { submitting, formError, fieldErrors, resetEmail, resetNotice } = storeToRefs(auth)
const router = useRouter()

const email = ref('')
const code = ref('')
const password = ref('')
const emailField = useTemplateRef('emailField')
const codeField = useTemplateRef('codeField')
const focusFirstError = useFirstErrorFocus([
  ['email', emailField],
  ['code', codeField],
  ['password', useTemplateRef('passwordField')],
])

onMounted(() => auth.restartReset())

async function startOver(): Promise<void> {
  auth.restartReset()
  // The button just clicked is gone; focus the field to fill.
  await nextTick()
  emailField.value?.focus()
}

async function send(): Promise<void> {
  if (await auth.requestReset(email.value)) {
    await nextTick()
    codeField.value?.focus()
    return
  }
  await focusFirstError(fieldErrors.value)
}

async function change(): Promise<void> {
  if (await auth.confirmReset({ code: code.value, password: password.value })) {
    await router.replace({ name: 'login' })
    return
  }
  await focusFirstError(fieldErrors.value)
}
</script>

<template>
  <AuthCard>
    <h1 id="reset-title">
      Esqueci a senha
    </h1>
    <p
      v-if="resetNotice"
      role="status"
    >
      {{ resetNotice }}
    </p>
    <p
      v-if="formError"
      role="alert"
    >
      {{ formError }}
    </p>
    <form
      v-if="resetEmail === null"
      novalidate
      aria-labelledby="reset-title"
      @submit.prevent="send"
    >
      <p>Informe o e-mail da sua conta. Enviaremos um código para você criar uma nova senha.</p>
      <BaseInput
        ref="emailField"
        v-model="email"
        label="E-mail"
        type="email"
        autocomplete="username"
        required
        :error="fieldErrors.email"
      />
      <BaseButton
        variant="primary"
        type="submit"
        :disabled="submitting"
      >
        Enviar código
      </BaseButton>
    </form>
    <template v-else>
      <form
        novalidate
        aria-labelledby="reset-title"
        @submit.prevent="change"
      >
        <BaseInput
          ref="codeField"
          v-model="code"
          label="Código"
          autocomplete="one-time-code"
          inputmode="numeric"
          required
          :error="fieldErrors.code"
        />
        <BaseInput
          ref="passwordField"
          v-model="password"
          label="Nova senha"
          type="password"
          autocomplete="new-password"
          hint="Pelo menos 8 caracteres."
          required
          :error="fieldErrors.password"
        />
        <BaseButton
          variant="primary"
          type="submit"
          :disabled="submitting"
        >
          Alterar senha
        </BaseButton>
      </form>
      <div class="actions">
        <BaseButton
          :disabled="submitting"
          @click="auth.resendReset()"
        >
          Enviar outro código
        </BaseButton>
        <BaseButton @click="startOver">
          Usar outro e-mail
        </BaseButton>
      </div>
    </template>
    <p>
      <RouterLink :to="{ name: 'login' }">
        Voltar para o login
      </RouterLink>
    </p>
  </AuthCard>
</template>

<style scoped>
.actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
p {
  margin: 0;
}
</style>
