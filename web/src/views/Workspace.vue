<script setup lang="ts">
import { computed, h, nextTick, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useRoute, useRouter } from 'vue-router'
import { Radio, Cable, ArrowUp, Square, Menu, ChevronRight, Pencil, RefreshCw, Folder, FolderPlus, MessageSquare, Search, GitBranch, KeyRound, ArrowLeft, Settings } from '@lucide/vue'
import { request } from '../api'
import { useWorkspace } from '../composables/useWorkspace'
import { showLocalSetup } from '../composables/localSetup'
import { shouldSendTask } from '../composables/composerKeyboard'
import EntryCard from '../components/EntryCard.vue'
import LanguagePicker from '../components/LanguagePicker.vue'
import ThemePicker from '../components/ThemePicker.vue'
import LocalSetup from '../components/LocalSetup.vue'
import FolderPicker from '../components/FolderPicker.vue'
import ProjectTree from '../components/ProjectTree.vue'
import ProjectOverview from '../components/ProjectOverview.vue'
import RequirementOverview from '../components/RequirementOverview.vue'
import SecretManager from '../components/SecretManager.vue'
import ModelCatalogSettings from '../components/ModelCatalogSettings.vue'
import SkillDetail from '../components/SkillDetail.vue'
import { useI18n } from '../i18n'
import type { TranslationKey } from '../i18n/catalog'
import InteractionCard from '../components/InteractionCard.vue'
import SessionPermissions from '../components/SessionPermissions.vue'
import type { AccessMode, Endpoint, Project, Requirement, Reply, ProjectSkill, Session } from '../types'

