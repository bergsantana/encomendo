export { default as BaseButton } from './components/BaseButton.vue'
export { default as BaseDialog } from './components/BaseDialog.vue'
export { default as BaseInput } from './components/BaseInput.vue'
export { default as BaseSelect } from './components/BaseSelect.vue'
export type { SelectOption } from './components/BaseSelect.vue'
export { default as StatusBadge } from './components/StatusBadge.vue'
export { default as SuspendedBadge } from './components/SuspendedBadge.vue'
export { trapTab } from './focusTrap'
export { phoneMaxWidth, phoneQuery, useIsPhone, useMediaQuery } from './responsive'
export { isPhase, phases, toneForPhase } from './phase'
export type { Phase, StatusTone } from './phase'

// Tabler outline icons, re-exported so apps use the one icon set without depending on it directly.
export { IconArchive, IconArrowDown, IconArrowUp, IconMenu2, IconX } from '@tabler/icons-vue'
