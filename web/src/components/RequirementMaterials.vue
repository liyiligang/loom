<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { Download, FileText, Pencil, RefreshCw, MessageSquare } from '@lucide/vue'
import { request } from '../api'
import { useI18n } from '../i18n'
import type { MaterialCatalog, MaterialDetail, MaterialKind, Project, Requirement, RequirementMaterial } from '../types'
import StreamingMarkdown from './StreamingMarkdown.vue'
import PrototypePreview from './PrototypePreview.vue'

const props = defineProps<{ requirement: Requirement; project?: Project; kind: MaterialKind; empty: string; refreshKey: number; writers?: { sessionId: string; directory: string }[] }>()
const emit = defineEmits<{ busy: [value: boolean]; generate: [] }>()
const { t, errorMessage } = useI18n()
const catalog = ref<MaterialCatalog>()
const detail = ref<MaterialDetail>()
const loading = ref(false)
const saving = ref(false)
const error = ref<unknown>()
const editing = ref(false)
const content = ref('')
const related = ref<string[]>([])
const success = ref('')
let revision = 0
const drafts = new Map<string, { content: string; detail: MaterialDetail }>()
const selections = new Map<string, string>()
const route = `/requirements/${encodeURIComponent(props.requirement.id)}/materials`
const binding = computed(() => `${props.requirement.workspace?.directory ?? props.project?.cwd}:${props.requirement.workspace?.branch ?? ''}:${props.requirement.workspace?.state ?? ''}`)
const files = computed(() => catalog.value?.files.filter(file => file.kind === props.kind) ?? [])
const writerBusy = computed(() => props.writers?.some(writer => writer.directory === catalog.value?.scope.directory) ?? false)
const dirty = computed(() => editing.value && content.value !== detail.value?.content)
const selectionKey = () => `${catalog.value?.scope.key}:${props.kind}`
const draftKey = () => `${selectionKey()}:${detail.value?.id}`
const otherFiles = computed(() => catalog.value?.files.filter(file => file.id !== detail.value?.id) ?? [])
const markdown = computed(() => (detail.value?.path ?? '').toLowerCase().endsWith('.md'))
const download = computed(() => detail.value ? `/api/v1${route}/${detail.value.id}/download` : '')
const changed = computed(() => Boolean(detail.value && catalog.value?.files.find(file => file.id === detail.value?.id)?.hash !== detail.value.hash))
const prototype = computed(() => props.kind === 'prototypes' && detail.value?.editable && !detail.value.truncated)
const preview = computed(() => {
  if (!detail.value || !prototype.value) return ''
  const query = new URLSearchParams({ scopeKey: detail.value.scope.key, hash: detail.value.hash })
  return `/api/v1${route}/${detail.value.id}/preview?${query}`
})

function stash() {
  if (dirty.value && detail.value) drafts.set(draftKey(), { content: content.value, detail: detail.value })
}
function reset() {
  detail.value = undefined; editing.value = false
  content.value = ''; related.value = []; success.value = ''
}
function accept(value: MaterialDetail) {
  detail.value = value; content.value = value.content
  related.value = [...value.relatedIds]; editing.value = false
}
async function select(file: RequirementMaterial) {
  if (saving.value) return
  stash()
  const version = ++revision
  reset(); loading.value = true; error.value = undefined
  selections.set(selectionKey(), file.id)
  const draft = drafts.get(`${selectionKey()}:${file.id}`)
  if (draft?.detail) {
    accept(draft.detail); content.value = draft.content; editing.value = true
    loading.value = false
    return
  }
  try {
    const value = await request<MaterialDetail>(`${route}/${file.id}`)
    if (version === revision && value.scope.key === catalog.value?.scope.key) accept(value)
  } catch (cause) { if (version === revision) error.value = cause }
  finally { if (version === revision) loading.value = false }
}
async function load(preserve = true) {
  if (!props.project?.local) return
  const version = ++revision
  loading.value = true; error.value = undefined
  try {
    const value = await request<MaterialCatalog>(route)
    if (version !== revision) return
    if (catalog.value && value.scope.key !== catalog.value.scope.key) { stash(); reset() }
    catalog.value = value
    if (preserve && (dirty.value || editing.value)) return
    const selected = selections.get(selectionKey())
    const file = files.value.find(file => file.id === (detail.value?.id ?? selected)) ?? (props.kind === 'prototypes' ? files.value[0] : undefined)
    if (file) await select(file)
    else reset()
  } catch (cause) { if (version === revision) error.value = cause }
  finally { if (version === revision) loading.value = false }
}
function cancel() {
  drafts.delete(draftKey())
  if (detail.value) accept(detail.value)
  else reset()
  void load(false)
}
async function save() {
  if (!catalog.value || !detail.value || saving.value || writerBusy.value) return
  const scope = catalog.value.scope.key
  const key = draftKey()
  saving.value = true; error.value = undefined; success.value = ''
  try {
    const value = await request<MaterialDetail>(`${route}/${detail.value.id}`, 'PUT', { kind: props.kind, name: detail.value.path.split('/').at(-1) ?? '', content: content.value, expectedHash: detail.value.hash, scopeKey: scope })
    drafts.delete(key); accept(value); selections.set(selectionKey(), value.id)
    await load(false); success.value = t('materials.saved')
  } catch (cause) { error.value = cause }
  finally { saving.value = false }
}
async function saveLinks() {
  if (!catalog.value || !detail.value || saving.value) return
  saving.value = true; error.value = undefined; success.value = ''
  try {
    await request(`${route}/${detail.value.id}/links`, 'PUT', { scopeKey: catalog.value.scope.key, relatedIds: related.value })
    detail.value.relatedIds = [...related.value]
    await load(); success.value = t('materials.linksSaved')
  } catch (cause) { error.value = cause }
  finally { saving.value = false }
}
watch(binding, () => { stash(); ++revision; reset(); catalog.value = undefined; void load(false) }, { immediate: true })
watch(() => props.kind, (kind, previous) => {
  // The previous category's draft key must be used after the prop changes.
  if (dirty.value && detail.value) drafts.set(`${catalog.value?.scope.key}:${previous}:${detail.value.id}`, { content: content.value, detail: detail.value })
  ++revision; reset(); void load(false)
})
watch(() => props.refreshKey, () => { if (!saving.value) void load() })
watch(saving, value => emit('busy', value))
onBeforeUnmount(() => { ++revision })
</script>

