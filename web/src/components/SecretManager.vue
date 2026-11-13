<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Copy, Eye, EyeOff, KeyRound, Pencil, Plus, RefreshCw, Search, Trash2 } from '@lucide/vue'
import { request } from '../api'
import { useI18n } from '../i18n'
import type { SecretValues, StoredSecret } from '../types'

const emit = defineEmits<{ changed: [] }>()
const { t, errorMessage } = useI18n()
const records = ref<StoredSecret[]>([])
const loading = ref(false)
const error = ref<unknown>()
const search = ref('')
const visibleRecords = computed(() => records.value.filter(value => value.name.toLowerCase().includes(search.value.toLowerCase())))
const mode = ref<'new' | 'edit' | 'view'>('new')
const selected = ref<StoredSecret>()
const form = reactive({ name: '', content: '' })
const saving = ref(false)
const formError = ref<unknown>()
const showing = ref(false)
const viewLoading = ref(false)
const removal = ref(false)
const deleting = ref(false)
let revision = 0
let selectionRevision = 0
let alive = true
const mutating = computed(() => saving.value || deleting.value)

async function load() {
  const version = ++revision
  loading.value = true
  error.value = undefined
  try { const result = await request<StoredSecret[]>('/secrets'); if (version === revision) records.value = result }
  catch (cause) { if (version === revision) error.value = cause }
  finally { if (version === revision) loading.value = false }
}
function edit(record?: StoredSecret) {
  if (mutating.value) return
  ++selectionRevision
  selected.value = record
  mode.value = record ? 'edit' : 'new'
  Object.assign(form, { name: record?.name ?? '', content: '' })
  formError.value = undefined
  showing.value = false
  viewLoading.value = false
  removal.value = false
}
async function save() {
  if (mutating.value) return
  const version = selectionRevision
  const id = selected.value?.id
  saving.value = true
  formError.value = undefined
  try {
    const body: Record<string, string> = { name: form.name }
    if (form.content) body.content = form.content
    const result = await request<{ id: string }>(`/secrets${id ? `/${id}` : ''}`, id ? 'PATCH' : 'POST', body)
    if (!alive) return
    emit('changed')
    if (version === selectionRevision) {
      form.content = ''
      await load()
      selected.value = records.value.find(value => value.id === (id ?? result.id))
      form.name = selected.value?.name ?? ''
      mode.value = 'edit'
    }
  } catch (cause) { if (alive && version === selectionRevision) formError.value = cause }
  finally { saving.value = false }
}
async function view(record: StoredSecret) {
  if (mutating.value) return
  edit(record)
  mode.value = 'view'
  const version = ++selectionRevision
  viewLoading.value = true
  try {
    const result = await request<SecretValues>(`/secrets/${record.id}/reveal`, 'POST')
    if (alive && version === selectionRevision) form.content = result.content ?? result.token ?? JSON.stringify({ accessKeyId: result.accessKeyId, secretAccessKey: result.secretAccessKey }, null, 2)
  } catch (cause) { if (alive && version === selectionRevision) formError.value = cause }
  finally { if (version === selectionRevision) viewLoading.value = false }
}
async function copy() {
  try { await navigator.clipboard.writeText(form.content); if (alive) ElMessage.success(t('secrets.copied')) }
  catch { if (alive) ElMessage.error(t('secrets.copyError')) }
}
async function remove() {
  if (!selected.value || mutating.value) return
  deleting.value = true
  formError.value = undefined
  try {
    await request(`/secrets/${selected.value.id}`, 'DELETE')
    if (!alive) return
    emit('changed')
    deleting.value = false
    edit()
    await load()
  } catch (cause) { if (alive) formError.value = cause }
  finally { deleting.value = false }
}
void load()
onBeforeUnmount(() => { alive = false; ++revision; ++selectionRevision; form.content = '' })
</script>

