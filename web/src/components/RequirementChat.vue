<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { ArrowUp, MessageSquare, RefreshCw, Square } from '@lucide/vue'
import { request } from '../api'
import { useWorkspace } from '../composables/useWorkspace'
import { shouldSendTask } from '../composables/composerKeyboard'
import { useI18n } from '../i18n'
import EntryCard from './EntryCard.vue'
import InteractionCard from './InteractionCard.vue'
import RequirementWorkflow from './RequirementWorkflow.vue'
import SessionPermissions from './SessionPermissions.vue'
import RequirementModels from './RequirementModels.vue'
import type { AccessMode, MaterialKind, Project, Reply, Requirement, WorkflowAction, WorkflowDetail } from '../types'

const props = defineProps<{ visible: boolean; requirement: Requirement; project?: Project; section: string; sectionLabel: string; workflow: WorkflowDetail; workflowBusy: boolean; workflowError?: unknown; workflowAction: (input: WorkflowAction) => Promise<boolean> }>()
const emit = defineEmits<{ manageModels: []; settled: []; navigate: [value: MaterialKind | 'chat'] }>()
const { t, message, errorMessage, stateLabel } = useI18n()
const sessionId = ref('')
const { snapshot, detail, online, loading, error: workspaceError, refresh } = useWorkspace(() => sessionId.value)
const selectedEndpoint = ref('')
const sending = ref(false)
const permissionBusy = ref(false)
const modelBusy = ref(false)
const busy = ref(false)
const error = ref<unknown>()
const replyBusy = ref('')
const draft = ref('')
const requestId = ref('')
const draftKey = `loom.requirement.draft.${props.requirement.id}`
const inputId = `requirement-${props.requirement.id}-message`
try {
  const saved = JSON.parse(localStorage.getItem(draftKey) ?? '{}')
  draft.value = typeof saved.text === 'string' ? saved.text : ''
  requestId.value = typeof saved.requestId === 'string' ? saved.requestId : ''
} catch { /* Conversation drafts remain usable when browser storage is unavailable. */ }

const agents = computed(() => snapshot.value.endpoints.filter(endpoint => endpoint.state === 'ready' && (props.project?.local ? endpoint.local : endpoint.id === props.project?.endpointId)))
const endpoint = computed(() => snapshot.value.endpoints.find(value => value.id === (detail.value?.session.endpointId ?? selectedEndpoint.value)))
const agentName = computed(() => endpoint.value?.provider === 'claude' ? 'Claude Code' : 'Codex')
const activeTurn = computed(() => detail.value?.turns.find(turn => turn.id === detail.value?.session.activeTurn))
const accessMode = computed(() => detail.value?.session.accessMode ?? snapshot.value.requirements.find(value => value.id === props.requirement.id)?.conversationAccessMode ?? props.requirement.conversationAccessMode)
async function savePermissions(mode: AccessMode) {
  await request(`/requirements/${props.requirement.id}/conversation/access`, 'PATCH', { accessMode: mode })
  await refresh()
}
const writerBusy = computed(() => Boolean(endpoint.value?.local && snapshot.value.writers?.some(value => value.sessionId !== sessionId.value && [props.project?.cwd, props.requirement.workspace?.directory].includes(value.directory))))
watch(() => detail.value?.session.state, (state, previous) => { if (state === 'idle' && previous && previous !== 'idle') emit('settled') })
const workflowActive = computed(() => ['pending', 'running'].includes(props.workflow.workflow?.state ?? ''))
const canSend = computed(() => online.value && endpoint.value?.state === 'ready' && !writerBusy.value && !sending.value && !permissionBusy.value && !modelBusy.value && !busy.value && !props.workflowBusy && !workflowActive.value && Boolean(draft.value.trim()) && (!sessionId.value || detail.value?.session.state === 'idle'))
async function workflowAction(input: WorkflowAction) {
  const accepted = await props.workflowAction(input)
  if (accepted && input.action === 'start') { draft.value = ''; requestId.value = ''; saveDraft() }
  if (accepted) { follow.value = false; await refresh() }
  return accepted
}
watch(() => snapshot.value.sessions, sessions => {
  const session = sessions.find(value => value.requirementId === props.requirement.id && value.purpose === 'requirement')
  if (session) sessionId.value = session.id
}, { immediate: true })
watch(agents, values => {
  if (!values.some(value => value.id === selectedEndpoint.value)) selectedEndpoint.value = values.find(value => value.id === 'local-codex')?.id ?? values[0]?.id ?? ''
}, { immediate: true })

