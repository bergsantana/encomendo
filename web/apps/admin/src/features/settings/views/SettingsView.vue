<script setup lang="ts">
/**
 * SettingsView is Configurações, the owner's screen (`settings:write`): the
 * status list the store uses for new orders, and the store defaults. It only
 * composes the store's state; the API checks the permission on every call.
 */
import { BaseButton } from '@encomendo/ui'
import { storeToRefs } from 'pinia'
import { onMounted } from 'vue'

import DefaultsForm from '../components/DefaultsForm.vue'
import WorkflowChoice from '../components/WorkflowChoice.vue'
import { useSettingsStore } from '../store'

const store = useSettingsStore()
const {
  settings,
  workflows,
  loadError,
  formKey,
  choosingId,
  workflowNotice,
  workflowError,
  workflowConflict,
  saving,
  defaultsError,
  defaultsFieldErrors,
  defaultsNotice,
  defaultsConflict,
} = storeToRefs(store)

onMounted(() => store.open())
</script>

<template>
  <section class="settings">
    <h1>Configurações</h1>

    <p
      v-if="loadError"
      role="alert"
    >
      {{ loadError }}
    </p>
    <BaseButton
      v-if="loadError"
      @click="store.open()"
    >
      Tentar de novo
    </BaseButton>
    <p
      v-else-if="settings === null"
      class="muted"
    >
      Carregando as configurações…
    </p>

    <template v-if="settings !== null">
      <WorkflowChoice
        :workflows="workflows"
        :current-id="settings.statusWorkflow.id"
        :current-version="settings.statusWorkflow.version"
        :choosing-id="choosingId"
        :notice="workflowNotice"
        :error="workflowError"
        :conflict="workflowConflict"
        @choose="store.chooseWorkflow($event)"
        @reload="store.reload()"
      />

      <p
        v-if="defaultsNotice"
        role="status"
        class="notice"
      >
        {{ defaultsNotice }}
      </p>
      <DefaultsForm
        :key="formKey"
        :settings="settings"
        :saving="saving"
        :error="defaultsError"
        :field-errors="defaultsFieldErrors"
        :conflict="defaultsConflict"
        @save="store.saveDefaults($event)"
        @reload="store.reload()"
      />
    </template>
  </section>
</template>

<style scoped>
.settings {
  display: flex;
  flex-direction: column;
  gap: 24px;
}
.settings > .enc-button {
  align-self: flex-start;
}
h1,
p {
  margin: 0;
}
.muted {
  color: var(--enc-ink-secondary);
}
.notice,
[role='alert'] {
  font-weight: 600;
}
</style>
