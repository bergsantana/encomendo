<script setup lang="ts">
/**
 * ReceberView is "Receber itens": what can be received, one group per pedido
 * de compra ("C-0012 · {fornecedor}") and then "Comprados sem pedido de
 * compra" (items bought at the counter), each line with "Tudo certo" and "Com
 * defeito". "Foto da nota fiscal" and the one main button, "Salvar
 * recebimento", close the screen. It is the screen of `/compras/receber`
 * (owner, admin and staff, `receiving:write`) and also the "Receber itens"
 * tab of Compras (`embedded`, under that screen's own title). It is built at
 * workshop sizes: 18 px text, 56 px targets and a 64 px main button
 * (`enc-workshop`). Nothing here is worked out: the checks and photos live in
 * the store, and what a receipt completed is the API's answer. `?pedido=` (the
 * pedido de compra's id, from its own screen) narrows the list to that one
 * pedido de compra, with "Mostrar todos" to see the rest.
 */
import { BaseButton, BaseInput } from '@encomendo/ui'
import { storeToRefs } from 'pinia'
import { computed, onMounted, ref } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'

import PhotoPicker from '../components/PhotoPicker.vue'
import ReceiptLine from '../components/ReceiptLine.vue'
import { backToBuyMessage, completedOrderMessage } from '../labels'
import { entryKey, invoiceTarget, receivableLines } from '../receipt'
import { useComprasStore } from '../store'

withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false })

const store = useComprasStore()
const { receivable, receiveLoading, receiveError, marks, invoicePhotos, photoErrors, uploading, saving, saveError, receipt } = storeToRefs(store)

const query = ref('')
const route = useRoute()
const router = useRouter()

/** only is the pedido de compra the list is narrowed to, or null for all of them. */
const only = computed(() => (typeof route.query.pedido === 'string' && route.query.pedido !== '' ? route.query.pedido : null))
const onlyNumber = computed(() => receivable.value?.groups.find((group) => group.supplierOrderId === only.value)?.number ?? null)

/** showAll drops the narrowing and keeps the rest of the address, such as the Compras tab. */
function showAll(): void {
  void router.replace({ query: Object.fromEntries(Object.entries(route.query).filter(([name]) => name !== 'pedido')) })
}

onMounted(() => store.openReceive())

const hasLines = computed(() => receivableLines(receivable.value).length > 0)
const checkedCount = computed(() => Object.keys(marks.value).length)
const canSave = computed(() => checkedCount.value > 0 && !saving.value && !uploading.value)

interface Section {
  key: string
  title: string
  lines: ReturnType<typeof receivableLines>
}

/** sections are the groups, then the counter items, narrowed by the search (item, order, supplier or C-number). */
const sections = computed<Section[]>(() => {
  const answer = receivable.value
  if (answer === null) return []
  const words = query.value.trim().toLowerCase()
  const matches = (...texts: string[]) => words === '' || texts.some((text) => text.toLowerCase().includes(words))
  const groups = only.value === null ? answer.groups : answer.groups.filter((group) => group.supplierOrderId === only.value)
  const found: Section[] = groups.map((group) => ({
    key: group.supplierOrderId,
    title: `${group.number} · ${group.supplierName}`,
    lines: matches(group.number, group.supplierName) ? group.lines : group.lines.filter((line) => matches(line.name, line.orderNumber)),
  }))
  if (only.value === null) found.push({ key: 'counter', title: 'Comprados sem pedido de compra', lines: answer.toBuy.filter((line) => matches(line.name, line.orderNumber)) })
  return found.filter((section) => section.lines.length > 0)
})
</script>

