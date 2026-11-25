<script setup lang="ts">
import { BaseButton, BaseInput } from '@encomendo/ui'
import { storeToRefs } from 'pinia'
import { nextTick, onMounted, ref, useTemplateRef } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { safeRedirect } from '../../../router/redirect'
import { useAuthStore } from '../store'

const auth = useAuthStore()
const { submitting, formError, fieldErrors } = storeToRefs(auth)
const route = useRoute()
const router = useRouter()

const email = ref('')
const password = ref('')
const emailField = useTemplateRef('emailField')
const passwordField = useTemplateRef('passwordField')

onMounted(() => auth.clearErrors())

async function submit(): Promise<void> {
  const signedIn = await auth.logIn({ email: email.value, password: password.value })
  if (signedIn) {
    await router.replace(safeRedirect(route.query.redirect) ?? { name: 'hoje' })
    return
  }
  // Keyboard and screen reader users land on the first field to fix, and
  // hear its error through aria-describedby.
  await nextTick()
  if (fieldErrors.value.email !== undefined) emailField.value?.focus()
  else if (fieldErrors.value.password !== undefined) passwordField.value?.focus()
}
</script>

<template>
  <main class="login">
    <form
      class="login__card"
      novalidate
      aria-labelledby="login-title"
      @submit.prevent="submit"
    >
      <div class="login__brand">
        <span
          class="login__mark"
          aria-hidden="true"
        />
        Encomendo
      </div>
      <h1 id="login-title">
        Entrar
      </h1>
      <p
        v-if="formError"
        role="alert"
        class="login__error"
      >
        {{ formError }}
      </p>
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
  </main>
</template>

<style scoped>
.login {
  display: grid;
  place-items: center;
  min-height: 100vh;
  padding: 16px;
  box-sizing: border-box;
}
.login__card {
  display: flex;
  flex-direction: column;
  gap: 16px;
  width: 100%;
  max-width: 360px;
  padding: 24px;
  border: 1px solid var(--enc-hairline);
  border-radius: var(--enc-radius-card);
  background: var(--enc-surface);
}
.login__brand {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
}
.login__mark {
  width: 20px;
  height: 20px;
  border-radius: 6px;
  background: var(--enc-accent);
}
h1 {
  margin: 0;
  font-size: 20px;
}
.login__error {
  margin: 0;
  color: var(--enc-ink);
  font-weight: 600;
}
</style>