function saveDraft() {
  try { localStorage.setItem(draftKey, JSON.stringify({ text: draft.value, requestId: requestId.value })) } catch { /* Storage does not gate sending. */ }
}
function editDraft() { requestId.value = ''; saveDraft() }
async function send() {
  if (!canSend.value) return
  sending.value = true
  error.value = undefined
  if (!requestId.value) requestId.value = crypto.randomUUID()
  saveDraft()
  try {
    const result = await request<{ sessionId: string; turnId: string }>(`/requirements/${props.requirement.id}/conversation/turns`, 'POST', { text: draft.value, clientRequestId: requestId.value, endpointId: endpoint.value?.id, section: props.section })
    sessionId.value = result.sessionId
    draft.value = ''
    requestId.value = ''
    try { localStorage.removeItem(draftKey) } catch { /* A confirmed message does not require storage access. */ }
    follow.value = true
    await refresh()
  } catch (cause) { error.value = cause }
  finally { sending.value = false }
}
function keydown(event: KeyboardEvent) {
  if (shouldSendTask(event)) { event.preventDefault(); void send() }
}
async function action(path: string, body?: unknown) {
  busy.value = true
  error.value = undefined
  try { await request(path, 'POST', body); await refresh() }
  catch (cause) { error.value = cause }
  finally { busy.value = false }
}
async function respond(id: string, reply: Reply) {
  replyBusy.value = id
  try { await request(`/interactions/${id}/respond`, 'POST', reply); await refresh() }
  catch (cause) { error.value = cause }
  finally { replyBusy.value = '' }
}

const timeline = ref<HTMLElement>()
const conversation = ref<HTMLElement>()
const follow = ref(true)
const newOutput = ref(false)
watch(() => props.workflow.workflow?.state, state => {
  if (state && ['questions', 'review', 'preview', 'failed', 'cancelled'].includes(state)) {
    follow.value = false
    void nextTick(() => { if (timeline.value) timeline.value.scrollTop = 0; newOutput.value = false })
  }
}, { immediate: true })
function showPreparation() { follow.value = false; if (timeline.value) timeline.value.scrollTop = 0; newOutput.value = false }
let scrollFrame = 0
function scrollToBottom() {
  follow.value = true
  newOutput.value = false
  if (timeline.value) timeline.value.scrollTop = timeline.value.scrollHeight
}
function scheduleScroll() {
  if (scrollFrame) return
  scrollFrame = requestAnimationFrame(() => { scrollFrame = 0; if (follow.value) scrollToBottom(); else if (detail.value?.session.activeTurn) newOutput.value = true })
}
const observer = new ResizeObserver(scheduleScroll)
watch(conversation, (element, previous) => { if (previous) observer.unobserve(previous); if (element) observer.observe(element) }, { flush: 'post' })
watch(detail, () => { void nextTick(scheduleScroll) }, { flush: 'post' })
function trackScroll() {
  if (!timeline.value) return
  const element = timeline.value
  follow.value = element.scrollHeight - element.scrollTop - element.clientHeight < 100
  if (follow.value) newOutput.value = false
}
onBeforeUnmount(() => { observer.disconnect(); cancelAnimationFrame(scrollFrame) })
</script>

