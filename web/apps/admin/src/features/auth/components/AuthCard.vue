<script setup lang="ts">
/**
 * AuthCard is the centred card the screens outside the management shell
 * share: login, sign-up and the store picker.
 */
</script>

<template>
  <main class="auth">
    <div class="auth__card">
      <div class="auth__brand">
        <span
          class="auth__mark"
          aria-hidden="true"
        />
        Encomendo
      </div>
      <slot />
    </div>
  </main>
</template>

<style scoped>
.auth {
  display: grid;
  place-items: center;
  min-height: 100vh;
  padding: 16px;
  box-sizing: border-box;
}
.auth__card {
  display: flex;
  flex-direction: column;
  gap: 16px;
  width: 100%;
  max-width: 400px;
  padding: 24px;
  border: 1px solid var(--enc-hairline);
  border-radius: var(--enc-radius-card);
  background: var(--enc-surface);
}
.auth__brand {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
}
.auth__mark {
  width: 20px;
  height: 20px;
  border-radius: 6px;
  background: var(--enc-accent);
}
:slotted(h1) {
  margin: 0;
  font-size: 20px;
}
:slotted(form) {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
:slotted([role='alert']) {
  margin: 0;
  color: var(--enc-ink);
  font-weight: 600;
}
</style>