type Page = 'conversation' | 'connections' | 'group' | 'secrets' | 'project' | 'requirement' | 'skill' | 'newSession' | 'settings'
const router = useRouter()
const route = useRoute()
const page = ref<Page>('conversation')
const secretReturnPage = ref<Page>('conversation')
const settingsReturnPage = ref<Page>('conversation')
const groupReturnPage = ref<Page>('conversation')
const skillContext = ref<{ project: Project; session?: Session; skill: ProjectSkill }>()
function pageFlag(value: Page) {
  return computed({
    get: () => page.value === value,
    set: open => {
      if (open) {
        if (value === 'settings' && page.value !== 'settings') settingsReturnPage.value = page.value
        if (value === 'secrets' && page.value !== 'secrets') secretReturnPage.value = page.value
        if (value === 'group' && page.value !== 'group') groupReturnPage.value = page.value === 'project' || page.value === 'requirement' ? page.value : 'conversation'
        page.value = value
        sidebarOpen.value = false
        folderOpen.value = false
      } else if (page.value === value) {
        page.value = value === 'settings' ? settingsReturnPage.value : value === 'secrets' ? secretReturnPage.value : value === 'group' ? groupReturnPage.value : 'conversation'
      }
    },
  })
}
function backToWorkspace() {
  if (page.value === 'settings') settingsOpen.value = false
  else if (page.value === 'secrets') secretsOpen.value = false
  else if (page.value === 'group') { groupOpen.value = false; folderOpen.value = false }
  else if (page.value === 'newSession') page.value = 'requirement'
  else if (page.value === 'skill' && skillsProject.value) page.value = 'project'
  else { page.value = 'conversation'; folderOpen.value = false }
}
function openSkill(skill: ProjectSkill) {
  if (!skillsProject.value) return
  skillContext.value = { project: skillsProject.value, session: skillsSession.value, skill }
  page.value = 'skill'
  sidebarOpen.value = false
}
const { t, message, errorMessage, stateLabel: label, sessionTitle, number } = useI18n()
const { snapshot, detail, discovery, online, loading, error, refresh, discover, selectedId } = useWorkspace()
const remoteOnboarding = ref(false)
const localSetupRequested = ref(false)
const localSetupOpen = computed(() => showLocalSetup(snapshot.value.local, snapshot.value.endpoints, localSetupRequested.value, remoteOnboarding.value))
const folderOpen = ref(false)
const skillsProjectId = ref('')
const skillsSessionId = ref('')
const requirementViewId = ref('')
const requirementOverview = ref<InstanceType<typeof RequirementOverview>>()
const viewedRequirement = computed(() => snapshot.value.requirements.find(requirement => requirement.id === requirementViewId.value))
const viewedWorkflow = computed(() => snapshot.value.workflows?.find(value => value.requirementId === viewedRequirement.value?.id))
const workflowSessionState = computed(() => snapshot.value.sessions.find(value => value.id === viewedWorkflow.value?.sessionId)?.state)
const requirementProject = computed(() => snapshot.value.projects.find(project => project.id === viewedRequirement.value?.projectId))
const sessionForm = reactive({ title: '', endpointId: '', branchId: '' })
const sessionCreateError = ref<unknown>()
const sessionAgents = computed(() => snapshot.value.endpoints.filter(value => requirementProject.value?.local ? value.local : value.id === requirementProject.value?.endpointId))
const sessionCreationEndpoint = computed(() => sessionAgents.value.find(value => value.id === sessionForm.endpointId))
const sessionBranches = computed(() => viewedRequirement.value?.branchGroup?.members ?? [])
const sessionBranch = computed(() => sessionBranches.value.find(value => value.id === sessionForm.branchId))
const canCreateSession = computed(() => online.value && !busy.value && Boolean(viewedRequirement.value) && sessionCreationEndpoint.value?.state === 'ready' && (!viewedRequirement.value?.branchGroup || sessionBranch.value?.workspace.state === 'ready'))
function defaultSessionAgent() { return sessionAgents.value.find(value => value.id === 'local-codex' && value.state === 'ready')?.id ?? sessionAgents.value.find(value => value.state === 'ready')?.id ?? sessionAgents.value[0]?.id ?? '' }
watch(sessionAgents, values => { if (!values.some(value => value.id === sessionForm.endpointId)) sessionForm.endpointId = defaultSessionAgent() })
const skillsProject = computed(() => snapshot.value.projects.find(project => project.id === skillsProjectId.value))
const skillsSession = computed(() => snapshot.value.sessions.find(session => session.id === skillsSessionId.value))
const currentProject = computed(() => snapshot.value.projects.find(project => project.id === snapshot.value.requirements.find(requirement => requirement.id === detail.value?.session.requirementId)?.projectId))
function openProject(project: Project) {
  skillsProjectId.value = project.id
  skillsSessionId.value = ''
  skillContext.value = undefined
  page.value = 'project'
  sidebarOpen.value = false
  folderOpen.value = false
}
function openDirectorySkills() {
  if (!currentProject.value || !detail.value) return
  openProject(currentProject.value)
  skillsSessionId.value = detail.value.session.id
}
const groupOpen = pageFlag('group')
const groupKind = ref<'project' | 'requirement'>('project')
const groupForm = reactive({ name: '', cwd: '', endpointId: '', projectId: '' })
const secretListRevision = ref(0)
const navigationProjectId = computed(() => page.value === 'project' || page.value === 'skill' ? skillsProjectId.value : page.value === 'requirement' || page.value === 'newSession' ? viewedRequirement.value?.projectId : page.value === 'group' ? groupForm.projectId : '')
const navigationRequirementId = computed(() => page.value === 'requirement' || page.value === 'newSession' ? requirementViewId.value : '')
const projectScopes = computed(() => snapshot.value.endpoints.filter((value, index, all) => !value.local || all.findIndex(endpoint => endpoint.local) === index))
const localProject = computed(() => snapshot.value.endpoints.find(value => value.id === groupForm.endpointId)?.local === true)
const groupPageTitle = computed(() => t(groupKind.value === 'project' ? 'navigation.newProject' : 'navigation.newRequirement'))
const agentName = computed(() => endpoint.value?.provider === 'claude' ? 'Claude Code' : 'Codex')
const localName = (value: Endpoint) => value.local ? t('local.serviceName', { agent: value.name }) : value.name
async function confirmLocal() { await action(() => request('/local/confirm', 'POST')) }
async function detectLocal() {
  if (!snapshot.value.local?.confirmed) { remoteOnboarding.value = false; localSetupRequested.value = true }
  await action(() => request('/local/rescan', 'POST'))
}
const connectionsOpen = pageFlag('connections')
const secretsOpen = pageFlag('secrets')
const settingsOpen = pageFlag('settings')
watch(secretsOpen, open => { if (!open) ++secretListRevision.value })
watch(connectionsOpen, open => { if (!open) endpointForm.token = '' })
const pageTitle = computed(() => page.value === 'settings' ? t('settings.title') : page.value === 'connections' ? t('connection.connections') : page.value === 'group' ? groupPageTitle.value : page.value === 'newSession' ? t('session.new') : page.value === 'project' ? skillsProject.value?.name ?? t('skills.title') : page.value === 'requirement' ? (viewedRequirement.value ? requirementName(viewedRequirement.value) : t('navigation.requirement')) : page.value === 'skill' ? skillContext.value?.skill.name ?? t('skills.title') : t('secrets.title'))
const remoteFormOpen = ref(false)
const sidebarOpen = ref(false)
const busy = ref(false)
const sending = ref(false)
const permissionBusy = ref(false)
const replyBusy = ref('')
const editingEndpoint = ref('')
const endpointForm = reactive({ name: '', address: 'ws://127.0.0.1:3000/', token: '' })
const draft = ref('')
const requestId = ref('')
const timeline = ref<HTMLElement>()
const conversation = ref<HTMLElement>()
const followOutput = ref(true)
const newOutput = ref(false)
let scrollFrame = 0
let lastScrollTop = 0
const outputObserver = new ResizeObserver(() => {
  if (followOutput.value) scheduleFollow()
})
watch(conversation, (element, previous) => {
  if (previous) outputObserver.unobserve(previous)
  if (element) outputObserver.observe(element)
}, { flush: 'post' })
onBeforeUnmount(() => { outputObserver.disconnect(); cancelAnimationFrame(scrollFrame) })
const endpoint = computed(() => snapshot.value.endpoints.find(value => value.id === detail.value?.session.endpointId))
function requirementName(requirement: Requirement) { return requirement.defaultName ? t('navigation.general') : requirement.name }
const activeTurn = computed(() => detail.value?.turns.find(turn => turn.id === detail.value?.session.activeTurn))
const worktreeWriterBusy = computed(() => Boolean(endpoint.value?.local && snapshot.value.writers?.some(value => value.sessionId !== selectedId.value && value.directory === detail.value?.session.cwd)))
const canSend = computed(() => online.value && endpoint.value?.state === 'ready' && detail.value?.session.state === 'idle' && !worktreeWriterBusy.value && !sending.value && !permissionBusy.value && Boolean(draft.value.trim()))
async function savePermissions(mode: AccessMode) {
  const id = detail.value?.session.id
  if (!id) return
  await request(`/sessions/${id}`, 'PATCH', { accessMode: mode })
  await refresh()
}
const failedTurns = computed(() => detail.value?.turns.filter(turn => (turn.state === 'failed' || turn.state === 'unknown') && (!turn.error || turn.error !== detail.value?.session.error)) ?? [])
function notifyError(cause: unknown, fallback: TranslationKey) {
  ElMessage.error({ message: () => h('span', errorMessage(cause, fallback)) })
}

