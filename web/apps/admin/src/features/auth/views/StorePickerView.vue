<script setup lang="ts">
/**
 * StorePickerView is where a member without a store selected lands: right
 * after sign-up (create the first store) or with several stores (choose one).
 * Opening a store is a full page load, so nothing of another store stays in
 * memory.
 */
import { BaseButton, BaseInput } from '@encomendo/ui'
import { storeToRefs } from 'pinia'
import { computed, onMounted, ref, useTemplateRef } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import AuthCard from '../components/AuthCard.vue'
import { useFirstErrorFocus } from '../../../composables/useFirstErrorFocus'
import { loadPage } from '../../../pageLoad'
import { safeRedirect } from '../../../router/redirect'
import { useSessionStore } from '../../../stores/session'
import { useAuthStore } from '../store'

const auth = useAuthStore()
const { submitting, formError, fieldErrors, loggingOut, logOutError } = storeToRefs(auth)
const { memberships } = storeToRefs(useSessionStore())
const route = useRoute()
const router = useRouter()

const name = ref('')
const slug = ref('')
const focusFirstError = useFirstErrorFocus([
  ['name', useTemplateRef('nameField')],
  ['slug', useTemplateRef('slugField')],
])

const hasStores = computed(() => memberships.value.length > 0)
const storeDomain = import.meta.env.VITE_STORE_BASE_DOMAIN ?? 'encomendo.com.br'
// The contract's Slug rule, only to word the hint: the API still decides.
const slugRule = /^[a-z0-9]+(-[a-z0-9]+)*$/
const linkHint = computed(() => {
  const link = slug.value.trim().toLowerCase()
  const looksValid = link.length >= 3 && link.length <= 40 && slugRule.test(link)
  return looksValid ? `Sua loja ficará em ${link}.${storeDomain}.` : 'Use de 3 a 40 letras minúsculas, números e hífens.'
})

onMounted(() => auth.clearErrors())

function openStore(): void {
  // Through the router, whose links never leave this origin, then a full page load.
  const target = safeRedirect(route.query.redirect) ?? { name: 'hoje' }
  loadPage(router.resolve(target).href)
}

async function choose(tenantId: string): Promise<void> {
  if (await auth.chooseStore(tenantId)) openStore()
}

async function create(): Promise<void> {
  if (await auth.createStore({ name: name.value, slug: slug.value })) {
    openStore()
    return
  }
  await focusFirstError(fieldErrors.value)
}
</script>

<template>
  <AuthCard>
    <h1>{{ hasStores ? 'Escolha a loja' : 'Crie sua loja' }}</h1>
    <p
      v-if="formError"
      role="alert"
    >
      {{ formError }}
    </p>
    <ul
      v-if="hasStores"
      class="stores"
    >
      <li
        v-for="membership in memberships"
        :key="membership.tenant.id"
        class="store"
      >
        <BaseButton
          :disabled="submitting"
          @click="choose(membership.tenant.id)"
        >
          {{ membership.tenant.name }}
        </BaseButton>
        <span class="store__address">{{ membership.tenant.slug }}.{{ storeDomain }}</span>
      </li>
    </ul>
    <form
      novalidate
      :aria-label="hasStores ? 'Criar outra loja' : 'Crie sua loja'"
      @submit.prevent="create"
    >
      <h2 v-if="hasStores">
        Criar outra loja
      </h2>
      <BaseInput
        ref="nameField"
        v-model="name"
        label="Nome da loja"
        autocomplete="organization"
        required
        :error="fieldErrors.name"
      />
      <BaseInput
        ref="slugField"
        v-model="slug"
        label="Link da loja"
        :hint="linkHint"
        required
        :error="fieldErrors.slug"
      />
      <BaseButton
        :variant="hasStores ? 'secondary' : 'primary'"
        type="submit"
        :disabled="submitting"
      >
        Criar loja
      </BaseButton>
    </form>
    <BaseButton
      :disabled="loggingOut"
      @click="auth.logOut()"
    >
      Sair
    </BaseButton>
    <p
      v-if="logOutError"
      role="alert"
    >
      {{ logOutError }}
    </p>
  </AuthCard>
</template>

<style scoped>
.stores {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0;
  padding: 0;
  list-style: none;
}
.store {
  display: flex;
  align-items: center;
  gap: 12px;
}
.store__address {
  color: var(--enc-ink-secondary);
  font-family: var(--enc-font-mono);
}
h2 {
  margin: 8px 0 0;
  font-size: 16px;
}
</style>
