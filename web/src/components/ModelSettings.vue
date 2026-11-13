<script setup lang="ts">
import { computed, reactive, ref, useId, watch } from 'vue'
import { Check, Cpu } from '@lucide/vue'
import { request } from '../api'
import { useI18n } from '../i18n'
import type { ConfiguredModel, ModelCatalog, Endpoint, ModelPreferences, Project, Requirement } from '../types'

const props = defineProps<{ catalog?: ModelCatalog; project: Project; requirement?: Requirement; endpoints: Endpoint[]; disabled?: boolean; refresh: () => Promise<void> }>()
const emit = defineEmits<{ busy: [value: boolean]; manageModels: [] }>()
const { t, errorMessage } = useI18n()
type Agent = 'codex' | 'claude'
const id = useId()
const selections = reactive<Partial<Record<Agent, string>>>({})
const saving = ref(false)
const saved = ref(false)
const error = ref<unknown>()
const endpoints = computed(() => props.endpoints.filter(value => props.project.local ? value.local : value.id === props.project.endpointId))
const agents = computed<Agent[]>(() => props.project.local ? ['codex', 'claude'] : endpoints.value.some(value => value.provider === 'claude') ? ['claude'] : ['codex'])
const preferences = computed(() => props.requirement ? props.requirement.models ?? {} : props.project.models ?? {})
function reset() {
  for (const agent of agents.value) {
    const value = preferences.value[agent]
    selections[agent] = value === undefined ? (props.requirement ? 'inherit' : 'default') : value === '' ? 'default' : `model:${value}`
  }
}
reset()
watch(() => JSON.stringify(preferences.value), () => { if (!saving.value) reset() })
function name(agent: Agent) { return agent === 'claude' ? 'Claude Code' : 'Codex' }
function value(agent: Agent) {
  const selected = selections[agent] ?? (props.requirement ? 'inherit' : 'default')
  if (selected === 'inherit') return props.project.models?.[agent] ?? ''
  return selected.startsWith('model:') ? selected.slice(6) : ''
}
function effective(agent: Agent) {
  const id = value(agent)
  return props.catalog?.models.find(model => model.provider === agent && model.id === id)?.name || id || t('models.agentDefault')
}
function options(agent: Agent) {
  const values: (ConfiguredModel & { unlisted?: boolean })[] = (props.catalog?.models ?? []).filter(model => model.provider === agent)
  const selected = selections[agent]
  if (selected?.startsWith('model:') && !values.some(item => item.id === selected.slice(6))) values.unshift({ provider: agent, id: selected.slice(6), name: selected.slice(6), unlisted: true })
  return values
}
function changed() { saved.value = false; error.value = undefined }
async function save() {
  if (saving.value || props.disabled) return
  saving.value = true; error.value = undefined; saved.value = false; emit('busy', true)
  const models: ModelPreferences = { ...preferences.value }
  for (const agent of agents.value) {
    if (selections[agent] === 'inherit') delete models[agent]
    else models[agent] = value(agent)
  }
  try {
    await request(props.requirement ? `/requirements/${props.requirement.id}/models` : `/projects/${props.project.id}/models`, 'PATCH', { models })
    await props.refresh()
    saved.value = true
  } catch (cause) { error.value = cause }
  finally { saving.value = false; emit('busy', false) }
}
</script>

<template>
  <form class="model-settings" :aria-label="t(requirement ? 'models.requirementTitle' : 'models.projectTitle')" :aria-busy="saving" @submit.prevent="save" @change="changed">
    <header class="model-settings-heading"><div><h2><Cpu :size="17" aria-hidden="true" />{{ t(requirement ? 'models.requirementTitle' : 'models.projectTitle') }}</h2><p>{{ t(requirement ? 'models.requirementHint' : 'models.projectHint') }}</p></div></header>
    <fieldset v-for="agent in agents" :key="agent" :disabled="disabled || saving">
      <legend>{{ name(agent) }}</legend>
      <label :for="`${id}-${agent}`">{{ t('session.model') }}</label>
      <select :id="`${id}-${agent}`" v-model="selections[agent]" class="text-field">
        <option v-if="requirement" value="inherit">{{ t('models.inherit', { model: project.models?.[agent] || t('models.agentDefault') }) }}</option>
        <option value="default">{{ t('models.agentDefault') }}</option>
        <option v-for="model in options(agent)" :key="model.id" :value="`model:${model.id}`">{{ model.unlisted ? t('models.unlisted', { model: model.id }) : model.name }}{{ !model.unlisted && model.name !== model.id ? ` · ${model.id}` : '' }}</option>
      </select>
      <p class="model-effective">{{ t('models.effective', { model: effective(agent) }) }}</p>
      <p v-if="!catalog?.models.some(model => model.provider === agent)" class="model-note">{{ t('modelCatalog.noAgentModels') }}</p>
    </fieldset>
    <button type="button" class="text-button" :disabled="saving" @click="emit('manageModels')">{{ t('modelCatalog.manage') }}</button>
    <p class="model-note">{{ t('models.nextTurn') }}</p>
    <div class="model-settings-actions"><button class="button primary" :disabled="disabled || saving">{{ t(saving ? 'models.saving' : 'navigation.save') }}</button><span v-if="saved" class="model-saved" role="status"><Check :size="14" aria-hidden="true" />{{ t('models.saved') }}</span></div>
    <p v-if="error" class="form-error" role="alert">{{ errorMessage(error, 'errors.modelUpdate') }}</p>
  </form>
</template>

<style scoped>
.model-settings { display: flex; flex-direction: column; gap: 18px; max-width: 620px; }
.model-settings-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 14px; }
.model-settings h2 { display: flex; align-items: center; gap: 8px; margin: 0 0 8px; font-size: 15px; }
.model-settings-heading p, .model-note { color: var(--muted); font-size: 12px; line-height: 1.7; margin: 0; }
.model-settings fieldset { display: flex; flex-direction: column; gap: 10px; min-width: 0; padding: 16px; border: 1px solid var(--border); border-radius: 10px; }
.model-settings legend { font-weight: 600; font-size: 13px; padding: 0 6px; }
.model-settings label { font-size: 12px; color: var(--muted); }
.model-effective { font-size: 12px; color: var(--text); overflow-wrap: anywhere; margin: 0; }
.model-settings-actions, .model-saved { display: flex; align-items: center; gap: 8px; }
.model-saved { color: var(--muted); font-size: 12px; }
</style>