async function action(work: () => Promise<unknown>) {
  busy.value = true
  try { await work(); await refresh() } catch (cause) { notifyError(cause, 'errors.action') } finally { busy.value = false }
}

function openConnections(address?: string) {
  sidebarOpen.value = false
  editingEndpoint.value = ''
  endpointForm.name = ''
  endpointForm.address = address ?? 'ws://127.0.0.1:3000/'
  endpointForm.token = ''
  connectionsOpen.value = true
  remoteFormOpen.value = Boolean(address) || remoteOnboarding.value || !snapshot.value.endpoints.length
}
function editEndpoint(value: Endpoint) {
  remoteFormOpen.value = true
  editingEndpoint.value = value.id
  endpointForm.name = value.name
  endpointForm.address = value.address
  endpointForm.token = ''
}
async function saveEndpoint() {
  await action(async () => {
    const body: Record<string, string> = { name: endpointForm.name, address: endpointForm.address }
    if (!editingEndpoint.value || endpointForm.token) body.token = endpointForm.token
    await request(`/endpoints${editingEndpoint.value ? `/${editingEndpoint.value}` : ''}`, editingEndpoint.value ? 'PATCH' : 'POST', body)
    endpointForm.token = ''
    editingEndpoint.value = ''
    remoteFormOpen.value = false
    remoteOnboarding.value = false
  })
}
function newProject() {
  if (!projectScopes.value.length) { openConnections(); return }
  groupKind.value = 'project'
  groupForm.name = ''
  groupForm.endpointId = projectScopes.value.find(value => value.local === Boolean(endpoint.value?.local))?.id ?? projectScopes.value[0]!.id
  groupForm.cwd = localProject.value ? snapshot.value.local?.defaultDirectory ?? '' : ''
  groupForm.projectId = ''
  groupOpen.value = true
}
function newRequirement(project: Project) {
  groupKind.value = 'requirement'
  groupForm.name = ''
  groupForm.projectId = project.id
  groupOpen.value = true
}
async function saveProject(update: { secretIds: string[] }) {
  const project = skillsProject.value
  if (project) await action(() => request(`/projects/${project.id}`, 'PATCH', update))
}
function openRequirement(requirement: Requirement, section?: 'gitBranch') {
  requirementViewId.value = requirement.id
  page.value = 'requirement'
  sidebarOpen.value = false
  folderOpen.value = false
  if (section === 'gitBranch') void nextTick(() => {
    if (requirementViewId.value === requirement.id) requirementOverview.value?.selectBranchGroup()
  })
}
watch(() => snapshot.value.requirements, requirements => {
  if (!import.meta.env.DEV || route.query.prototype !== 'loop' || requirementViewId.value) return
  const requirement = requirements.find(value => value.id === route.query.requirement) ?? requirements[0]
  if (requirement) openRequirement(requirement)
}, { immediate: true })
function newSession(requirement: Requirement) {
  requirementViewId.value = requirement.id
  sessionForm.title = ''
  sessionForm.branchId = requirement.branchGroup?.members.length === 1 ? requirement.branchGroup.members[0]!.id : ''
  sessionForm.endpointId = defaultSessionAgent()
  sessionCreateError.value = undefined
  page.value = 'newSession'
  sidebarOpen.value = false
  folderOpen.value = false
}
async function createSession() {
  if (!canCreateSession.value || !viewedRequirement.value) return
  busy.value = true
  sessionCreateError.value = undefined
  try {
    const result = await request<{ id: string }>('/sessions', 'POST', { requirementId: viewedRequirement.value.id, endpointId: sessionForm.endpointId, title: sessionForm.title, branchId: sessionForm.branchId })
    await router.push(`/sessions/${result.id}`)
    await refresh()
  } catch (cause) {
    if (page.value === 'newSession') sessionCreateError.value = cause
    else notifyError(cause, 'errors.createSession')
  } finally { busy.value = false }
}