<template>
  <section class="requirement-materials" :aria-label="t('materials.files')" :aria-busy="loading || saving">
    <p v-if="!project?.local" class="subtle">{{ t('materials.localOnly') }}</p>
    <template v-else>
      <div class="material-toolbar"><button class="icon-button" :disabled="loading || saving" :aria-label="t('materials.refresh')" @click="load()"><RefreshCw :size="15" /></button><small v-if="catalog?.scope.branch" class="subtle">{{ catalog.scope.branch }}</small></div>
      <p v-if="error" class="form-error" role="alert">{{ errorMessage(error, 'materials.loadError') }}</p>
      <p v-if="success" class="subtle" role="status">{{ success }}</p>
      <p v-if="catalog?.limited" class="subtle" role="status">{{ t('materials.limited') }}</p>
      <p v-if="writerBusy" class="subtle" role="status">{{ t('materials.writerBusy') }}</p>
      <p v-if="loading && !catalog" class="subtle" role="status">{{ t('materials.loading') }}</p>
      <ul v-if="files.length" class="material-list">
        <li v-for="file in files" :key="file.id"><button :class="{ selected: detail?.id === file.id }" :disabled="saving" @click="select(file)"><FileText :size="15" /><span>{{ file.path.split('/').at(-1) }}</span><small v-if="drafts.has(`${selectionKey()}:${file.id}`)">{{ t('materials.draft') }}</small></button></li>
      </ul>
      <div v-else-if="catalog" class="content-empty"><FileText :size="28" aria-hidden="true" /><p>{{ empty }}</p><p class="subtle">{{ t(kind === 'prototypes' ? 'prototype.aiHint' : 'materials.aiHint') }}</p><button class="button" @click="emit('generate')"><MessageSquare :size="16" aria-hidden="true" />{{ t('materials.openConversation') }}</button></div>
      <div v-if="detail" class="material-detail">
        <form @submit.prevent="save">
          <div class="material-detail-heading"><code>{{ detail.path.split('/').at(-1) }}</code><div><a :href="download" class="icon-button" :aria-label="t('materials.download')"><Download :size="15" /></a><button v-if="!editing && detail.editable" type="button" class="icon-button" :disabled="saving || writerBusy" :aria-label="t('materials.edit')" @click="editing = true; success = ''"><Pencil :size="15" /></button></div></div>
          <p v-if="changed" class="subtle" role="status">{{ t('materials.changed') }}</p>
          <label v-if="editing" class="form-field">{{ t('materials.content') }}<textarea v-model="content" class="text-field material-editor" rows="18" :disabled="saving" spellcheck="false"></textarea></label>
          <PrototypePreview v-else-if="prototype && preview" :src="preview" :title="detail.title" />
          <StreamingMarkdown v-else-if="markdown" :text="detail?.content ?? ''" status="completed" external-links-only />
          <pre v-else-if="detail?.content" class="material-preview">{{ detail.content }}</pre>
          <p v-else-if="!detail?.editable" class="subtle">{{ t('materials.downloadOnly') }}</p>
          <p v-if="detail?.truncated" class="subtle">{{ t('materials.truncated') }}</p>
          <div v-if="editing" class="form-actions"><button class="button primary" :disabled="saving || loading || writerBusy" type="submit">{{ t(saving ? 'worktree.saving' : 'navigation.save') }}</button><button class="button" :disabled="saving" type="button" @click="cancel">{{ t('common.cancel') }}</button></div>
        </form>
        <div v-if="otherFiles.length" class="material-links"><label :for="`material-${detail.id}-links`">{{ t('materials.related') }}</label><select :id="`material-${detail.id}-links`" v-model="related" class="text-field" multiple :disabled="saving"><option v-for="file in otherFiles" :key="file.id" :value="file.id">{{ file.path.split('/').slice(-2).join('/') }}</option></select><button class="button" :disabled="saving || loading" @click="saveLinks">{{ t('materials.saveLinks') }}</button></div>
      </div>
    </template>
  </section>
</template>