<template>
  <section class="receber enc-workshop">
    <h1 v-if="!embedded">
      Receber itens
    </h1>

    <p
      v-if="receiveLoading && receivable === null"
      class="muted"
    >
      Carregando o que falta receber…
    </p>
    <template v-if="receiveError">
      <p role="alert">
        {{ receiveError }}
      </p>
      <BaseButton @click="store.openReceive()">
        Tentar de novo
      </BaseButton>
    </template>

    <div
      v-if="receipt"
      class="result"
      role="status"
    >
      <p class="done">
        Recebimento salvo.
      </p>
      <p
        v-for="item in receipt.backToBuy"
        :key="item.orderItemId"
      >
        {{ backToBuyMessage(item.name) }}
      </p>
      <p
        v-for="done in receipt.completedOrders"
        :key="done.orderId"
        class="completed"
      >
        {{ completedOrderMessage(done.orderNumber) }}
        <RouterLink
          class="open-order"
          :to="{ name: 'order', params: { orderId: done.orderId } }"
          :aria-label="`Abrir pedido ${done.orderNumber}`"
        >
          Abrir pedido
        </RouterLink>
      </p>
    </div>

    <template v-if="receivable !== null">
      <p
        v-if="receivable.truncated"
        role="status"
        class="muted"
      >
        A lista está cortada. Salve o que já conferiu para ver os itens que faltam.
      </p>
      <p
        v-if="!hasLines"
        class="muted"
      >
        Nada para receber agora.
      </p>
      <template v-else>
        <div
          v-if="only"
          class="only"
        >
          <p>
            {{ onlyNumber ? `Mostrando só o pedido de compra ${onlyNumber}.` : 'Este pedido de compra não tem itens para receber.' }}
          </p>
          <BaseButton @click="showAll()">
            Mostrar todos
          </BaseButton>
        </div>
        <BaseInput
          v-model="query"
          label="Buscar item, pedido ou fornecedor"
        />
        <p class="muted">
          Marque um item só quando toda a quantidade chegou.
        </p>
        <p
          v-if="sections.length === 0 && (only === null || onlyNumber !== null)"
          class="muted"
        >
          Nenhum item encontrado.
        </p>

        <section
          v-for="section in sections"
          :key="section.key"
          class="group"
        >
          <h2>{{ section.title }}</h2>
          <ul class="lines">
            <ReceiptLine
              v-for="line in section.lines"
              :key="entryKey(line)"
              :line="line"
              :mark="marks[entryKey(line)] ?? null"
              :note-error="store.missingNoteText(entryKey(line))"
              :photo-error="photoErrors[entryKey(line)] ?? null"
              @check="store.check(entryKey(line), $event)"
              @note="store.setNote(entryKey(line), $event)"
              @add-photos="store.addPhotos(entryKey(line), $event)"
              @remove-photo="store.removePhoto(entryKey(line), $event)"
            />
          </ul>
        </section>

        <PhotoPicker
          class="invoice"
          label="Foto da nota fiscal"
          subject="nota fiscal"
          :photos="invoicePhotos"
          :error="photoErrors[invoiceTarget] ?? null"
          @add="store.addPhotos(invoiceTarget, $event)"
          @remove="store.removePhoto(invoiceTarget, $event)"
        />

        <div class="save">
          <p
            v-if="saveError"
            role="alert"
            class="error"
          >
            {{ saveError }}
          </p>
          <p
            v-if="checkedCount === 0"
            class="muted"
          >
            Marque Tudo certo ou Com defeito em ao menos um item.
          </p>
          <div class="buttons">
            <BaseButton
              variant="primary"
              :disabled="!canSave"
              @click="store.saveReceipt()"
            >
              Salvar recebimento
            </BaseButton>
            <BaseButton
              v-if="saveError"
              :disabled="!canSave"
              @click="store.saveReceipt()"
            >
              Tentar de novo
            </BaseButton>
          </div>
        </div>
      </template>
    </template>
  </section>
</template>

<style scoped>
.receber {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 16px;
}
h1,
h2,
p {
  margin: 0;
}
h2 {
  font-size: 1.1em;
}
.muted {
  color: var(--enc-ink-secondary);
}
[role='alert'],
.error {
  color: var(--enc-status-alert-text);
  font-weight: 600;
}
.result {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px;
  border: 1px solid var(--enc-hairline);
  border-radius: var(--enc-radius-card);
  background: var(--enc-surface);
}
.done {
  font-weight: 600;
}
.completed {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
}
.open-order {
  display: inline-flex;
  align-items: center;
  box-sizing: border-box;
  min-height: var(--enc-control-height);
  padding: 0 14px;
  border: 1px solid var(--enc-secondary-border);
  border-radius: var(--enc-radius-card);
  background: var(--enc-surface);
  color: var(--enc-ink);
  font-weight: 600;
  text-decoration: none;
}
.only {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
}
.group {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.lines {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin: 0;
  padding: 0;
  list-style: none;
}
/* The main button stays in reach while a long list scrolls. */
.save {
  position: sticky;
  bottom: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px 0;
  background: var(--enc-ground);
}
.buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

@media (max-width: 640px) {
  .buttons > button {
    flex: 1 1 auto;
  }
}
</style>