async function renameNavigation(kind: 'project' | 'requirement' | 'session', id: string, name: string) {
  await request(`/${kind}s/${id}`, 'PATCH', kind === 'session' ? { title: name } : { name })
  await refresh()
}
async function saveGroup() {
  await action(async () => {
    const collection = groupKind.value === 'project' ? 'projects' : 'requirements'
    const body = groupKind.value === 'project' ? { name: groupForm.name, endpointId: groupForm.endpointId, cwd: groupForm.cwd } : { name: groupForm.name, projectId: groupForm.projectId }
    await request(`/${collection}`, 'POST', body)
    groupOpen.value = false
  })
}
function chooseFolder(path: string) {
  groupForm.cwd = path
  folderOpen.value = false
}
async function send() {
  if (!canSend.value) return
  sending.value = true
  if (!requestId.value) requestId.value = crypto.randomUUID()
  saveDraft()
  const sessionId = selectedId.value
  const text = draft.value
  const clientRequestId = requestId.value
  try {
    await request(`/sessions/${sessionId}/turns`, 'POST', { text, clientRequestId })
    try { localStorage.removeItem(`loom.draft.${sessionId}`) } catch { /* A confirmed submission does not depend on browser storage. */ }
    if (selectedId.value === sessionId && requestId.value === clientRequestId) {
      draft.value = ''
      requestId.value = ''
      followOutput.value = true
    }
    await refresh()
  } catch (cause) { notifyError(cause, 'errors.submit') } finally { sending.value = false }
}
function taskKeydown(event: KeyboardEvent) {
  if (shouldSendTask(event)) {
    event.preventDefault()
    void send()
  }
}
async function respond(id: string, reply: Reply) {
  replyBusy.value = id
  try { await request(`/interactions/${id}/respond`, 'POST', reply); await refresh() } catch (cause) { notifyError(cause, 'errors.reply') } finally { replyBusy.value = '' }
}
function saveDraft() {
  if (!selectedId.value) return
  try { localStorage.setItem(`loom.draft.${selectedId.value}`, JSON.stringify({ text: draft.value, requestId: requestId.value })) } catch { /* The composer remains usable when browser storage is unavailable. */ }
}
function editDraft() { requestId.value = ''; saveDraft() }
watch(selectedId, () => {
  page.value = secretReturnPage.value = groupReturnPage.value = 'conversation'
  folderOpen.value = false
  skillsProjectId.value = ''
  skillsSessionId.value = ''
  requirementViewId.value = ''
  skillContext.value = undefined
  draft.value = ''
  requestId.value = ''
  followOutput.value = true
  newOutput.value = false
  lastScrollTop = 0
  try { const value = JSON.parse(localStorage.getItem(`loom.draft.${selectedId.value}`) ?? '{}'); draft.value = typeof value.text === 'string' ? value.text : ''; requestId.value = typeof value.requestId === 'string' ? value.requestId : '' } catch { /* An invalid saved draft is ignored. */ }
}, { immediate: true })
function trackScroll() {
  if (!timeline.value) return
  const { scrollHeight, scrollTop, clientHeight } = timeline.value
  if (scrollTop < lastScrollTop - 1) followOutput.value = false
  if (scrollHeight - scrollTop - clientHeight < 100) followOutput.value = true
  lastScrollTop = scrollTop
  if (followOutput.value) newOutput.value = false
}
function scheduleFollow() {
  if (scrollFrame) return
  scrollFrame = requestAnimationFrame(() => {
    scrollFrame = 0
    if (!followOutput.value || !timeline.value) return
    timeline.value.scrollTop = timeline.value.scrollHeight
    lastScrollTop = timeline.value.scrollTop
  })
}
async function scrollToBottom() {
  followOutput.value = true
  newOutput.value = false
  await nextTick()
  scheduleFollow()
}
watch(detail, async () => { if (followOutput.value) await scrollToBottom(); else newOutput.value = true })
watch(() => groupForm.endpointId, () => {
  if (groupKind.value === 'project') groupForm.cwd = localProject.value ? snapshot.value.local?.defaultDirectory ?? '' : ''
})
</script>

