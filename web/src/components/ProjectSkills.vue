<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { BookOpen, Copy, Pencil, Plus, RefreshCw, Search, Trash2 } from '@lucide/vue'
import { request } from '../api'
import { useI18n } from '../i18n'
import type { Project, ProjectSkill, ProjectSkillCatalog, ProjectSkillDetail, Session } from '../types'

const props = defineProps<{ project: Project; session?: Session }>()
const emit = defineEmits<{ open: [skill: ProjectSkill] }>()
const { t, message, errorMessage, number } = useI18n()
const catalog = ref<ProjectSkillCatalog>()
const loading = ref(false)
const error = ref<unknown>()
const search = ref('')
const editor = ref<'new' | 'edit'>()
const selected = ref<ProjectSkillDetail>()
const skillName = ref('')
const content = ref('')
const editorScope = ref('')
const formLoading = ref(false)
const saving = ref(false)
const formError = ref<unknown>()
const success = ref('')
const copying = ref(false)
const removal = ref<ProjectSkill>()
const deleting = ref(false)
const deletionError = ref<unknown>()
const deleteDialog = ref<HTMLDialogElement>()
const dialogId = `skills-delete-${props.project.id}-${props.session?.id ?? 'root'}`
const mutating = computed(() => saving.value || deleting.value)
let editorRevision = 0
let updateSkillName: typeof import('../composables/skillSource').withSkillName | undefined
let alive = true
let revision = 0
const scope = computed(() => `${props.project.id}:${props.session?.id ?? ''}`)
const query = computed(() => props.session ? `?sessionId=${encodeURIComponent(props.session.id)}` : '')
const route = computed(() => `/projects/${encodeURIComponent(props.project.id)}/skills`)
const filtered = computed(() => {
  const text = search.value.trim().toLowerCase()
  return catalog.value?.skills.filter(skill => !text || `${skill.name} ${skill.description}`.toLowerCase().includes(text)) ?? []
})
const duplicateNames = computed(() => {
  const counts = new Map<string, number>()
  for (const skill of catalog.value?.skills ?? []) counts.set(skill.name, (counts.get(skill.name) ?? 0) + 1)
  return counts
})
function source(skill: ProjectSkill) {
  const path = skill.paths[0] ?? ''
  return path.startsWith('.claude/') ? 'Claude' : path.startsWith('.codex/') ? 'Codex' : t('skills.shared')
}
async function load() {
  const version = ++revision
  const currentScope = scope.value
  loading.value = true
  error.value = ''
  try {
    const value = await request<ProjectSkillCatalog>(`${route.value}${query.value}`)
    if (version === revision && scope.value === currentScope) catalog.value = value
  } catch (cause) {
    if (version === revision) error.value = cause
  } finally { if (version === revision) loading.value = false }
}
function template(name: string) {
  return `---\nname: new-skill\ndescription: Describe when this skill should be used.\nmetadata:\n  display_name: ${JSON.stringify(name)}\n---\n\n# Instructions\n\nDescribe the workflow, inputs, and expected results.\n`
}
function closeEditor() {
  if (mutating.value) return
  ++editorRevision; editor.value = undefined; selected.value = undefined; formError.value = undefined
  content.value = ''; formLoading.value = false; copying.value = false
}
async function create() {
  if (mutating.value || !catalog.value) return
  closeEditor(); editor.value = 'new'; skillName.value = ''; editorScope.value = catalog.value.scopeKey
  const version = ++editorRevision
  const currentScope = scope.value
  formLoading.value = true; success.value = ''
  try {
    const naming = await import('../composables/skillSource')
    if (!alive || version !== editorRevision || scope.value !== currentScope) return
    updateSkillName = naming.withSkillName; content.value = template(skillName.value)
  } catch (cause) { if (version === editorRevision) formError.value = cause }
  finally { if (version === editorRevision) formLoading.value = false }
}
function synchronizeName() {
  if (editor.value && !formLoading.value && updateSkillName) content.value = updateSkillName(content.value, skillName.value)
}
watch(skillName, synchronizeName)
async function edit(skill: ProjectSkill, copy = false) {
  if (mutating.value || !catalog.value) return
  closeEditor()
  const version = ++editorRevision
  const currentScope = scope.value
  editor.value = copy ? 'new' : 'edit'; copying.value = copy; formLoading.value = true; success.value = ''
  try {
    const [value, naming] = await Promise.all([
      request<ProjectSkillDetail>(`${route.value}/${encodeURIComponent(skill.id)}${query.value}`),
      import('../composables/skillSource'),
    ])
    if (!alive || version !== editorRevision || scope.value !== currentScope) return
    selected.value = copy ? undefined : value; content.value = value.source; editorScope.value = value.scopeKey
    updateSkillName = naming.withSkillName
    skillName.value = value.name
    if (copy) {
      const suffix = ` ${t('skills.copySuffix')}`
      skillName.value = Array.from(value.name).slice(0, 128 - Array.from(suffix).length).join('') + suffix
      content.value = updateSkillName(content.value, skillName.value)
    }
  } catch (cause) { if (version === editorRevision) formError.value = cause }
  finally { if (version === editorRevision) formLoading.value = false }
}
async function save() {
  if (mutating.value || formLoading.value || !editor.value || editor.value === 'edit' && !selected.value) return
  saving.value = true; formError.value = undefined
  const currentScope = scope.value
  synchronizeName()
  const body = editor.value === 'new' ? { name: skillName.value, content: content.value, scopeKey: editorScope.value } : { name: skillName.value, content: content.value, expectedHash: selected.value!.hash, scopeKey: editorScope.value }
  const path = `${route.value}${editor.value === 'edit' ? `/${encodeURIComponent(selected.value!.id)}` : ''}${query.value}`
  try {
    await request<ProjectSkillDetail>(path, editor.value === 'new' ? 'POST' : 'PUT', body)
    if (!alive || scope.value !== currentScope) return
    saving.value = false; closeEditor(); await load(); success.value = t('skills.saved')
  } catch (cause) { if (alive && scope.value === currentScope) formError.value = cause }
  finally { saving.value = false }
}
async function confirmDelete(skill: ProjectSkill) {
  if (mutating.value || !skill.deletable) return
  removal.value = skill; deletionError.value = undefined
  await nextTick(); deleteDialog.value?.showModal()
}
function cancelDelete() {
  if (deleting.value) return
  deleteDialog.value?.close(); removal.value = undefined; deletionError.value = undefined
}
async function remove() {
  if (!removal.value || !catalog.value || mutating.value) return
  const skill = removal.value
  const currentScope = scope.value
  deleting.value = true; deletionError.value = undefined
  try {
    await request(`${route.value}/${encodeURIComponent(skill.id)}${query.value}`, 'DELETE', { scopeKey: catalog.value.scopeKey, expectedRevision: skill.revision, confirmed: true })
    if (!alive || scope.value !== currentScope) return
    deleting.value = false; cancelDelete()
    if (selected.value?.id === skill.id) closeEditor()
    await load(); success.value = t('skills.deleted')
  } catch (cause) { if (alive && scope.value === currentScope) deletionError.value = cause }
  finally { deleting.value = false }
}
watch(scope, () => { closeEditor(); cancelDelete(); catalog.value = undefined; search.value = ''; success.value = ''; void load() }, { immediate: true })
onBeforeUnmount(() => { alive = false; ++revision; ++editorRevision; deleteDialog.value?.close() })
</script>

