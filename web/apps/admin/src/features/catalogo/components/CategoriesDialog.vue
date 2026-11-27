<script setup lang="ts">
/**
 * CategoriesDialog is "Categorias": the store's categories in their order,
 * with move up and down, rename, delete and a new one. Without
 * `catalog:write` it only lists them.
 */
import type { Category } from '@encomendo/api'
import { BaseButton, BaseDialog, BaseInput, IconArrowDown, IconArrowUp } from '@encomendo/ui'
import { ref } from 'vue'

defineProps<{
  categories: readonly Category[]
  canWrite: boolean
  /** error is why a move or deletion was refused. */
  error: string | null
  /** formError and fieldErrors come from the last add or rename. */
  formError: string | null
  fieldErrors: Readonly<Record<string, string>>
}>()

const emit = defineEmits<{
  close: []
  add: [name: string]
  rename: [category: Category, name: string]
  move: [category: Category, position: number]
  remove: [category: Category]
  clearErrors: []
}>()

const newName = ref('')
const renaming = ref<string | null>(null)
const renamed = ref('')

function start(category: Category): void {
  emit('clearErrors')
  renaming.value = category.id
  renamed.value = category.name
}

function stop(): void {
  renaming.value = null
  emit('clearErrors')
}

defineExpose({
  /** added clears the new-category field once the category exists. */
  added(): void {
    newName.value = ''
  },
  /** renamedDone closes the rename field once the new name is saved. */
  renamedDone(): void {
    renaming.value = null
  },
})
</script>

<template>
  <BaseDialog
    title="Categorias"
    @close="emit('close')"
  >
    <div class="body">
      <p
        v-if="error"
        role="alert"
        class="error"
      >
        {{ error }}
      </p>
      <ol v-if="categories.length > 0">
        <li
          v-for="(category, index) in categories"
          :key="category.id"
        >
          <form
            v-if="renaming === category.id"
            class="rename"
            @submit.prevent="emit('rename', category, renamed)"
          >
            <BaseInput
              v-model="renamed"
              :label="`Novo nome de ${category.name}`"
              :error="fieldErrors.name ?? null"
            />
            <BaseButton
              type="submit"
              variant="primary"
            >
              Salvar
            </BaseButton>
            <BaseButton @click="stop">
              Cancelar
            </BaseButton>
          </form>
          <template v-else>
            <span class="name">{{ category.name }}</span>
            <span
              v-if="canWrite"
              class="actions"
            >
              <BaseButton
                :disabled="index === 0"
                :aria-label="`Mover ${category.name} para cima`"
                @click="emit('move', category, index - 1)"
              >
                <IconArrowUp
                  :size="16"
                  aria-hidden="true"
                />
              </BaseButton>
              <BaseButton
                :disabled="index === categories.length - 1"
                :aria-label="`Mover ${category.name} para baixo`"
                @click="emit('move', category, index + 1)"
              >
                <IconArrowDown
                  :size="16"
                  aria-hidden="true"
                />
              </BaseButton>
              <BaseButton
                :aria-label="`Renomear ${category.name}`"
                @click="start(category)"
              >
                Renomear
              </BaseButton>
              <BaseButton
                :aria-label="`Excluir ${category.name}`"
                @click="emit('remove', category)"
              >
                Excluir
              </BaseButton>
            </span>
          </template>
        </li>
      </ol>
      <form
        v-if="canWrite"
        class="add"
        @submit.prevent="emit('add', newName)"
      >
        <p
          v-if="formError && renaming === null"
          role="alert"
          class="error"
        >
          {{ formError }}
        </p>
        <BaseInput
          v-model="newName"
          label="Nome da categoria"
          :error="renaming === null ? (fieldErrors.name ?? null) : null"
        />
        <BaseButton type="submit">
          Nova categoria
        </BaseButton>
      </form>
      <div class="enc-sheet-actions">
        <BaseButton @click="emit('close')">
          Fechar
        </BaseButton>
      </div>
    </div>
  </BaseDialog>
</template>

<style scoped>
.body {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
ol {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  list-style: none;
}
li {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 0;
  border-bottom: 1px solid var(--enc-hairline);
}
.name {
  font-weight: 600;
}
.actions {
  display: flex;
  gap: 6px;
}
.rename,
.add {
  display: flex;
  align-items: flex-end;
  gap: 8px;
}
.add {
  flex-wrap: wrap;
}
.error {
  flex-basis: 100%;
  margin: 0;
  color: var(--enc-status-alert-text);
  font-weight: 600;
}

/* Phone: a category's name and its four buttons stack, and a form's field and button do too. */
@media (max-width: 640px) {
  li {
    flex-wrap: wrap;
  }
  .name {
    overflow-wrap: anywhere;
  }
  .actions {
    flex-wrap: wrap;
    width: 100%;
  }
  .actions > .enc-button {
    flex: 1 1 auto;
    min-width: var(--enc-control-height);
  }
  .rename,
  .add {
    flex-direction: column;
    align-items: stretch;
    width: 100%;
  }
}
</style>
