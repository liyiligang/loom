<script setup lang="ts">
import { computed, ref, useId, watch } from 'vue'
import { Check, Cpu, Plus, RefreshCw, Trash2 } from '@lucide/vue'
import { request } from '../api'
import { useI18n } from '../i18n'
import type { ConfiguredModel, ModelCatalog } from '../types'

const props = defineProps<{ catalog?: ModelCatalog; disabled?: boolean; refresh: () => Promise<void> }>()
const { t, errorMessage } = useI18n()
const id = useId()
const rows = ref<(ConfiguredModel & { key: string })[]>([])
const revision = ref(0)
const dirty = ref(false)
const saving = ref(false)
const saved = ref(false)
const error = ref<unknown>()
function reset() {
  rows.value = (props.catalog?.models ?? []).map(value => ({ ...value, key: crypto.randomUUID() }))
  revision.value = props.catalog?.revision ?? 0
  dirty.value = false; saved.value = false; error.value = undefined
}
watch(() => JSON.stringify(props.catalog), () => { if (!dirty.value && !saving.value) reset() }, { immediate: true })
function edit() { dirty.value = true; saved.value = false; error.value = undefined }
function add() { rows.value.push({ key: crypto.randomUUID(), provider: 'codex', name: '', id: '' }); edit() }
function remove(key: string) { rows.value = rows.value.filter(value => value.key !== key); edit() }
const valid = computed(() => {
  const keys = new Set<string>()
  return rows.value.length <= 200 && rows.value.every(value => {
    const key = `${value.provider}:${value.id.trim()}`
    if (keys.has(key)) return false
    keys.add(key)
    return Boolean(value.name.trim()) && Boolean(value.id.trim()) && new TextEncoder().encode(value.id.trim()).length <= 120 && !/[\s\u0000-\u001f\u007f]/u.test(value.id.trim())
  })
})
async function reload() {
  try { await props.refresh(); reset() } catch (cause) { error.value = cause }
}
async function save() {
  if (saving.value || props.disabled || !valid.value) return
  saving.value = true; error.value = undefined
  try {
    const result = await request<ModelCatalog>('/settings/models', 'PUT', { revision: revision.value, models: rows.value.map(({ provider, id, name }) => ({ provider, id, name })) })
    rows.value = result.models.map(value => ({ ...value, key: crypto.randomUUID() }))
    revision.value = result.revision; dirty.value = false
    await props.refresh()
    saved.value = true
  } catch (cause) { error.value = cause }
  finally { saving.value = false }
}
</script>

<template>
  <section class="model-catalog-settings" :aria-label="t('models.title')">
    <header><div><h2><Cpu :size="18" aria-hidden="true" />{{ t('models.title') }}</h2><p>{{ t('modelCatalog.hint') }}</p></div><button type="button" class="icon-button" :aria-label="t('modelCatalog.reload')" :disabled="disabled || saving" @click="reload"><RefreshCw :size="15" /></button></header>
    <form :aria-busy="saving" @submit.prevent="save" @input="edit" @change="edit">
      <p v-if="!rows.length" class="model-catalog-empty">{{ t('modelCatalog.empty') }}</p>
      <fieldset v-for="(row, index) in rows" :key="row.key" :disabled="disabled || saving">
        <legend>{{ t('modelCatalog.item', { index: index + 1 }) }}</legend>
        <div class="model-catalog-row-heading"><label :for="`${id}-${row.key}-provider`">{{ t('modelCatalog.agent') }}</label><button type="button" class="icon-button" :aria-label="t('modelCatalog.remove', { model: row.name || t('modelCatalog.unnamed') })" @click="remove(row.key)"><Trash2 :size="15" /></button></div>
        <select :id="`${id}-${row.key}-provider`" v-model="row.provider" class="text-field"><option value="codex">{{ t('agents.codex') }}</option><option value="claude">{{ t('agents.claude') }}</option></select>
        <div class="model-catalog-fields"><div><label :for="`${id}-${row.key}-name`">{{ t('modelCatalog.name') }}</label><input :id="`${id}-${row.key}-name`" v-model="row.name" class="text-field" maxlength="120" :placeholder="t('modelCatalog.namePlaceholder')" /></div><div><label :for="`${id}-${row.key}-id`">{{ t('models.identifier') }}</label><input :id="`${id}-${row.key}-id`" v-model="row.id" class="text-field mono" maxlength="120" autocomplete="off" spellcheck="false" :placeholder="t('models.identifierPlaceholder')" /></div></div>
      </fieldset>
      <button type="button" class="button" :disabled="disabled || saving || rows.length >= 200" @click="add"><Plus :size="15" />{{ t('modelCatalog.add') }}</button>
      <p class="model-catalog-note">{{ t('modelCatalog.identifierHint') }}</p>
      <p v-if="rows.length && !valid" class="model-catalog-note">{{ t('modelCatalog.validation') }}</p>
      <div class="model-catalog-actions"><button type="submit" class="button primary" :disabled="disabled || saving || !valid || !dirty">{{ t(saving ? 'models.saving' : 'navigation.save') }}</button><span v-if="saved" role="status"><Check :size="14" />{{ t('models.saved') }}</span></div>
      <p v-if="error" class="form-error" role="alert">{{ errorMessage(error, 'errors.modelUpdate') }}</p>
    </form>
  </section>
</template>

<style scoped>
.model-catalog-settings { max-width: 700px; }
.model-catalog-settings header { display: flex; align-items: flex-start; justify-content: space-between; gap: 14px; margin-bottom: 24px; }
.model-catalog-settings h2 { display: flex; align-items: center; gap: 8px; margin: 0 0 10px; font-size: 17px; }
.model-catalog-settings header p, .model-catalog-note, .model-catalog-empty { color: var(--muted); font-size: 12px; line-height: 1.8; }
.model-catalog-settings form { display: flex; flex-direction: column; align-items: flex-start; gap: 16px; }
.model-catalog-settings fieldset { width: 100%; min-width: 0; display: flex; flex-direction: column; gap: 10px; padding: 16px; border: 1px solid var(--border); border-radius: 10px; }
.model-catalog-settings legend { font-size: 12px; color: var(--muted); padding: 0 6px; }
.model-catalog-row-heading { display: flex; justify-content: space-between; align-items: center; }
.model-catalog-settings label { display: block; font-size: 12px; color: var(--muted); }
.model-catalog-fields { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; }
.model-catalog-fields label { margin-bottom: 8px; }
.model-catalog-actions, .model-catalog-actions span { display: flex; align-items: center; gap: 8px; }
.model-catalog-actions span { color: var(--muted); font-size: 12px; }
.model-catalog-note { margin: 0; }
@media (max-width: 620px) { .model-catalog-fields { grid-template-columns: 1fr; } }
</style>