<template>
  <section class="project-skills" :aria-label="t('skills.projectList', { name: project.name })">
    <code class="skills-directory" :title="catalog?.directory ?? session?.cwd ?? project.cwd">{{ catalog?.directory ?? session?.cwd ?? project.cwd }}</code>
    <div class="skills-heading"><span>{{ t('skills.title') }}<small v-if="catalog">{{ number(catalog.skills.length) }}</small></span><div class="skill-actions"><button class="button" :disabled="loading || mutating || !catalog || Boolean(editor)" @click="create"><Plus :size="14" />{{ t('skills.add') }}</button><button class="tree-action" :disabled="loading || mutating" :aria-label="t('skills.refresh')" @click="load"><RefreshCw :size="13" :class="{ 'skills-refreshing': loading }" /></button></div></div>
    <p v-if="success" class="subtle" role="status">{{ success }}</p>
    <form v-if="editor" class="skill-editor settings-form" @submit.prevent="save">
      <h2>{{ t(editor === 'new' ? 'skills.add' : 'skills.edit') }}</h2>
      <p v-if="formLoading" class="subtle" role="status">{{ t('skills.loading') }}</p>
      <template v-else>
        <label class="form-field">{{ t('skills.name') }}<input v-model.trim="skillName" class="text-field" :placeholder="t('skills.namePlaceholder')" maxlength="128" required :disabled="saving" /><small>{{ t('skills.nameHint') }}</small></label>
        <p v-if="copying" class="subtle">{{ t('skills.copyHint') }}</p>
        <label class="form-field">{{ t('skills.source') }}<textarea v-model="content" class="text-field skill-source-editor" rows="18" required :disabled="saving" spellcheck="false" @blur="synchronizeName"></textarea><small>{{ t(editor === 'new' ? 'skills.newSourceHint' : 'skills.sourceHint') }}</small></label>
        <p v-if="formError" class="form-error" role="alert">{{ errorMessage(formError, 'skills.saveError') }}</p>
        <div class="form-actions"><button class="button primary" :disabled="saving || editor === 'edit' && !selected" type="submit">{{ t(saving ? 'skills.saving' : 'navigation.save') }}</button><button type="button" class="button" :disabled="saving" @click="closeEditor">{{ t('common.cancel') }}</button></div>
      </template>
    </form>
    <label v-if="catalog && catalog.skills.length > 5" class="skills-search"><Search :size="13" /><input v-model="search" :aria-label="t('skills.search')" :placeholder="t('skills.search')" /></label>
    <div class="skills-scroll">
      <p v-if="loading && !catalog" class="skills-empty" role="status">{{ t('skills.loading') }}</p>
      <p v-else-if="error" class="skills-error" role="alert">{{ errorMessage(error, 'skills.loadError') }}</p>
      <template v-else-if="catalog">
        <p v-if="catalog.limited || catalog.unreadable" class="skills-notice" role="status">{{ t(catalog.limited ? 'skills.limited' : 'skills.unreadable') }}</p>
        <ul v-if="filtered.length" class="skill-list">
          <li v-for="skill in filtered" :key="skill.id" class="skill-row"><button class="skill-link" :title="skill.paths.join('\n')" :disabled="mutating" @click="emit('open', skill)"><BookOpen :size="15" /><span><strong>{{ skill.name }}<small v-if="(duplicateNames.get(skill.name) ?? 0) > 1">{{ source(skill) }}</small></strong><p v-if="skill.error" class="skill-unavailable">{{ message(skill.error) }}</p><p v-else-if="skill.description">{{ skill.description }}</p></span></button><div class="skill-actions"><button v-if="!skill.editable && !skill.error" class="icon-button" :disabled="mutating || formLoading || Boolean(editor)" :aria-label="t('skills.copyNamed', { name: skill.name })" :title="t('skills.copyNamed', { name: skill.name })" @click="edit(skill, true)"><Copy :size="15" /></button><button class="icon-button" :disabled="!skill.editable || mutating || formLoading || Boolean(editor)" :aria-label="t('skills.editNamed', { name: skill.name })" :title="t(skill.editable ? 'skills.edit' : 'skills.linkedReadOnly')" @click="edit(skill)"><Pencil :size="15" /></button><button class="icon-button skill-delete-button" :disabled="!skill.deletable || mutating" :aria-label="t('skills.deleteNamed', { name: skill.name })" :title="t('skills.delete')" @click="confirmDelete(skill)"><Trash2 :size="15" /></button></div></li>
        </ul>
        <p v-else class="skills-empty">{{ t(search.trim() ? 'skills.noMatches' : 'skills.empty') }}</p>
      </template>
    </div>
    <dialog ref="deleteDialog" class="skill-delete-dialog" role="alertdialog" :aria-labelledby="`${dialogId}-title`" :aria-describedby="`${dialogId}-description`" @cancel.prevent="cancelDelete">
      <h2 :id="`${dialogId}-title`">{{ t('skills.deleteNamed', { name: removal?.name ?? '' }) }}</h2>
      <p :id="`${dialogId}-description`">{{ t('skills.deleteConfirm') }}</p>
      <p v-if="deletionError" class="form-error" role="alert">{{ errorMessage(deletionError, 'skills.deleteError') }}</p>
      <div class="form-actions"><button class="button" :disabled="deleting" autofocus @click="cancelDelete">{{ t('common.cancel') }}</button><button class="button danger" :disabled="deleting" @click="remove">{{ t(deleting ? 'skills.deleting' : 'skills.delete') }}</button></div>
    </dialog>
  </section>
</template>
