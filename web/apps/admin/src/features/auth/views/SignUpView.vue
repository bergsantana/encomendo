<script setup lang="ts">
import { BaseButton, BaseInput } from '@encomendo/ui'
import { storeToRefs } from 'pinia'
import { onMounted, ref, useTemplateRef } from 'vue'
import { RouterLink, useRouter } from 'vue-router'

import AuthCard from '../components/AuthCard.vue'
import { useFirstErrorFocus } from '../../../composables/useFirstErrorFocus'
import { useAuthStore } from '../store'

const auth = useAuthStore()
const { submitting, formError, fieldErrors } = storeToRefs(auth)
const router = useRouter()

const name = ref('')
const email = ref('')
const password = ref('')
const focusFirstError = useFirstErrorFocus([
  ['name', useTemplateRef('nameField')],
  ['email', useTemplateRef('emailField')],
  ['password', useTemplateRef('passwordField')],
])

onMounted(() => auth.clearErrors())

async function submit(): Promise<void> {
  // A new account has no store yet: the picker offers to create one.
  if (await auth.signUp({ name: name.value, email: email.value, password: password.value })) {
    await router.replace({ name: 'stores' })
    return
  }
  await focusFirstError(fieldErrors.value)
}
</script>

<template>
  <AuthCard>
    <h1 id="signup-title">
      Criar conta
    </h1>
    <p
      v-if="formError"
      role="alert"
    >
      {{ formError }}
    </p>
    <form
      novalidate
      aria-labelledby="signup-title"
      @submit.prevent="submit"
    >
      <BaseInput
        ref="nameField"
        v-model="name"
        label="Nome"
        autocomplete="name"
        required
        :error="fieldErrors.name"
      />
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
        Criar conta
      </BaseButton>
    </form>
    <p>
      <RouterLink :to="{ name: 'login' }">
        Já tenho conta
      </RouterLink>
    </p>
  </AuthCard>
</template>
