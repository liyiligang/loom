<script setup lang="ts">
import { computed, defineAsyncComponent, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { Database, FileText, Compass, ListChecks, Workflow, NotebookPen, GitBranch, MessageSquare, PanelsTopLeft, ChevronDown } from '@lucide/vue'
import { useI18n } from '../i18n'
import type { MaterialKind, Project, Requirement, RequirementWorkflow as WorkflowState } from '../types'
import { useRequirementWorkflow } from '../composables/useRequirementWorkflow'
import RequirementWorkflow from './RequirementWorkflow.vue'
import RequirementGit from './RequirementGit.vue'
import RequirementChat from './RequirementChat.vue'
import RequirementMaterials from './RequirementMaterials.vue'

const RequirementLoop = defineAsyncComponent(() => import('./RequirementLoop.vue'))
const props = defineProps<{ visible?: boolean; requirement: Requirement; workflow?: WorkflowState; workflowSessionState?: string; project?: Project; requirements: Requirement[]; busy: boolean; refresh: () => Promise<void>; writers?: { sessionId: string; directory: string }[] }>()
const emit = defineEmits<{ manageModels: [] }>()
const flow = useRequirementWorkflow(() => props.requirement.id, () => props.workflow?.revision)
const { t } = useI18n()
type ContentTab = MaterialKind | 'gitBranch'
type Tab = ContentTab | 'chat'
const route = useRoute()
const prototypeEnabled = computed(() => import.meta.env.DEV && route.query.prototype === 'loop')
const LoopDesignPrototype = import.meta.env.DEV ? defineAsyncComponent(() => import('../prototypes/loop-design/LoopDesignPrototype.vue')) : undefined
const tab = ref<Tab>(prototypeEnabled.value ? 'loopDesign' : 'chat')
const contextTab = ref<ContentTab>(prototypeEnabled.value ? 'loopDesign' : 'documents')
const gitVisited = ref(false)
const refreshKey = ref(0)
const materialBusy = ref(false)
const buttons = new Map<Tab, HTMLButtonElement>()
const scopeId = `requirement-${props.requirement.id}`
const navigation = ref<HTMLElement>()
const tabsRail = ref<HTMLElement>()
const moreButton = ref<HTMLButtonElement>()
const chat = ref<InstanceType<typeof RequirementChat>>()
const moreOpen = ref(false)
const menuButtons = new Map<Tab, HTMLButtonElement>()
const primaryTabs: Tab[] = ['chat', 'documents', 'designDocuments', 'loopDesign']
const tabs = computed(() => [
  { id: 'chat' as const, label: t('requirement.chat'), empty: '', icon: MessageSquare },
  { id: 'documents' as const, label: t('requirement.documents'), empty: t('requirement.noDocuments'), icon: FileText },
  { id: 'designDocuments' as const, label: t('requirement.designDocuments'), empty: t('requirement.noDesignDocuments'), icon: NotebookPen },
  { id: 'prototypes' as const, label: t('requirement.prototypes'), empty: t('requirement.noPrototypes'), icon: PanelsTopLeft },
  { id: 'decisions' as const, label: t('requirement.decisions'), empty: t('requirement.noDecisions'), icon: Compass },
  { id: 'loopDesign' as const, label: t('requirement.loopDesign'), empty: t('requirement.noLoopDesign'), icon: Workflow },
  { id: 'gitBranch' as const, label: t('branchGroup.title'), empty: '', icon: GitBranch },
  { id: 'testCases' as const, label: t('requirement.testCases'), empty: t('requirement.noTestCases'), icon: ListChecks },
  { id: 'datasets' as const, label: t('requirement.datasets'), empty: t('requirement.noDatasets'), icon: Database },
])
const visibleTabs = computed(() => [...tabs.value.filter(item => primaryTabs.includes(item.id)), ...tabs.value.filter(item => item.id === tab.value && !primaryTabs.includes(item.id))])
const secondaryTabs = computed(() => tabs.value.filter(item => !primaryTabs.includes(item.id)))
function setMenuButton(value: Tab, element: unknown) {
  if (element instanceof HTMLButtonElement) menuButtons.set(value, element)
  else menuButtons.delete(value)
}
function closeMore(focus = false) { moreOpen.value = false; if (focus) moreButton.value?.focus() }
function outsideNavigation(event: PointerEvent) { if (event.target instanceof Node && !navigation.value?.contains(event.target)) closeMore() }
let tabsObserver: ResizeObserver | undefined
onMounted(() => {
  document.addEventListener('pointerdown', outsideNavigation)
  tabsObserver = new ResizeObserver(() => buttons.get(tab.value)?.scrollIntoView({ block: 'nearest', inline: 'nearest' }))
  if (tabsRail.value) tabsObserver.observe(tabsRail.value)
})
onBeforeUnmount(() => { document.removeEventListener('pointerdown', outsideNavigation); tabsObserver?.disconnect() })
function openMore() {
  moreOpen.value = true
  void nextTick(() => menuButtons.get(secondaryTabs.value.find(item => item.id === tab.value)?.id ?? secondaryTabs.value[0]!.id)?.focus())
}
function navigateMenu(event: KeyboardEvent) {
  if (event.key === 'Escape') { event.preventDefault(); closeMore(true); return }
  if (event.key === 'Tab') {
    event.preventDefault()
    closeMore()
    void nextTick(() => (event.shiftKey ? buttons.get(tab.value) : document.getElementById(`${scopeId}-${tab.value}-panel`))?.focus({ preventScroll: true }))
    return
  }
  const current = secondaryTabs.value.findIndex(item => menuButtons.get(item.id) === document.activeElement)
  const index = event.key === 'Home' ? 0 : event.key === 'End' ? secondaryTabs.value.length - 1 : event.key === 'ArrowDown' ? (current + 1) % secondaryTabs.value.length : event.key === 'ArrowUp' ? (current + secondaryTabs.value.length - 1) % secondaryTabs.value.length : -1
  if (index >= 0) { event.preventDefault(); menuButtons.get(secondaryTabs.value[index]!.id)?.focus() }
}
function openGeneration() { selectTab('chat'); void nextTick(() => chat.value?.focusComposer()) }

function setButton(value: Tab, element: unknown) {
  if (element instanceof HTMLButtonElement) buttons.set(value, element)
  else buttons.delete(value)
}
function selectTab(value: Tab, focus = false) {
  if (props.busy || materialBusy.value) return
  tab.value = value
  closeMore()
  if (value !== 'chat') contextTab.value = value
  if (value === 'gitBranch') gitVisited.value = true
  void nextTick(() => { if (focus) buttons.get(value)?.focus({ preventScroll: true }); buttons.get(value)?.scrollIntoView({ block: 'nearest', inline: 'nearest' }) })
}
defineExpose({ selectBranchGroup: () => selectTab('gitBranch') })
function navigateTabs(event: KeyboardEvent) {
  const current = visibleTabs.value.findIndex(item => item.id === tab.value)
  let index: number
  if (event.key === 'Home') index = 0
  else if (event.key === 'End') index = visibleTabs.value.length - 1
  else if (event.key === 'ArrowRight') index = (current + 1) % visibleTabs.value.length
  else if (event.key === 'ArrowLeft') index = (current + visibleTabs.value.length - 1) % visibleTabs.value.length
  else return
  event.preventDefault()
  selectTab(visibleTabs.value[index]!.id, true)
}
</script>

<template>
  <section class="requirement-overview requirement-layout" :class="{ 'loop-prototype-layout': prototypeEnabled && tab === 'loopDesign' }">
    <div class="requirement-content">
    <div ref="navigation" class="requirement-navigation">
      <div ref="tabsRail" class="content-tabs" role="tablist" :aria-label="t('requirement.sections')" @keydown="navigateTabs">
        <button v-for="item in visibleTabs" :id="`${scopeId}-${item.id}-tab`" :key="item.id" :ref="element => setButton(item.id, element)" role="tab" :aria-selected="tab === item.id" :aria-controls="`${scopeId}-${item.id}-panel`" :tabindex="tab === item.id ? 0 : -1" :disabled="busy || materialBusy" @click="selectTab(item.id)"><component :is="item.icon" :size="15" aria-hidden="true" />{{ item.label }}</button>
      </div>
      <button ref="moreButton" class="button quiet requirement-more-trigger" aria-haspopup="menu" :aria-expanded="moreOpen" :aria-controls="`${scopeId}-more`" :disabled="busy || materialBusy" @click="moreOpen ? closeMore() : openMore()" @keydown.down.prevent="openMore()" @keydown.esc.prevent="closeMore(true)">{{ t('requirement.moreSections') }}<ChevronDown :size="14" /></button>
      <div v-if="moreOpen" :id="`${scopeId}-more`" class="requirement-more-menu" role="menu" :aria-label="t('requirement.moreSections')" @keydown="navigateMenu">
        <button v-for="item in secondaryTabs" :key="item.id" :ref="element => setMenuButton(item.id, element)" role="menuitemradio" :aria-checked="tab === item.id" :disabled="busy || materialBusy" @click="selectTab(item.id, true)"><component :is="item.icon" :size="16" aria-hidden="true" />{{ item.label }}</button>
      </div>
    </div>
    <div :id="`${scopeId}-${tab}-panel`" class="requirement-tab-panel" :class="{ 'requirement-chat-panel': tab === 'chat' }" role="tabpanel" :aria-labelledby="`${scopeId}-${tab}-tab`" tabindex="0">
      <LoopDesignPrototype v-if="prototypeEnabled && LoopDesignPrototype" v-show="tab === 'loopDesign'" :inert="tab !== 'loopDesign'" :requirement="requirement" :project="project" />
      <RequirementWorkflow v-if="tab === 'decisions'" :detail="flow.value.value" :busy="flow.busy.value || flow.loading.value" :error="flow.error.value" :act="flow.act" view="grill" @navigate="selectTab" />
      <RequirementLoop v-if="tab === 'loopDesign' && !prototypeEnabled" :detail="flow.value.value" :session-state="workflowSessionState" @conversation="selectTab('chat')" />
      <KeepAlive><RequirementMaterials v-if="tab !== 'chat' && tab !== 'gitBranch' && !(prototypeEnabled && tab === 'loopDesign')" :requirement="requirement" :project="project" :kind="tab" :empty="tabs.find(item => item.id === tab)?.empty ?? ''" :refresh-key="refreshKey + (flow.value.value.workflow?.revision ?? 0)" :writers="writers" @generate="openGeneration" @busy="materialBusy = $event" /></KeepAlive>
      <RequirementGit v-if="gitVisited" v-show="tab === 'gitBranch'" :inert="tab !== 'gitBranch'" :requirement="requirement" :project="project" :requirements="requirements" :busy="busy" :refresh="refresh" @busy="materialBusy = $event" />
      <RequirementChat ref="chat" @manage-models="emit('manageModels')" :visible="tab === 'chat' && visible !== false" v-show="tab === 'chat'" :inert="tab !== 'chat'" :requirement="requirement" :project="project" :section="contextTab" :section-label="tabs.find(item => item.id === contextTab)?.label ?? ''" :workflow="flow.value.value" :workflow-busy="flow.busy.value || flow.loading.value" :workflow-error="flow.error.value" :workflow-action="flow.act" @navigate="selectTab" @settled="refreshKey++; flow.refresh()" />
    </div>
    </div>
  </section>
</template>

<style scoped>
.loop-prototype-layout > .requirement-content { padding: 22px 24px 80px; }
.loop-prototype-layout .content-tabs { margin-bottom: 20px; }
</style>
