<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'
import { ChevronDown, Cpu } from '@lucide/vue'
import ModelSettings from './ModelSettings.vue'
import { useI18n } from '../i18n'
import type { Endpoint, ModelCatalog, ModelPreferences, Project, Requirement } from '../types'

const props = defineProps<{ catalog?: ModelCatalog; visible: boolean; project: Project; requirement: Requirement; endpoints: Endpoint[]; endpoint?: Endpoint; active?: boolean; activeModel?: string; disabled?: boolean; refresh: () => Promise<void> }>()
const emit = defineEmits<{ busy: [value: boolean]; manageModels: [] }>()
const { t } = useI18n()
const panelId = useId()
const root = ref<HTMLElement>()
const button = ref<HTMLButtonElement>()
const panel = ref<HTMLElement>()
const position = ref<Record<string, string>>({})
const open = ref(false)
const saving = ref(false)
watch(() => props.visible, value => { if (!value) open.value = false })
const model = computed(() => {
  const agent = (props.endpoint?.provider ?? 'codex') as keyof ModelPreferences
  const id = props.requirement.models?.[agent] ?? props.project.models?.[agent] ?? ''
  return props.catalog?.models.find(model => model.provider === agent && model.id === id)?.name || id
})
function close() { if (!saving.value) { open.value = false; button.value?.focus() } }
function place() {
  if (!open.value || !button.value) return
  const anchor = button.value.getBoundingClientRect()
  const width = Math.min(420, window.innerWidth - 32)
  const top = window.innerHeight - anchor.bottom > 260 ? anchor.bottom + 8 : 16
  position.value = { top: `${top}px`, left: `${Math.max(16, Math.min(anchor.right - width, window.innerWidth - width - 16))}px`, width: `${width}px`, maxHeight: `${window.innerHeight - top - 16}px` }
}
function toggle() { open.value = !open.value; if (open.value) void nextTick(place) }
function outside(event: PointerEvent) { if (event.target instanceof Node && !root.value?.contains(event.target) && !panel.value?.contains(event.target) && !saving.value) open.value = false }
onMounted(() => { document.addEventListener('pointerdown', outside); window.addEventListener('resize', place) })
onBeforeUnmount(() => { document.removeEventListener('pointerdown', outside); window.removeEventListener('resize', place) })
</script>

<template>
  <div ref="root" class="requirement-models" @keydown.esc.stop.prevent="close">
    <button ref="button" type="button" class="button model-trigger" :aria-label="t('models.change')" :aria-expanded="open" :aria-controls="panelId" :disabled="disabled || saving" @click="toggle"><Cpu :size="14" aria-hidden="true" /><span>{{ model || t('models.title') }}</span><ChevronDown :size="12" aria-hidden="true" /></button>
    <Teleport to="body"><div v-if="open" :id="panelId" ref="panel" class="requirement-model-panel" :style="position" role="group" :aria-label="t('models.requirementTitle')" @keydown.esc.stop.prevent="close">
      <ModelSettings :catalog="catalog" @manage-models="open = false; emit('manageModels')" :project="project" :requirement="requirement" :endpoints="endpoints" :disabled="disabled" :refresh="refresh" @busy="saving = $event; emit('busy', $event)" />
      <p v-if="active" class="model-active">{{ t('models.active', { model: activeModel || t('models.agentDefault') }) }}</p>
    </div></Teleport>
  </div>
</template>

<style scoped>
.requirement-models { position: relative; flex-shrink: 0; }
.model-trigger { padding: 7px 9px; font-size: 11px; font-weight: 500; }
.model-trigger span { max-width: 190px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.requirement-model-panel { position: fixed; z-index: 80; overflow-y: auto; padding: 20px; border: 1px solid var(--border-strong); border-radius: 12px; background: var(--surface-raised); box-shadow: 0 12px 36px rgb(0 0 0 / 18%); }
.model-active { margin: 16px 0 0; color: var(--muted); font-size: 12px; line-height: 1.7; }
</style>
