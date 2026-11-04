<script setup lang="ts">
import { computed, nextTick, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Plus, Radio, Cable, ArrowUp, Square, Menu, ChevronRight, Pencil, RefreshCw, Folder, MessageSquare, Search } from '@lucide/vue'
import { request } from '../api'
import { useWorkspace } from '../composables/useWorkspace'
import EntryCard from '../components/EntryCard.vue'
import InteractionCard from '../components/InteractionCard.vue'
import type { Endpoint, Reply } from '../types'

const router = useRouter()
const { snapshot, detail, discovery, online, loading, error, refresh, discover, selectedId } = useWorkspace()
const connectionsOpen = ref(false)
const sessionOpen = ref(false)
const renameOpen = ref(false)
const sidebarOpen = ref(false)
const busy = ref(false)
const sending = ref(false)
const replyBusy = ref('')
const editingEndpoint = ref('')
const endpointForm = reactive({ name: '', address: 'ws://127.0.0.1:3000/', token: '' })
const sessionForm = reactive({ endpointId: '', title: '', cwd: '', model: '' })
const renameTitle = ref('')
const draft = ref('')
const requestId = ref('')
const timeline = ref<HTMLElement>()
const followOutput = ref(true)
const newOutput = ref(false)
const endpoint = computed(() => snapshot.value.endpoints.find(value => value.id === detail.value?.session.endpointId))
const readyEndpoints = computed(() => snapshot.value.endpoints.filter(value => value.state === 'ready'))
const activeTurn = computed(() => detail.value?.turns.find(turn => turn.id === detail.value?.session.activeTurn))
const canSend = computed(() => online.value && endpoint.value?.state === 'ready' && detail.value?.session.state === 'idle' && !sending.value && Boolean(draft.value.trim()))
const failedTurns = computed(() => detail.value?.turns.filter(turn => turn.state === 'failed' || turn.state === 'unknown') ?? [])
const statusLabels: Record<string, string> = { idle: 'Ready for a task', running: 'Working', waiting_approval: 'Waiting for approval', waiting_input: 'Waiting for your input', syncing: 'Synchronization required', pending: 'Submitting', ready: 'Connected', auth_required: 'Token required', reconnecting: 'Reconnecting', connecting: 'Connecting', offline: 'Offline', disconnected: 'Disconnected', incompatible: 'Incompatible service' }
const label = (state: string) => statusLabels[state] ?? state

async function action(work: () => Promise<unknown>) {
  busy.value = true
  try { await work(); await refresh() } catch (cause) { ElMessage.error(cause instanceof Error ? cause.message : 'The action failed.') } finally { busy.value = false }
}

function openConnections(address?: string) {
  editingEndpoint.value = ''
  endpointForm.name = ''
  endpointForm.address = address ?? 'ws://127.0.0.1:3000/'
  endpointForm.token = ''
  connectionsOpen.value = true
}
function editEndpoint(value: Endpoint) {
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
    ElMessage.success('Service saved. Connecting to Codex.')
  })
}
function newSession() {
  if (!readyEndpoints.value.length) { openConnections(); return }
  sessionForm.endpointId = endpoint.value?.state === 'ready' ? endpoint.value.id : readyEndpoints.value[0]!.id
  sessionForm.cwd = detail.value?.session.cwd ?? ''
  sessionForm.title = ''
  sessionForm.model = ''
  sessionOpen.value = true
}
async function createSession() {
  await action(async () => {
    const result = await request<{ id: string }>('/sessions', 'POST', sessionForm)
    sessionOpen.value = false
    sidebarOpen.value = false
    await router.push(`/sessions/${result.id}`)
  })
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
  } catch (cause) { ElMessage.error(cause instanceof Error ? cause.message : 'Task submission failed. Your draft is saved.') } finally { sending.value = false }
}
async function respond(id: string, reply: Reply) {
  replyBusy.value = id
  try { await request(`/interactions/${id}/respond`, 'POST', reply); await refresh() } catch (cause) { ElMessage.error(cause instanceof Error ? cause.message : 'Could not send the response.') } finally { replyBusy.value = '' }
}
function saveDraft() {
  if (!selectedId.value) return
  try { localStorage.setItem(`loom.draft.${selectedId.value}`, JSON.stringify({ text: draft.value, requestId: requestId.value })) } catch { /* The composer remains usable when browser storage is unavailable. */ }
}
function editDraft() { requestId.value = ''; saveDraft() }
watch(selectedId, () => {
  draft.value = ''
  requestId.value = ''
  followOutput.value = true
  newOutput.value = false
  try { const value = JSON.parse(localStorage.getItem(`loom.draft.${selectedId.value}`) ?? '{}'); draft.value = typeof value.text === 'string' ? value.text : ''; requestId.value = typeof value.requestId === 'string' ? value.requestId : '' } catch { /* An invalid saved draft is ignored. */ }
}, { immediate: true })
function trackScroll() {
  if (timeline.value) followOutput.value = timeline.value.scrollHeight - timeline.value.scrollTop - timeline.value.clientHeight < 100
  if (followOutput.value) newOutput.value = false
}
async function scrollToBottom() {
  await nextTick()
  timeline.value?.scrollTo({ top: timeline.value.scrollHeight })
  followOutput.value = true
  newOutput.value = false
}
watch(detail, async () => { if (followOutput.value) await scrollToBottom(); else newOutput.value = true })
function openRename() { renameTitle.value = detail.value?.session.title ?? ''; renameOpen.value = true }
async function renameSession() { await action(async () => { await request(`/sessions/${selectedId.value}`, 'PATCH', { title: renameTitle.value }); renameOpen.value = false }) }
</script>

