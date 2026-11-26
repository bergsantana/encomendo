<script setup lang="ts">
/**
 * App is the admin root. Whenever the session ends, on any screen, it leaves
 * for login with a full page load, so no store keeps data the next session
 * should not see. Only an expired session comes back to the same page after
 * signing in; after a logout, or when another tab's login took over the
 * cookie, the next person starts on Hoje. A suspended store (ADR 0022) leaves
 * for the store picker instead, which says why.
 */
import { storeToRefs } from 'pinia'
import { watch } from 'vue'
import { RouterView, useRoute, useRouter } from 'vue-router'

import { loadPage } from './pageLoad'
import { pathWithoutFragment } from './router/redirect'
import { useSessionStore } from './stores/session'

const { ended } = storeToRefs(useSessionStore())
const route = useRoute()
const router = useRouter()

watch(ended, (reason) => {
  // A switch leaves through the store picker, which knows where to go.
  if (reason === null || reason === 'switched') return
  if (reason === 'suspended') {
    loadPage(router.resolve({ name: 'stores', query: { suspended: '1' } }).href)
    return
  }
  const query = reason === 'expired' ? { redirect: pathWithoutFragment(route.fullPath) } : {}
  loadPage(router.resolve({ name: 'login', query }).href)
})
</script>

<template>
  <RouterView />
</template>
