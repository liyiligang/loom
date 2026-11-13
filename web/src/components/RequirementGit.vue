<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { GitBranch, RefreshCw } from '@lucide/vue'
import { request } from '../api'
import { useI18n } from '../i18n'
import type { GitRepository, Project, Requirement } from '../types'

const props = defineProps<{ requirement: Requirement; project?: Project; requirements: Requirement[]; busy: boolean; refresh: () => Promise<void> }>()
const emit = defineEmits<{ busy: [value: boolean] }>()
const { t, errorMessage, message } = useI18n()
const repository = ref<GitRepository>()
const loading = ref(false)
const saving = ref(false)
const loadError = ref<unknown>()
const saveError = ref<unknown>()
const prefix = ref(props.requirement.branchGroup?.prefix ?? '')
const baseRef = ref(props.requirement.branchGroup?.baseRef ?? 'main')
const memberName = ref('')
const sourceBranchId = ref('')
const materialBranchId = ref(props.requirement.branchGroup?.materialBranchId ?? '')
const group = computed(() => props.requirement.branchGroup)
const members = computed(() => group.value?.members ?? [])
const blocked = computed(() => props.busy || saving.value || loading.value)
const scopeId = `requirement-${props.requirement.id}-git`
let revision = 0
const availableBranches = computed(() => repository.value?.branches.filter(value =>
  value.name.startsWith(`${group.value?.prefix}/`) && value.path !== repository.value?.root &&
  !members.value.some(member => member.workspace.branch === value.name) &&
  !props.requirements.some(requirement => requirement.id !== props.requirement.id && requirement.branchGroup?.members.some(member => member.workspace.commonDir === repository.value?.commonDir && member.workspace.branch === value.name))) ?? [])
const canSaveGroup = computed(() => !blocked.value && Boolean(repository.value) && Boolean(prefix.value.trim()) &&
  (!group.value || prefix.value.trim().replace(/\/+$/, '') !== group.value.prefix || baseRef.value.trim() !== group.value.baseRef || !group.value.explicitSelection))
const canAdd = computed(() => !blocked.value && Boolean(repository.value && group.value && memberName.value.trim()))
const canSetMaterials = computed(() => !blocked.value && materialBranchId.value !== (group.value?.materialBranchId ?? ''))

async function load() {
  const version = ++revision
  repository.value = undefined
  loadError.value = undefined
  loading.value = Boolean(props.project?.local)
  if (!props.project?.local) return
  try {
    const value = await request<GitRepository>(`/projects/${props.project.id}/git`)
    if (version === revision) repository.value = value
  } catch (cause) { if (version === revision) loadError.value = cause }
  finally { if (version === revision) loading.value = false }
}
async function mutate(work: () => Promise<unknown>) {
  saving.value = true
  emit('busy', true)
  saveError.value = undefined
  try { await work(); await props.refresh(); return true }
  catch (cause) { saveError.value = cause; return false }
  finally { saving.value = false; emit('busy', false) }
}
async function saveGroup() {
  if (!canSaveGroup.value) return
  const base = baseRef.value.trim()
  const saved = await mutate(() => request(`/requirements/${props.requirement.id}/branch-group`, 'PUT', {
    prefix: prefix.value.trim(), ...(base !== (group.value?.baseRef ?? '') ? { baseRef: base } : {}),
  }))
  if (saved) { prefix.value = group.value?.prefix ?? prefix.value; baseRef.value = group.value?.baseRef ?? base }
}
async function addMember() {
  if (!canAdd.value) return
  if (await mutate(() => request(`/requirements/${props.requirement.id}/branches`, 'POST', { name: memberName.value.trim(), sourceBranchId: sourceBranchId.value }))) {
    memberName.value = ''
    await load()
  }
}
async function retry(id: string) {
  await mutate(() => request(`/requirements/${props.requirement.id}/branches/${id}/retry`, 'POST'))
}
async function setMaterials() {
  if (!canSetMaterials.value) return
  await mutate(() => request(`/requirements/${props.requirement.id}/branch-group/materials`, 'PUT', { branchId: materialBranchId.value }))
}
watch(() => props.project?.id, () => { void load() }, { immediate: true })
watch(() => group.value?.prefix, (value, previous) => { if (!saving.value && prefix.value === (previous ?? '')) prefix.value = value ?? '' })
watch(() => group.value?.baseRef, (value, previous) => { if (!saving.value && baseRef.value === (previous ?? 'main')) baseRef.value = value ?? 'main' })
watch(() => group.value?.materialBranchId, (value, previous) => { if (!saving.value && materialBranchId.value === (previous ?? '')) materialBranchId.value = value ?? '' })
onBeforeUnmount(() => { ++revision })
</script>