<template>
  <section class="requirement-chat" :aria-label="t('requirement.chat')">
    <header class="requirement-chat-heading"><h2><MessageSquare :size="17" aria-hidden="true" />{{ t('requirement.chat') }}</h2><button v-if="workflow.workflow" class="button" @click="showPreparation">{{ t('workflow.viewProgress') }}</button><small v-if="sessionId && endpoint">{{ endpoint.name }}</small><RequirementModels :catalog="snapshot.modelCatalog" @manage-models="emit('manageModels')" v-if="project" :visible="visible" :project="snapshot.projects.find(value => value.id === project?.id) ?? project" :requirement="snapshot.requirements.find(value => value.id === requirement.id) ?? requirement" :endpoints="snapshot.endpoints" :endpoint="endpoint" :active-model="activeTurn?.model ?? detail?.session.model" :active="Boolean(activeTurn)" :disabled="!online || busy || sending" :refresh="refresh" @busy="modelBusy = $event" /><SessionPermissions :value="accessMode" :active="Boolean(activeTurn)" :active-mode="activeTurn?.accessMode" :disabled="!online || busy || sending" :save="savePermissions" @busy="permissionBusy = $event" /><p>{{ t('requirement.chatSection', { section: sectionLabel }) }}</p></header>
    <div v-if="!sessionId && agents.length > 1" class="requirement-chat-agent"><label :for="`${inputId}-agent`">{{ t('requirement.chatAgent') }}</label><select :id="`${inputId}-agent`" v-model="selectedEndpoint" class="text-field" :disabled="sending"><option v-for="agent in agents" :key="agent.id" :value="agent.id">{{ agent.name }}</option></select></div>
    <div ref="timeline" class="requirement-chat-timeline" @scroll="trackScroll">
      <div ref="conversation">
        <RequirementWorkflow :detail="workflow" :busy="workflowBusy" :error="workflowError" :act="workflowAction" view="chat" :brief="draft" :endpoint-id="endpoint?.id" :can-start="canSend" @navigate="emit('navigate', $event)" />
        <p v-if="loading" class="subtle">{{ t('welcome.loading') }}</p>
        <p v-else-if="!detail?.entries.length" class="requirement-chat-empty">{{ t('requirement.chatEmpty') }}</p>
        <EntryCard v-for="entry in detail?.entries ?? []" :key="entry.id" :entry="entry" :agent-name="agentName" />
        <InteractionCard v-for="interaction in detail?.interactions ?? []" :key="interaction.id" :interaction="interaction" :agent-name="agentName" :execution-detail="detail?.entries.find(entry => entry.providerId === interaction.payload.itemId && entry.turnId === interaction.turnId)?.detail" :busy="replyBusy === interaction.id" :online="online && endpoint?.state === 'ready'" @respond="respond" />
        <p v-if="detail?.session.error" class="form-error" role="status">{{ message(detail.session.error) }}</p>
        <p v-if="workspaceError" class="form-error" role="alert">{{ message(workspaceError) }}</p>
        <p v-if="error" class="form-error" role="alert">{{ errorMessage(error, 'errors.submit') }}</p>
      </div>
    </div>
    <div class="requirement-chat-composer">
      <button v-if="newOutput" class="button requirement-chat-new-output" @click="scrollToBottom">{{ t('composer.newOutput') }}</button>
      <form class="composer" @submit.prevent="send">
        <label :for="inputId" class="sr-only">{{ t('requirement.chatInput') }}</label>
        <textarea :id="inputId" v-model="draft" rows="3" maxlength="65536" :placeholder="t('requirement.chatPlaceholder')" :disabled="sending" @input="editDraft" @keydown="keydown"></textarea>
        <div class="composer-controls"><span class="execution-status">{{ !online ? t('connection.loomReconnecting') : endpoint?.state !== 'ready' ? t('requirement.chatNoAgent') : writerBusy ? t('worktree.writerBusy') : workflowActive ? t('workflow.activeConversation') : detail ? stateLabel(detail.session.state) : t('requirement.chatReady') }}</span><div class="requirement-chat-buttons"><button v-if="activeTurn" type="button" class="button interrupt" :disabled="busy || !activeTurn.providerId || endpoint?.state !== 'ready'" @click="action(`/turns/${activeTurn.id}/interrupt`)"><Square :size="12" />{{ t('composer.interrupt') }}</button><button v-if="detail?.session.state === 'syncing'" type="button" class="icon-button" :aria-label="t('session.synchronize')" :disabled="busy || endpoint?.state !== 'ready'" @click="action(`/sessions/${sessionId}/restore`)"><RefreshCw :size="16" /></button><button class="send-button" type="submit" :disabled="!canSend" :aria-label="t('requirement.chatSend')"><ArrowUp :size="19" /></button></div></div>
      </form>
    </div>
  </section>
</template>
