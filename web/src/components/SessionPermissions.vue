<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, useId } from 'vue'
import { Check, ChevronDown, Shield, ShieldCheck } from '@lucide/vue'
import { useI18n } from '../i18n'
import type { AccessMode } from '../types'

const props = defineProps<{ value?: AccessMode; activeMode?: AccessMode; active?: boolean; disabled?: boolean; save: (mode: AccessMode) => Promise<void> }>()
const emit = defineEmits<{ busy: [value: boolean] }>()
const { t, errorMessage } = useI18n()
const root = ref<HTMLElement>()
const button = ref<HTMLButtonElement>()
const open = ref(false)
const saving = ref(false)
const error = ref<unknown>()
const panelId = useId()
const current = computed(() => props.value === 'full' ? 'full' : 'workspace')
const modes: AccessMode[] = ['workspace', 'full']
function label(mode: AccessMode) { return t(mode === 'full' ? 'permissions.full' : 'permissions.workspace') }
function close() { if (!saving.value) { open.value = false; button.value?.focus() } }
function outside(event: PointerEvent) { if (event.target instanceof Node && !root.value?.contains(event.target) && !saving.value) open.value = false }
onMounted(() => document.addEventListener('pointerdown', outside))
onBeforeUnmount(() => document.removeEventListener('pointerdown', outside))

async function choose(mode: AccessMode) {
  if (saving.value || props.disabled) return
  if (mode === current.value) { close(); return }
  saving.value = true; error.value = undefined; emit('busy', true)
  try { await props.save(mode); open.value = false; button.value?.focus() }
  catch (cause) { error.value = cause }
  finally { saving.value = false; emit('busy', false) }
}
</script>

<template>
  <div ref="root" class="session-permissions" @keydown.esc.stop.prevent="close">
    <button ref="button" type="button" class="button permissions-trigger" :aria-label="t('permissions.change')" :aria-expanded="open" :aria-controls="panelId" :disabled="disabled || saving" @click="open = !open; error = undefined">
      <component :is="current === 'full' ? ShieldCheck : Shield" :size="14" aria-hidden="true" /><span>{{ label(current) }}</span><ChevronDown :size="12" aria-hidden="true" />
    </button>
    <div v-if="open" :id="panelId" class="permissions-panel" role="group" :aria-label="t('permissions.change')" :aria-busy="saving">
      <button v-for="mode in modes" :key="mode" type="button" class="permissions-option" :aria-pressed="current === mode" :disabled="disabled || saving" @click="choose(mode)">
        <span class="permissions-option-heading">{{ label(mode) }}<Check v-if="current === mode" :size="15" aria-hidden="true" /></span>
        <span>{{ t(mode === 'full' ? 'permissions.fullHint' : 'permissions.workspaceHint') }}</span>
      </button>
      <p class="permissions-note">{{ t('permissions.nextTurn') }}</p>
      <p v-if="active" class="permissions-note">{{ t('permissions.active', { mode: label(activeMode === 'full' ? 'full' : 'workspace') }) }}</p>
      <p v-if="error" class="form-error" role="alert">{{ errorMessage(error, 'errors.accessUpdate') }}</p>
    </div>
  </div>
</template>

<style scoped>
.session-permissions { position: relative; flex-shrink: 0; }
.permissions-trigger { padding: 7px 9px; font-size: 11px; font-weight: 500; }
.permissions-panel { position: absolute; z-index: 40; top: calc(100% + 8px); right: 0; width: min(310px, calc(100vw - 40px)); padding: 6px; border: 1px solid var(--border-strong); border-radius: 10px; background: var(--surface-raised); box-shadow: 0 12px 36px rgb(0 0 0 / 18%); }
.permissions-option { display: flex; flex-direction: column; gap: 5px; width: 100%; padding: 10px; border: 0; border-radius: 6px; text-align: left; color: var(--muted); background: transparent; font-size: 11px; line-height: 1.6; }
.permissions-option:hover:not(:disabled), .permissions-option[aria-pressed="true"] { background: var(--hover); }
.permissions-option-heading { display: flex; justify-content: space-between; align-items: center; gap: 10px; color: var(--text); font-size: 12px; font-weight: 600; }
.permissions-note { margin: 8px 10px; color: var(--muted); font-size: 11px; line-height: 1.6; }
.permissions-panel .form-error { margin: 8px 10px; }
</style>