<template>
  <section class="requirement-git" :aria-label="t('branchGroup.title')">
    <div class="form-heading"><strong>{{ t('branchGroup.title') }}</strong><button v-if="project?.local" type="button" class="icon-button" :aria-label="t('worktree.refresh')" :disabled="blocked" @click="load"><RefreshCw :size="15" /></button></div>
    <p v-if="!project?.local" class="subtle">{{ t('branchGroup.localOnly') }}</p>
    <p v-else-if="loading" class="subtle" role="status">{{ t('worktree.loading') }}</p>
    <p v-if="loadError" class="form-error" role="alert">{{ errorMessage(loadError, 'worktree.loadError') }}</p>
    <template v-if="project?.local && (repository || group)">
      <form class="settings-form" novalidate @submit.prevent="saveGroup">
        <label :for="`${scopeId}-prefix`">{{ t('branchGroup.prefix') }}</label>
        <input :id="`${scopeId}-prefix`" v-model="prefix" class="text-field mono" maxlength="201" :disabled="blocked" :placeholder="t('branchGroup.prefixPlaceholder')" />
        <p class="subtle">{{ t('branchGroup.prefixHint') }}</p>
        <label :for="`${scopeId}-base`">{{ t('branchGroup.base') }}<small>{{ t('common.optional') }}</small></label>
        <input :id="`${scopeId}-base`" v-model="baseRef" class="text-field mono" maxlength="512" :disabled="blocked" :placeholder="t('branchGroup.basePlaceholder')" />
        <button class="button primary" :disabled="!canSaveGroup">{{ t(saving ? 'worktree.saving' : group ? 'navigation.save' : 'branchGroup.bind') }}</button>
      </form>
      <div v-if="group" class="branch-group-members">
        <div class="form-heading"><strong>{{ t('branchGroup.members') }}</strong></div>
        <p v-if="!members.length" class="subtle">{{ t('branchGroup.empty') }}</p>
        <article v-for="member in members" :key="member.id" class="branch-group-member">
          <div class="branch-member-heading"><GitBranch :size="15" aria-hidden="true" /><code :title="member.workspace.directory">{{ member.workspace.branch }}</code><small>{{ t(member.workspace.state === 'ready' ? 'worktree.ready' : member.workspace.state === 'error' ? 'worktree.failed' : 'worktree.preparing') }}</small></div>
          <p class="subtle">{{ t('branchGroup.from', { base: member.workspace.baseRef }) }}</p>
          <p v-if="member.workspace.error" class="form-error" role="alert">{{ message(member.workspace.error) }}</p>
          <button v-if="member.workspace.state === 'error'" class="text-button" :disabled="blocked" @click="retry(member.id)">{{ t('worktree.retry') }}</button>
        </article>
        <form class="settings-form branch-group-add" novalidate @submit.prevent="addMember">
          <label :for="`${scopeId}-member`">{{ t('branchGroup.memberName') }}</label>
          <input :id="`${scopeId}-member`" v-model="memberName" class="text-field mono" :list="`${scopeId}-branches`" maxlength="240" :disabled="blocked" :placeholder="t('branchGroup.memberPlaceholder')" />
          <datalist :id="`${scopeId}-branches`"><option v-for="value in availableBranches" :key="value.name" :value="value.name" /></datalist>
          <p class="subtle">{{ t('branchGroup.memberHint', { prefix: `${group.prefix}/` }) }}</p>
          <label :for="`${scopeId}-source`">{{ t('branchGroup.source') }}</label>
          <select :id="`${scopeId}-source`" v-model="sourceBranchId" class="text-field" :disabled="blocked"><option value="">{{ t('branchGroup.groupBase', { base: group.baseRef }) }}</option><option v-for="member in members" :key="member.id" :value="member.id" :disabled="member.workspace.state !== 'ready'">{{ member.workspace.branch }}</option></select>
          <button class="button" :disabled="!canAdd">{{ t('branchGroup.add') }}</button>
        </form>
        <form class="settings-form branch-group-materials" novalidate @submit.prevent="setMaterials">
          <label :for="`${scopeId}-materials`">{{ t('branchGroup.materialSource') }}</label>
          <select :id="`${scopeId}-materials`" v-model="materialBranchId" class="text-field" :disabled="blocked"><option value="">{{ t('branchGroup.projectDirectory') }}</option><option v-for="member in members" :key="member.id" :value="member.id" :disabled="member.workspace.state !== 'ready'">{{ member.workspace.branch }}</option></select>
          <p class="subtle">{{ t('branchGroup.materialHint') }}</p>
          <button class="button" :disabled="!canSetMaterials">{{ t('navigation.save') }}</button>
        </form>
      </div>
    </template>
    <p v-if="saveError" class="form-error" role="alert">{{ errorMessage(saveError, 'errors.action') }}</p>
  </section>
</template>