<template>
  <div class="workspace">
    <Transition name="fade"><button v-if="sidebarOpen" class="sidebar-backdrop" :aria-label="t('session.closeNavigation')" @click="sidebarOpen = false"></button></Transition>
    <aside class="sidebar" :class="{ open: sidebarOpen }" :aria-label="t('session.navigation')">
      <RouterLink to="/" class="brand" @click="sidebarOpen = false; page = 'conversation'"><svg viewBox="0 0 32 32" aria-hidden="true"><path d="M7 4v24M16 4v24M25 4v24M4 9h24M4 16h24M4 23h24" /><path class="weave" d="M7 9h9v7h9v7" /></svg><span>Loom</span></RouterLink>
      <button class="sidebar-add" @click="newProject"><FolderPlus :size="17" /><span>{{ t('navigation.newProject') }}</span></button>
      <div class="rail-heading">{{ t('navigation.projects') }}<span>{{ number(snapshot.projects.length) }}</span></div>
      <ProjectTree :projects="snapshot.projects" :requirements="snapshot.requirements" :sessions="snapshot.sessions" :endpoints="snapshot.endpoints" :selected-id="page === 'conversation' ? selectedId : ''" :active-project-id="navigationProjectId" :active-requirement-id="navigationRequirementId" :busy="busy" :rename="renameNavigation" @new-requirement="newRequirement" @new-session="newSession" @view-project="openProject" @view-requirement="openRequirement" @navigate="sidebarOpen = false; page = 'conversation'" />
      <div class="rail-bottom">
        <div class="rail-heading">{{ t('connection.services') }}</div>
        <button v-for="value in snapshot.endpoints" :key="value.id" class="rail-agent" @click="openConnections(); if (!value.local) editEndpoint(value)"><span class="rail-agent-icon" aria-hidden="true"><Radio :size="14" /></span><span class="rail-agent-details"><strong>{{ localName(value) }}</strong><small><i class="state-dot" :class="value.state" aria-hidden="true"></i>{{ label(value.state) }}</small></span></button>
        <button v-if="!snapshot.endpoints.length" class="rail-connect" @click="openConnections()"><Cable :size="17" />{{ t('connection.first') }}</button>
        <div v-if="!online" class="server-status" role="status"><span class="state-dot offline"></span>{{ t('connection.loomReconnecting') }}</div>
      </div>
      <div class="rail-actions">
        <button class="icon-button" :aria-label="t('secrets.title')" :title="t('secrets.title')" :disabled="secretsOpen" @click="secretsOpen = true"><KeyRound :size="17" /></button>
        <button class="icon-button" :aria-label="t('settings.title')" :title="t('settings.title')" :disabled="settingsOpen" @click="settingsOpen = true"><Settings :size="17" /></button>
        <LanguagePicker />
        <ThemePicker />
      </div>
    </aside>

    <main class="main-workspace">
      <header class="workspace-header">
        <button class="icon-button mobile-menu" :aria-label="t('session.openNavigation')" @click="sidebarOpen = true"><Menu :size="21" /></button>
        <div v-if="!localSetupOpen" class="workspace-title"><h1 v-if="page !== 'conversation'">{{ pageTitle }}</h1><strong v-else>{{ detail ? sessionTitle(detail.session) : t('navigation.projects') }}</strong></div>
        <SessionPermissions v-if="page === 'conversation' && detail && !localSetupOpen" :key="detail.session.id" :value="detail.session.accessMode" :active="Boolean(activeTurn)" :active-mode="activeTurn?.accessMode" :disabled="!online || busy || sending" :save="savePermissions" @busy="permissionBusy = $event" />
      </header>
      <div v-if="error" class="error-banner" role="alert">{{ message(error) }}</div>
      <LocalSetup v-if="snapshot.local && localSetupOpen" :status="snapshot.local" :busy="busy" :open="true" @confirm="confirmLocal" @rescan="detectLocal" @remote="localSetupRequested = false; remoteOnboarding = true; openConnections()" />
      <div v-else-if="page !== 'conversation'" class="settings-page" :class="{ 'settings-page-wide': secretsOpen, 'settings-page-requirement': page === 'requirement' }">
        <button v-if="page !== 'project' && page !== 'requirement'" class="settings-back" :disabled="busy" @click="backToWorkspace"><ArrowLeft :size="16" />{{ t(page === 'skill' ? 'skills.back' : 'common.back') }}</button>
        <ModelCatalogSettings v-if="settingsOpen" :catalog="snapshot.modelCatalog" :disabled="!online" :refresh="refresh" />
        <SecretManager v-if="secretsOpen" @changed="secretListRevision++" />
        <ProjectOverview :catalog="snapshot.modelCatalog" @manage-models="settingsOpen = true" :endpoints="snapshot.endpoints" :refresh="refresh" v-if="skillsProject" v-show="page === 'project'" :key="`${skillsProject.id}:${skillsSessionId}`" :project="skillsProject" :session="skillsSession" :refresh-key="secretListRevision" :busy="busy" @open-skill="openSkill" @save="saveProject" />
        <RequirementOverview :visible="page === 'requirement'" @manage-models="settingsOpen = true" v-if="viewedRequirement" v-show="page === 'requirement'" :key="viewedRequirement.id" ref="requirementOverview" :requirement="viewedRequirement" :workflow="viewedWorkflow" :workflow-session-state="workflowSessionState" :project="snapshot.projects.find(project => project.id === viewedRequirement?.projectId)" :requirements="snapshot.requirements" :busy="busy" :writers="snapshot.writers" :refresh="refresh" />
        <SkillDetail v-if="page === 'skill' && skillContext" :project="skillContext.project" :session="skillContext.session" :skill="skillContext.skill" />
        <section v-if="page === 'newSession' && viewedRequirement && requirementProject" class="settings-section">
          <p class="subtle">{{ requirementProject.name }} / {{ requirementName(viewedRequirement) }}</p>
          <form class="settings-form" novalidate @submit.prevent="createSession">
            <label for="new-session-title">{{ t('session.title') }}<small>{{ t('common.optional') }}</small></label>
            <input id="new-session-title" v-model="sessionForm.title" class="text-field" :placeholder="t('session.titlePlaceholder')" maxlength="120" :disabled="busy" />
            <label for="new-session-agent">{{ t('session.agent') }}</label>
            <select id="new-session-agent" v-model="sessionForm.endpointId" class="text-field" :disabled="busy || !sessionAgents.length"><option v-for="value in sessionAgents" :key="value.id" :value="value.id">{{ localName(value) }} — {{ label(value.state) }}</option></select>
            <template v-if="viewedRequirement.branchGroup">
              <label for="new-session-branch">{{ t('branchGroup.executionBranch') }}</label>
              <select id="new-session-branch" v-model="sessionForm.branchId" class="text-field" :disabled="busy || !sessionBranches.length"><option value="" disabled>{{ t('branchGroup.chooseBranch') }}</option><option v-for="member in sessionBranches" :key="member.id" :value="member.id" :disabled="member.workspace.state !== 'ready'">{{ member.workspace.branch }}</option></select>
              <p v-if="!sessionBranches.length" class="subtle" role="status">{{ t('branchGroup.addBeforeSession') }}</p>
            </template>
            <label>{{ t('session.cwd') }}</label>
            <code class="folder-path">{{ sessionBranch?.workspace.directory ?? requirementProject.cwd }}</code>
            <p v-if="sessionBranch && sessionBranch.workspace.state !== 'ready'" class="form-error" role="status">{{ t('errors.worktreeNotReady') }}</p>
            <p v-if="sessionCreationEndpoint?.state !== 'ready'" class="subtle" role="status">{{ t('requirement.chatNoAgent') }}</p>
            <p v-if="sessionCreateError" class="form-error" role="alert">{{ errorMessage(sessionCreateError, 'errors.createSession') }}</p>
            <button class="button primary" :disabled="!canCreateSession">{{ t(busy ? 'session.creating' : 'session.create') }}</button>
          </form>
        </section>
        <section v-if="connectionsOpen" class="settings-section">
          <div v-if="snapshot.endpoints.length" class="connection-list"><div v-for="value in snapshot.endpoints" :key="value.id" class="connection-row"><span class="state-dot" :class="value.state"></span><div><strong>{{ localName(value) }}</strong><code v-if="!value.local">{{ value.address }}</code><small>{{ value.error ? message(value.error) : label(value.state) }}</small></div><button v-if="!value.local" class="icon-button" :aria-label="t('connection.edit')" @click="editEndpoint(value)"><Pencil :size="16" /></button><button class="button" :disabled="busy" @click="action(() => request(`/endpoints/${value.id}/${value.state === 'ready' ? 'disconnect' : 'connect'}`, 'POST'))">{{ t(value.state === 'ready' ? 'connection.disconnect' : 'connection.connect') }}</button></div></div>
          <button v-if="!snapshot.local?.confirmed" class="button" @click="connectionsOpen = false; remoteOnboarding = false; localSetupRequested = true">{{ t('local.returnSetup') }}</button><button v-if="!remoteFormOpen" class="button" @click="remoteFormOpen = true">{{ t('local.addRemote') }}</button><form v-if="remoteFormOpen" class="settings-form" novalidate @submit.prevent="saveEndpoint"><div class="form-heading"><strong>{{ t(editingEndpoint ? 'connection.editService' : 'local.addRemote') }}</strong><button v-if="editingEndpoint" type="button" class="text-button" @click="openConnections()">{{ t('connection.addAnother') }}</button></div><label for="service-name">{{ t('connection.name') }}</label><input id="service-name" v-model="endpointForm.name" class="text-field" :placeholder="t('connection.namePlaceholder')" maxlength="120" required /><label for="service-address">{{ t('connection.address') }}</label><input id="service-address" v-model="endpointForm.address" class="text-field mono" placeholder="ws://192.168.1.20:3000/" required /><label for="service-token">{{ t('connection.token') }}<small>{{ t(editingEndpoint ? 'connection.keepToken' : 'connection.tokenOptional') }}</small></label><input id="service-token" v-model="endpointForm.token" class="text-field" type="password" autocomplete="new-password" maxlength="8192" /><button class="button primary" :disabled="busy || !endpointForm.name.trim()">{{ t(busy ? 'connection.saving' : 'connection.save') }}</button></form>
          <div class="connection-tools">
            <button class="text-button" :disabled="busy || snapshot.local?.state === 'scanning'" @click="detectLocal"><Radio :size="15" />{{ t('local.detectAgain') }}</button>
            <button v-if="remoteFormOpen || snapshot.endpoints.some(value => !value.local) || discovery" class="text-button" :disabled="discovery?.state === 'scanning'" @click="action(discover)"><Search :size="15" />{{ t(discovery?.state === 'scanning' ? 'discovery.running' : 'discovery.again') }}</button>
          </div>
          <div v-if="discovery" class="connection-discovery">
            <p v-if="discovery.state === 'scanning'" class="subtle">{{ t('discovery.progress', { checked: number(discovery.checked), total: number(discovery.total) }) }}</p>
            <p v-else-if="!discovery.candidates.length" class="subtle">{{ t('discovery.empty') }}</p>
            <button v-for="candidate in discovery.candidates" :key="candidate.address" class="candidate" @click="openConnections(candidate.address)"><span>{{ candidate.address }}</span><small>{{ message(candidate.note) }}</small></button>
            <p v-if="discovery.limited" class="subtle">{{ t('discovery.limited') }}</p>
          </div>
        </section>
        <section v-if="groupOpen" class="settings-section">
          <form class="settings-form" novalidate @submit.prevent="saveGroup">
            <label for="group-name">{{ t(groupKind === 'project' ? 'navigation.projectName' : 'navigation.requirementName') }}<small v-if="groupKind === 'project'">{{ t('common.optional') }}</small></label>
            <input id="group-name" v-model="groupForm.name" class="text-field" maxlength="120" :placeholder="t(groupKind === 'project' ? 'navigation.projectPlaceholder' : 'navigation.requirementPlaceholder')" :required="groupKind === 'requirement'" />
            <template v-if="groupKind === 'project'">
              <template v-if="projectScopes.length > 1">
                <label for="project-machine">{{ t('navigation.machine') }}</label>
                <select id="project-machine" v-model="groupForm.endpointId" class="text-field"><option v-for="value in projectScopes" :key="value.id" :value="value.id">{{ value.local ? t('navigation.thisComputer') : value.name }}</option></select>
              </template>
              <template v-if="localProject">
                <label>{{ t('local.projectFolder') }}</label><code class="folder-path">{{ groupForm.cwd }}</code>
                <button type="button" class="button" @click="folderOpen = true"><Folder :size="16" />{{ t('local.chooseFolder') }}</button><FolderPicker :open="folderOpen" :path="groupForm.cwd" @close="folderOpen = false" @choose="chooseFolder" />
              </template>
              <template v-else><label for="project-cwd">{{ t('session.cwd') }}</label><input id="project-cwd" v-model="groupForm.cwd" class="text-field mono" :placeholder="t('session.cwdPlaceholder')" required /></template>
            </template>
            <button class="button primary" :disabled="busy || (groupKind === 'requirement' && !groupForm.name.trim()) || (groupKind === 'project' && !groupForm.cwd.trim())">{{ t('navigation.create') }}</button>
          </form>
        </section>
      </div>
      <template v-else-if="detail">
        <div class="session-context"><button class="session-directory" :disabled="!currentProject" :aria-label="t('skills.viewDirectory', { directory: detail.session.cwd })" :title="detail.session.cwd" @click="openDirectorySkills"><Folder :size="16" /><code>{{ detail.session.cwd }}</code></button><span v-if="detail.session.workspace" class="context-branch" :title="detail.session.workspace.branch"><GitBranch :size="13" />{{ detail.session.workspace.branch }}</span><span class="context-agent">{{ endpoint?.name }}<span class="state-dot" :class="endpoint?.state"></span></span></div>
        <div v-if="detail.session.error" class="session-notice" role="status">{{ message(detail.session.error) }}<button v-if="detail.session.state === 'syncing'" class="button" :disabled="busy || endpoint?.state !== 'ready'" @click="action(() => request(`/sessions/${selectedId}/restore`, 'POST'))"><RefreshCw :size="15" />{{ t('session.synchronize') }}</button></div>
        <div ref="timeline" class="timeline" @scroll="trackScroll">
          <div :key="detail.session.id" ref="conversation" class="conversation">
            <TransitionGroup name="timeline-item">
              <div v-if="!detail.entries.length" key="empty" class="session-empty"><span class="empty-icon"><MessageSquare :size="27" /></span><h1>{{ t('session.emptyHeading', { agent: agentName }) }}</h1></div>
              <EntryCard v-for="entry in detail.entries" :key="entry.id" :entry="entry" :agent-name="agentName" />
              <div v-for="turn in failedTurns" :key="turn.id" class="turn-error" role="status"><strong>{{ t(turn.state === 'unknown' ? 'turn.unknown' : 'turn.failed') }}</strong><p v-if="turn.error">{{ message(turn.error) }}</p></div>
              <InteractionCard v-for="interaction in detail.interactions" :key="interaction.id" :interaction="interaction" :agent-name="agentName" :execution-detail="detail.entries.find(entry => entry.providerId === interaction.payload.itemId && entry.turnId === interaction.turnId)?.detail" :busy="replyBusy === interaction.id" :online="online && endpoint?.state === 'ready'" @respond="respond" />
            </TransitionGroup>
          </div>
        </div>
        <div class="composer-area">
          <Transition name="fade"><button v-if="newOutput" class="new-output button" @click="scrollToBottom">{{ t('composer.newOutput') }}</button></Transition>
          <form class="composer" @submit.prevent="send">
            <label for="task-input" class="sr-only">{{ t('composer.task', { agent: agentName }) }}</label>
            <textarea id="task-input" v-model="draft" rows="3" maxlength="65536" :placeholder="t('composer.placeholder')" :disabled="sending" @input="editDraft" @keydown="taskKeydown"></textarea>
            <div class="composer-controls"><div class="composer-meta"><span class="execution-status"><span class="state-dot" :class="detail.session.state"></span>{{ worktreeWriterBusy ? t('worktree.writerBusy') : label(detail.session.state) }}</span><span v-if="detail.session.model" class="composer-model">{{ detail.session.model }}</span></div><div><button v-if="activeTurn" type="button" class="button interrupt" :disabled="busy || !activeTurn.providerId || endpoint?.state !== 'ready'" @click="action(() => request(`/turns/${activeTurn!.id}/interrupt`, 'POST'))"><Square :size="12" />{{ t('composer.interrupt') }}</button><button v-if="detail.session.state === 'syncing' && !detail.session.error" type="button" class="button" :disabled="busy || endpoint?.state !== 'ready'" @click="action(() => request(`/sessions/${selectedId}/restore`, 'POST'))">{{ t('session.synchronize') }}</button><button class="send-button" type="submit" :disabled="!canSend" :aria-label="t('composer.send')"><ArrowUp :size="19" /></button></div></div>
          </form>
        </div>
      </template>
      <div v-else-if="loading" class="welcome"><p>{{ t('welcome.loading') }}</p></div>
      <div v-else class="welcome">
        <div class="welcome-content"><div class="welcome-mark"><svg viewBox="0 0 64 64" aria-hidden="true"><path d="M14 8v48M32 8v48M50 8v48M8 18h48M8 32h48M8 46h48" /><path class="weave" d="M14 18h18v14h18v14" /></svg></div><h1>{{ t('welcome.heading') }}</h1><p class="welcome-description">{{ t('welcome.description') }}</p>
        </div>
      </div>
    </main>


  </div>
</template>
