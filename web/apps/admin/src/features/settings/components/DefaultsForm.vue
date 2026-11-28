<script setup lang="ts">
/**
 * DefaultsForm is the store defaults of Configurações: "Prazos e sinal",
 * "Compras", "Estoque e Produção", "WhatsApp da loja" and "Fuso horário",
 * saved together with "Salvar". It holds the draft and hands it to the store;
 * the store sends every field with the version it read.
 */
import type { TenantSettings } from '@encomendo/api'
import { BaseButton, BaseInput } from '@encomendo/ui'
import { reactive, useId } from 'vue'

import { niche } from '../../../niche'
import { type DefaultsDraft, draftFromSettings, timeZoneChoices } from '../defaultsForm'

const props = defineProps<{
  settings: TenantSettings
  saving: boolean
  /** error is the message above the form; `fieldErrors` the ones next to the fields. */
  error: string | null
  fieldErrors: Readonly<Record<string, string>>
  /** conflict offers "Recarregar" beside the message. */
  conflict: boolean
}>()

const emit = defineEmits<{
  save: [draft: DefaultsDraft]
  reload: []
}>()

const draft = reactive<DefaultsDraft>(draftFromSettings(props.settings))
const zoneName = useId()
const purchaseHintId = useId()
const sourcingHintId = useId()
const zoneHintId = useId()
</script>

<template>
  <form
    class="form"
    novalidate
    @submit.prevent="emit('save', draft)"
  >
    <div
      v-if="error"
      class="alert"
      role="alert"
    >
      <p>{{ error }}</p>
      <BaseButton
        v-if="conflict"
        @click="emit('reload')"
      >
        Recarregar
      </BaseButton>
    </div>

    <fieldset class="group">
      <legend>Prazos e sinal</legend>
      <BaseInput
        v-model="draft.depositPercent"
        label="Sinal (%)"
        hint="Quanto o cliente paga para confirmar o pedido."
        inputmode="numeric"
        :error="fieldErrors.depositPercent ?? null"
      />
      <BaseInput
        v-model="draft.assemblyDays"
        :label="`Dias de ${niche.montagem}`"
        inputmode="numeric"
        :error="fieldErrors.assemblyDays ?? null"
      />
      <BaseInput
        v-model="draft.safetyMarginDays"
        label="Margem de segurança (dias)"
        hint="Folga somada ao prazo para não prometer em cima da hora."
        inputmode="numeric"
        :error="fieldErrors.safetyMarginDays ?? null"
      />
    </fieldset>

    <fieldset class="group">
      <legend>Compras</legend>
      <label class="enc-check">
        <input
          v-model="draft.purchaseBeforeDeposit"
          type="checkbox"
          :aria-describedby="purchaseHintId"
        >
        Liberar compras antes do sinal
      </label>
      <p
        :id="purchaseHintId"
        class="hint"
      >
        Desligado: nada é comprado antes de o cliente pagar o sinal.
      </p>
    </fieldset>

    <fieldset class="group">
      <legend>Estoque e Produção</legend>
      <label class="enc-check">
        <input
          v-model="draft.stockEnabled"
          type="checkbox"
          :aria-describedby="sourcingHintId"
        >
        Usar Estoque
      </label>
      <label class="enc-check">
        <input
          v-model="draft.productionEnabled"
          type="checkbox"
          :aria-describedby="sourcingHintId"
        >
        Usar Produção própria
      </label>
      <p
        :id="sourcingHintId"
        class="hint"
      >
        Desligados, os menus e as origens ficam escondidos.
      </p>
    </fieldset>

    <div class="group">
      <BaseInput
        v-model="draft.whatsapp"
        label="WhatsApp da loja"
        hint="Com DDD, como (92) 99123-4567."
        inputmode="tel"
        autocomplete="tel"
        :error="fieldErrors.whatsappPhone ?? null"
      />
    </div>

    <fieldset class="group">
      <legend>Fuso horário</legend>
      <p
        :id="zoneHintId"
        class="hint"
      >
        Decide quando um dia começa para os prazos e alertas.
      </p>
      <label
        v-for="zone in timeZoneChoices"
        :key="zone.value"
        class="enc-check"
      >
        <input
          v-model="draft.timeZone"
          type="radio"
          :name="zoneName"
          :value="zone.value"
          :aria-describedby="zoneHintId"
        >
        {{ zone.label }}
      </label>
    </fieldset>

    <div class="buttons">
      <BaseButton
        variant="primary"
        type="submit"
        :disabled="saving"
      >
        Salvar
      </BaseButton>
    </div>
  </form>
</template>

<style scoped>
.form {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 560px;
}
.group {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-width: 0;
  margin: 0;
  padding: 12px 16px 16px;
  border: 1px solid var(--enc-hairline);
  border-radius: var(--enc-radius-card);
}
legend {
  padding: 0 4px;
  font-weight: 600;
}
.hint {
  margin: 0;
  color: var(--enc-ink-secondary);
}
.alert {
  display: flex;
  align-items: center;
  gap: 12px;
  font-weight: 600;
  color: var(--enc-status-alert-text);
}
.alert p {
  margin: 0;
}

/* Phone: "Salvar" is as wide as the screen. */
@media (max-width: 640px) {
  .form {
    max-width: none;
  }
  .alert {
    flex-wrap: wrap;
  }
  .buttons > .enc-button {
    width: 100%;
  }
}
</style>