<template>
  <div class="credential-workspace">
    <section class="credential-index" :aria-label="t('secrets.title')">
      <div class="secret-toolbar"><button class="button" :disabled="mutating" @click="edit()"><Plus :size="16" />{{ t('secrets.add') }}</button><button class="icon-button" :disabled="loading || mutating" :aria-label="t('secrets.refresh')" :title="t('secrets.refresh')" @click="load"><RefreshCw :size="16" /></button></div>
      <label v-if="records.length > 5" class="skills-search secret-search"><Search :size="14" /><input v-model="search" :aria-label="t('secrets.search')" :placeholder="t('secrets.search')" /></label>
      <p v-if="error" class="form-error" role="alert">{{ errorMessage(error, 'secrets.loadError') }}</p>
      <p v-else-if="loading && !records.length" class="subtle" role="status">{{ t('secrets.loading') }}</p>
      <div v-else class="secret-list">
        <p v-if="!visibleRecords.length" class="subtle">{{ t(records.length ? 'secrets.noMatches' : 'secrets.empty') }}</p>
        <button v-for="record in visibleRecords" :key="record.id" class="credential-link" :class="{ selected: selected?.id === record.id }" :disabled="mutating" :aria-pressed="selected?.id === record.id" @click="edit(record)"><KeyRound :size="16" aria-hidden="true" /><strong>{{ record.name }}</strong></button>
      </div>
    </section>
    <section class="credential-editor" :aria-label="t(mode === 'new' ? 'secrets.add' : mode === 'edit' ? 'secrets.edit' : 'secrets.view')" :aria-busy="mutating || viewLoading">
      <div class="credential-editor-heading"><h2>{{ selected?.name ?? t('secrets.add') }}</h2><div v-if="selected" class="secret-row-actions"><button v-if="mode === 'view'" class="icon-button" :disabled="mutating" :aria-label="t('secrets.edit')" @click="edit(selected)"><Pencil :size="16" /></button><button v-else class="icon-button" :disabled="mutating" :aria-label="t('secrets.viewNamed', { name: selected.name })" :title="t('secrets.view')" @click="view(selected)"><Eye :size="16" /></button><button class="icon-button" :disabled="mutating" :aria-label="t('secrets.deleteNamed', { name: selected.name })" :title="t('secrets.delete')" @click="removal = !removal"><Trash2 :size="16" /></button></div></div>
      <div v-if="removal && selected" class="inline-confirmation" role="group" :aria-label="t('secrets.delete')"><p>{{ t('secrets.deleteConfirm', { name: selected.name }) }}</p><div class="form-actions"><button class="button secret-delete" :disabled="mutating" @click="remove">{{ t('secrets.delete') }}</button><button class="button" :disabled="mutating" @click="removal = false">{{ t('common.cancel') }}</button></div></div>
      <form v-if="mode !== 'view'" class="settings-form" novalidate @submit.prevent="save">
        <label for="secret-name">{{ t('secrets.name') }}<small>{{ t('common.optional') }}</small></label><input id="secret-name" v-model="form.name" class="text-field" maxlength="120" :disabled="mutating" :placeholder="t('secrets.namePlaceholder')" autocomplete="off" />
        <label for="secret-content">{{ t('secrets.content') }}</label><textarea id="secret-content" v-model="form.content" class="text-field mono credential-content" :disabled="mutating" :placeholder="t(mode === 'edit' ? 'secrets.keepValue' : 'secrets.contentPlaceholder')" autocomplete="off" autocapitalize="off" :spellcheck="false"></textarea>
        <div class="form-actions"><button class="button primary" :disabled="mutating || (mode === 'new' && !form.content.trim())">{{ t(saving ? 'secrets.saving' : 'navigation.save') }}</button><button v-if="form.content" type="button" class="text-button" :disabled="mutating" @click="form.content = ''">{{ t('secrets.clear') }}</button></div>
      </form>
      <template v-else><p v-if="viewLoading" class="subtle" role="status">{{ t('secrets.loading') }}</p><template v-else-if="!formError"><div class="credential-view-actions"><span>{{ t('secrets.content') }}</span><button class="icon-button" :aria-label="t(showing ? 'secrets.hideValue' : 'secrets.showValue')" @click="showing = !showing"><component :is="showing ? EyeOff : Eye" :size="16" /></button><button class="icon-button" :aria-label="t('secrets.copyField', { field: t('secrets.content') })" @click="copy"><Copy :size="16" /></button></div><textarea v-if="showing" class="text-field mono credential-content" :value="form.content" :aria-label="t('secrets.content')" autocomplete="off" :spellcheck="false" readonly></textarea><div v-else class="credential-masked" :aria-label="t('secrets.hidden')">••••••••••••••••</div></template></template>
      <p v-if="formError" class="form-error" role="alert">{{ errorMessage(formError, 'secrets.saveError') }}</p>
    </section>
  </div>
</template>