<template>
  <div class="workspace">
    <button v-if="sidebarOpen" class="sidebar-backdrop" aria-label="Close session navigation" @click="sidebarOpen = false"></button>
    <aside class="sidebar" :class="{ open: sidebarOpen }" aria-label="Sessions and agents">
      <RouterLink to="/" class="brand" @click="sidebarOpen = false"><svg viewBox="0 0 32 32" aria-hidden="true"><path d="M7 4v24M16 4v24M25 4v24M4 9h24M4 16h24M4 23h24" /><path class="weave" d="M7 9h9v7h9v7" /></svg><span>Loom</span><small>Workspace</small></RouterLink>
      <button class="new-session" @click="newSession"><Plus :size="18" />New session</button>
      <div class="rail-heading">Sessions<span>{{ snapshot.sessions.length }}</span></div>
      <nav class="session-list" aria-label="Session history">
        <RouterLink v-for="session in snapshot.sessions" :key="session.id" :to="`/sessions/${session.id}`" class="session-link" :class="{ selected: selectedId === session.id }" @click="sidebarOpen = false">
          <MessageSquare :size="16" /><span><strong>{{ session.title }}</strong><small>{{ snapshot.endpoints.find(value => value.id === session.endpointId)?.name ?? 'Codex' }}</small></span><i class="state-dot" :class="session.state" :title="label(session.state)"></i>
        </RouterLink>
        <p v-if="!snapshot.sessions.length" class="rail-empty">Your sessions will appear here.</p>
      </nav>
      <div class="rail-bottom">
        <div class="rail-heading">Agent services<button aria-label="Manage agent services" @click="openConnections()"><Plus :size="16" /></button></div>
        <button v-for="value in snapshot.endpoints" :key="value.id" class="rail-agent" @click="connectionsOpen = true; editEndpoint(value)"><span class="state-dot" :class="value.state"></span><span><strong>{{ value.name }}</strong><small>{{ label(value.state) }}</small></span><ChevronRight :size="14" /></button>
        <button v-if="!snapshot.endpoints.length" class="rail-connect" @click="openConnections()"><Cable :size="17" />Connect your first agent</button>
        <div class="server-status"><span class="state-dot" :class="online ? 'ready' : 'offline'"></span>{{ online ? 'Loom is connected' : 'Reconnecting to Loom' }}<span>v{{ snapshot.version }}</span></div>
      </div>
    </aside>

    <main class="main-workspace">
      <header class="workspace-header">
        <button class="icon-button mobile-menu" aria-label="Open session navigation" @click="sidebarOpen = true"><Menu :size="21" /></button>
        <div class="workspace-title"><span>{{ detail ? 'Conversation' : 'Agent workspace' }}</span><strong>{{ detail?.session.title ?? 'Your next task starts here' }}</strong></div>
        <div class="header-actions"><button class="button quiet" :disabled="discovery?.state === 'scanning'" @click="action(discover)"><Radio :size="16" /><span>{{ discovery?.state === 'scanning' ? 'Discovering' : 'Discover agents' }}</span></button><button class="button" @click="openConnections()"><Cable :size="16" /><span>Connections</span></button></div>
      </header>
      <div v-if="error" class="error-banner" role="alert">{{ error }}</div>
      <template v-if="detail">
        <div class="session-context"><Folder :size="16" /><code>{{ detail.session.cwd }}</code><span class="context-agent">{{ endpoint?.name }}<span class="state-dot" :class="endpoint?.state"></span></span><button class="icon-button" aria-label="Rename session" @click="openRename"><Pencil :size="15" /></button></div>
        <div v-if="detail.session.error" class="session-notice" role="status">{{ detail.session.error }}<button v-if="detail.session.state === 'syncing'" class="button" :disabled="busy || endpoint?.state !== 'ready'" @click="action(() => request(`/sessions/${selectedId}/restore`, 'POST'))"><RefreshCw :size="15" />Synchronize</button></div>
        <div ref="timeline" class="timeline" @scroll="trackScroll">
          <div class="conversation">
            <div v-if="!detail.entries.length" class="session-empty"><span class="empty-icon"><MessageSquare :size="27" /></span><h1>What should Codex work on?</h1><p>Describe a task for <strong>{{ detail.session.title }}</strong>. Codex will work in the project directory above.</p></div>
            <EntryCard v-for="entry in detail.entries" :key="entry.id" :entry="entry" />
            <div v-for="turn in failedTurns" :key="turn.id" class="turn-error" role="status"><strong>{{ turn.state === 'unknown' ? 'Execution status is unknown' : 'Task failed' }}</strong><p>{{ turn.error }}</p></div>
            <InteractionCard v-for="interaction in detail.interactions" :key="interaction.id" :interaction="interaction" :execution-detail="detail.entries.find(entry => entry.providerId === interaction.payload.itemId && entry.turnId === interaction.turnId)?.detail" :busy="replyBusy === interaction.id" :online="online && endpoint?.state === 'ready'" @respond="respond" />
          </div>
        </div>
        <div class="composer-area">
          <button v-if="newOutput" class="new-output button" @click="scrollToBottom">New output available</button>
          <form class="composer" @submit.prevent="send">
            <label for="task-input" class="sr-only">Task for Codex</label>
            <textarea id="task-input" v-model="draft" rows="3" maxlength="65536" placeholder="Describe your task, ask a question, or continue the conversation…" :disabled="sending" @input="editDraft" @keydown.enter.exact.prevent="send"></textarea>
            <div class="composer-controls"><span class="execution-status"><span class="state-dot" :class="detail.session.state"></span>{{ label(detail.session.state) }}</span><div><button v-if="activeTurn" type="button" class="button interrupt" :disabled="busy || !activeTurn.providerId || endpoint?.state !== 'ready'" @click="action(() => request(`/turns/${activeTurn!.id}/interrupt`, 'POST'))"><Square :size="12" />Interrupt</button><button v-if="detail.session.state === 'syncing' && !detail.session.error" type="button" class="button" :disabled="busy || endpoint?.state !== 'ready'" @click="action(() => request(`/sessions/${selectedId}/restore`, 'POST'))">Synchronize</button><button class="send-button" type="submit" :disabled="!canSend" aria-label="Send task"><ArrowUp :size="19" /></button></div></div>
          </form>
          <div class="composer-note"><span>Enter to send. Shift + Enter for a new line.</span><span>{{ detail.session.model || 'Service default model' }}</span></div>
        </div>
      </template>
      <div v-else-if="loading" class="welcome"><p>Loading your workspace…</p></div>
      <div v-else class="welcome">
        <div class="welcome-content"><div class="welcome-mark"><svg viewBox="0 0 64 64" aria-hidden="true"><path d="M14 8v48M32 8v48M50 8v48M8 18h48M8 32h48M8 46h48" /><path class="weave" d="M14 18h18v14h18v14" /></svg></div><h1>A workspace for<br />your coding agents.</h1><p>Connect a service, choose a project, and turn a task into a conversation. Keep the work and its history in one place.</p><div class="welcome-actions"><button class="button primary" @click="readyEndpoints.length ? newSession() : openConnections()"><Plus :size="18" />{{ readyEndpoints.length ? 'Start a session' : 'Connect an agent' }}</button><span>{{ readyEndpoints.length ? `${readyEndpoints.length} service${readyEndpoints.length === 1 ? '' : 's'} connected` : 'Codex is available in this first build' }}</span></div>
          <div class="discovery-summary"><div><Radio :size="18" /><strong>{{ discovery?.state === 'scanning' ? 'Looking for agent services' : 'Nearby services' }}</strong><small v-if="discovery?.state === 'scanning'">{{ discovery.checked }} / {{ discovery.total }} checked</small></div><p v-if="!discovery?.candidates.length">{{ discovery?.state === 'scanning' ? 'You can enter an address while discovery runs.' : 'No services found. Start an app-server on your target machine or enter its address.' }}</p><button v-for="candidate in discovery?.candidates ?? []" :key="candidate.address" class="candidate" @click="openConnections(candidate.address)"><span class="state-dot" :class="candidate.verified ? 'ready' : 'offline'"></span><span><code>{{ candidate.address }}</code><small>{{ candidate.note }}</small></span><ChevronRight :size="17" /></button><p v-if="discovery?.limited" class="subtle">Discovery scope is limited. Enter an address for services outside the scanned range.</p></div>
        </div>
      </div>
    </main>

    <ElDialog v-model="connectionsOpen" title="Agent connections" width="640px" class="loom-dialog" :close-on-click-modal="false">
      <p class="dialog-intro">Connect to a Codex app-server on this computer or your LAN. The service must be running and reachable.</p>
      <div v-if="snapshot.endpoints.length" class="connection-list"><div v-for="value in snapshot.endpoints" :key="value.id" class="connection-row"><span class="state-dot" :class="value.state"></span><div><strong>{{ value.name }}</strong><code>{{ value.address }}</code><small>{{ value.error || label(value.state) }}</small></div><button class="icon-button" aria-label="Edit connection" @click="editEndpoint(value)"><Pencil :size="16" /></button><button class="button" :disabled="busy" @click="action(() => request(`/endpoints/${value.id}/${value.state === 'ready' ? 'disconnect' : 'connect'}`, 'POST'))">{{ value.state === 'ready' ? 'Disconnect' : 'Connect' }}</button></div></div>
      <form class="dialog-form" @submit.prevent="saveEndpoint"><div class="form-heading"><strong>{{ editingEndpoint ? 'Edit service' : 'Add service' }}</strong><button v-if="editingEndpoint" type="button" class="text-button" @click="openConnections()">Add another</button></div><label for="service-name">Service name</label><input id="service-name" v-model="endpointForm.name" class="text-field" placeholder="Workstation Codex" maxlength="120" required /><label for="service-address">WebSocket address</label><input id="service-address" v-model="endpointForm.address" class="text-field mono" placeholder="ws://192.168.1.20:3000/" required /><label for="service-token">Service token <small>{{ editingEndpoint ? '(leave empty to keep the saved token)' : '(if required)' }}</small></label><input id="service-token" v-model="endpointForm.token" class="text-field" type="password" autocomplete="new-password" maxlength="8192" /><button class="button primary" :disabled="busy || !endpointForm.name.trim()">{{ busy ? 'Saving…' : 'Save and connect' }}</button></form>
      <div class="dialog-discovery"><button class="text-button" :disabled="discovery?.state === 'scanning'" @click="action(discover)"><Search :size="15" />{{ discovery?.state === 'scanning' ? 'Discovery is running' : 'Discover again' }}</button><button v-for="candidate in discovery?.candidates ?? []" :key="candidate.address" class="candidate" @click="openConnections(candidate.address)"><span>{{ candidate.address }}</span><small>{{ candidate.note }}</small></button></div>
    </ElDialog>
    <ElDialog v-model="sessionOpen" title="New session" width="500px" class="loom-dialog" :close-on-click-modal="false"><form class="dialog-form" @submit.prevent="createSession"><label for="session-agent">Agent service</label><select id="session-agent" v-model="sessionForm.endpointId" class="text-field" required><option v-for="value in readyEndpoints" :key="value.id" :value="value.id">{{ value.name }}</option></select><label for="session-title">Session title</label><input id="session-title" v-model="sessionForm.title" class="text-field" maxlength="120" placeholder="Build a feature" /><label for="session-cwd">Project directory on the target machine</label><input id="session-cwd" v-model="sessionForm.cwd" class="text-field mono" placeholder="D:\Projects\my-app or /home/me/my-app" required /><label for="session-model">Model <small>(optional)</small></label><input id="session-model" v-model="sessionForm.model" class="text-field" maxlength="120" placeholder="Use the service default" /><p class="subtle">Codex uses workspace write access and requests approval when needed.</p><button class="button primary" :disabled="busy || !sessionForm.cwd.trim()">{{ busy ? 'Creating…' : 'Create session' }}</button></form></ElDialog>
    <ElDialog v-model="renameOpen" title="Rename session" width="400px" class="loom-dialog"><form class="dialog-form" @submit.prevent="renameSession"><label for="rename-title">Session title</label><input id="rename-title" v-model="renameTitle" class="text-field" maxlength="120" required /><button class="button primary" :disabled="busy">Save title</button></form></ElDialog>
  </div>
</template>
