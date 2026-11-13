<script setup lang="ts">
// Prototype only. Compare canvas, agent-lane, and focused execution layouts.
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Sparkles, Play, Pause, RotateCcw, ChevronRight, Files, Check, Workflow, SlidersHorizontal, ArrowRight, Code2, PanelRightClose, PanelRightOpen, Activity, CircleHelp, LoaderCircle, Crosshair } from '@lucide/vue'
import { useI18n } from '../../i18n'
import { useReducedMotion } from '../../composables/useReducedMotion'
import type { Project, Requirement } from '../../types'
import LoopCanvasVariant from './LoopCanvasVariant.vue'
import LoopLanesVariant from './LoopLanesVariant.vue'
import LoopFocusVariant from './LoopFocusVariant.vue'
import LoopNodeInspector from './LoopNodeInspector.vue'
import PrototypeSwitcher from './PrototypeSwitcher.vue'
import { useLoopDemo } from './demo'
import type { NodeId } from './demo'
import './prototype.css'

defineProps<{ requirement: Requirement; project?: Project }>()
const { t } = useI18n()
const router = useRouter()
const route = useRoute()
const variant = computed<'canvas' | 'lanes' | 'focus'>(() => route.query.variant === 'lanes' || route.query.variant === 'focus' ? route.query.variant : 'canvas')
function changeVariant(value: string) { void router.replace({ query: { ...route.query, variant: value } }) }
const { phase, states, selectedId, selected, nodes, generationStep, attempt, scenario, maxAttempts, follow, events, generated, passed, active, snapshot, generate, reset, run, pause, resume, advance, resolveBlocked, finish, updateTask } = useLoopDemo()
const reducedMotion = useReducedMotion()
const visible = ref(!document.hidden)
const visibilityChanged = () => { visible.value = !document.hidden }
onMounted(() => document.addEventListener('visibilitychange', visibilityChanged))
onBeforeUnmount(() => document.removeEventListener('visibilitychange', visibilityChanged))
const effects = ref(true)
const settingsOpen = ref(false)
const inspectorOpen = ref(true)
const jsonOpen = ref(false)
const busy = computed(() => ['running', 'generating', 'paused', 'waiting', 'blocked'].includes(phase.value))
const frozen = computed(() => phase.value !== 'preview')
const currentEvent = computed(() => events.value.at(-1))
const progress = computed(() => Math.round(passed.value / nodes.value.length * 100))
const generationLabels = ['loopPrototype.generate.sources', 'loopPrototype.generate.tasks', 'loopPrototype.generate.checks', 'loopPrototype.generate.graph'] as const
function select(id: NodeId) { selectedId.value = id; follow.value = false; inspectorOpen.value = true }
function toggleFollow() { follow.value = !follow.value; if (follow.value && active.value[0]) selectedId.value = active.value[0].id }
</script>

<template>
  <section class="loop-prototype" :class="{ 'loop-effects': effects && !reducedMotion && visible && phase !== 'paused' }" :aria-label="t('loopPrototype.title')">
    <div class="loop-prototype-notice"><span><CircleHelp :size="13" />{{ t('loopPrototype.notice') }}</span><span>{{ t('loopPrototype.previewFirst') }}</span></div>
    <header class="loop-studio-header">
      <div><div class="loop-eyebrow"><Workflow :size="14" />{{ t('loopPrototype.eyebrow') }}<span class="loop-revision">{{ t('loopPrototype.draftVersion') }}</span></div><h2>{{ t('loopPrototype.title') }}</h2><p>{{ t('loopPrototype.subtitle') }}</p></div>
      <div class="loop-header-actions">
        <button v-if="generated" class="loop-icon-button" :disabled="busy" :aria-label="t('loopPrototype.regenerate')" @click="generate"><RotateCcw :size="16" /></button>
        <button v-if="phase === 'empty' || phase === 'generating'" class="loop-primary-button" :disabled="phase === 'generating'" @click="generate"><LoaderCircle v-if="phase === 'generating'" :size="15" class="loop-spin" /><Sparkles v-else :size="16" />{{ t(phase === 'generating' ? 'loopPrototype.generating' : 'loopPrototype.generate') }}</button>
        <button v-else-if="phase === 'running'" class="loop-primary-button" @click="pause"><Pause :size="15" />{{ t('loopPrototype.pause') }}</button>
        <template v-else-if="phase === 'paused'"><button class="loop-secondary-button" @click="advance">{{ t('loopPrototype.step') }}</button><button class="loop-primary-button" @click="resume"><Play :size="15" />{{ t('loopPrototype.resume') }}</button></template>
        <button v-else-if="phase === 'waiting'" class="loop-primary-button" @click="finish"><Check :size="16" />{{ t('loopPrototype.finish') }}</button>
        <button v-else-if="phase === 'blocked'" class="loop-primary-button" @click="resolveBlocked"><ArrowRight :size="15" />{{ t('loopPrototype.resolve') }}</button>
        <button v-else class="loop-primary-button" @click="run"><Play :size="15" />{{ t(phase === 'completed' || phase === 'failed' ? 'loopPrototype.replay' : 'loopPrototype.run') }}</button>
      </div>
    </header>
    <div class="loop-source-strip"><div><Files :size="14" /><span>{{ t('loopPrototype.sampleRequirement') }}</span><code>requirements.md</code><span class="loop-source-version">v3</span><Check :size="12" /></div><span class="loop-source-plus">+</span><div><Files :size="14" /><span>{{ t('loopPrototype.sampleDesign') }}</span><code>design.md</code><span class="loop-source-version">v2</span><Check :size="12" /></div><span class="loop-source-count">{{ t('loopPrototype.sourceCriteria') }}</span></div>
    <div v-if="!generated" class="loop-empty" :class="{ generating: phase === 'generating' }">
      <div class="loop-orbit" aria-hidden="true"><div class="loop-orbit-ring"></div><div class="loop-orbit-ring second"></div><div class="loop-orbit-core"><Sparkles :size="29" /></div><span class="loop-orbit-point one"></span><span class="loop-orbit-point two"></span><span class="loop-orbit-point three"></span></div>
      <h3>{{ t(phase === 'generating' ? 'loopPrototype.generatingTitle' : 'loopPrototype.emptyTitle') }}</h3>
      <p>{{ t(phase === 'generating' ? 'loopPrototype.generatingHint' : 'loopPrototype.emptyHint') }}</p>
      <div v-if="phase === 'generating'" class="loop-generation-steps" role="status"><div v-for="(label, index) in generationLabels" :key="label" :class="{ done: generationStep > index, active: generationStep === index }"><Check v-if="generationStep > index" :size="13" /><LoaderCircle v-else-if="generationStep === index" :size="13" class="loop-spin" /><span v-else class="loop-step-dot"></span>{{ t(label) }}</div></div>
      <div v-else class="loop-empty-pipeline"><span>{{ t('loopPrototype.previewFlow.plan') }}</span><ChevronRight :size="14" /><span>{{ t('loopPrototype.previewFlow.build') }}</span><ChevronRight :size="14" /><span>{{ t('loopPrototype.previewFlow.verify') }}</span><span class="loop-empty-return">{{ t('loopPrototype.previewFlow.repair') }}</span></div>
      <div class="loop-empty-footer"><span>{{ t('loopPrototype.previewGuarantee') }}</span></div>
    </div>
    <template v-else>
      <div class="loop-plan-toolbar"><div class="loop-plan-status"><span class="loop-status" :class="phase">{{ t(`loopPrototype.phase.${phase}`) }}</span><span>{{ t('loopPrototype.nodeCount', { count: nodes.length }) }}</span><span class="loop-toolbar-divider"></span><span>{{ t('loopPrototype.budgetSummary', { count: maxAttempts }) }}</span></div><div class="loop-plan-tools"><button class="loop-icon-button" :class="{ active: follow }" :aria-label="t('loopPrototype.follow')" :aria-pressed="follow" @click="toggleFollow"><Crosshair :size="15" /></button><button class="loop-icon-button" :class="{ active: effects && !reducedMotion }" :aria-label="t('loopPrototype.effects')" :aria-pressed="effects && !reducedMotion" :disabled="reducedMotion" @click="effects = !effects"><Sparkles :size="15" /></button><button class="loop-icon-button" :aria-label="t('loopPrototype.settings')" :aria-expanded="settingsOpen" @click="settingsOpen = !settingsOpen"><SlidersHorizontal :size="15" /></button><button v-if="variant !== 'focus'" class="loop-icon-button" :aria-label="t('loopPrototype.inspector')" :aria-pressed="inspectorOpen" @click="inspectorOpen = !inspectorOpen"><PanelRightClose v-if="inspectorOpen" :size="15" /><PanelRightOpen v-else :size="15" /></button></div></div>
      <div v-if="settingsOpen" class="loop-demo-settings"><label>{{ t('loopPrototype.scenario') }}<select v-model="scenario" :disabled="busy"><option value="repair">{{ t('loopPrototype.scenario.repair') }}</option><option value="pass">{{ t('loopPrototype.scenario.pass') }}</option><option value="blocked">{{ t('loopPrototype.scenario.blocked') }}</option></select></label><label>{{ t('loopPrototype.maxAttempts') }}<input v-model.number="maxAttempts" type="number" min="1" max="6" :disabled="busy" /></label><button class="loop-secondary-button" @click="reset">{{ t('loopPrototype.reset') }}</button><small>{{ t('loopPrototype.settingsHint') }}</small></div>
      <div v-if="phase === 'preview'" class="loop-preview-banner"><span><Check :size="14" />{{ t('loopPrototype.ready') }}</span><span>{{ t('loopPrototype.readyHint') }}</span></div>
      <div v-else-if="phase === 'waiting'" class="loop-verdict-banner success"><Check :size="17" /><div><strong>{{ t('loopPrototype.verdictTitle') }}</strong><p>{{ t('loopPrototype.verdictHint') }}</p></div></div>
      <div v-else-if="phase === 'blocked' || phase === 'failed'" class="loop-verdict-banner attention"><CircleHelp :size="17" /><div><strong>{{ t(phase === 'blocked' ? 'loopPrototype.blockedTitle' : 'loopPrototype.budgetTitle') }}</strong><p>{{ t(phase === 'blocked' ? 'loopPrototype.blockedHint' : 'loopPrototype.budgetHint') }}</p></div></div>
      <div class="loop-visual-workspace" :class="{ 'with-inspector': inspectorOpen && variant !== 'focus' }">
        <LoopCanvasVariant v-if="variant === 'canvas'" :nodes="nodes" :states="states" :selected-id="selectedId" :attempt="attempt" @select="select" />
        <LoopLanesVariant v-else-if="variant === 'lanes'" :nodes="nodes" :states="states" :selected-id="selectedId" :attempt="attempt" @select="select" />
        <LoopFocusVariant v-else :nodes="nodes" :states="states" :selected-id="selectedId" :attempt="attempt" :readonly="frozen" @select="select" @update-task="updateTask" />
        <LoopNodeInspector v-if="inspectorOpen && variant !== 'focus'" :node="selected" :state="states[selectedId]" :readonly="frozen" @update-task="updateTask" />
      </div>
      <div class="loop-evidence-footer"><div class="loop-evidence-progress"><span>{{ t('loopPrototype.completedNodes', { passed, total: nodes.length }) }}</span><div><span :style="{ width: `${progress}%` }"></span></div></div><span><span class="loop-legend-dot passed"></span>{{ t('loopPrototype.state.passed') }}</span><span><span class="loop-legend-dot running"></span>{{ t('loopPrototype.state.running') }}</span><span><span class="loop-legend-dot failed"></span>{{ t('loopPrototype.feedbackLegend') }}</span><button class="loop-text-button" :aria-expanded="jsonOpen" @click="jsonOpen = !jsonOpen"><Code2 :size="14" />{{ t('loopPrototype.planStructure') }}</button></div>
      <div class="loop-activity" aria-live="polite"><Activity :size="14" /><span v-if="currentEvent">{{ t(currentEvent.label) }}</span><span v-else>{{ t('loopPrototype.previewActivity') }}</span><code v-if="currentEvent">{{ t('loopPrototype.attempt', { count: currentEvent.attempt }) }}</code></div>
      <details v-if="events.length" class="loop-event-log"><summary>{{ t('loopPrototype.eventLog') }}<span>{{ events.length }}</span></summary><ol><li v-for="event in events" :key="event.id"><code>{{ String(event.id).padStart(2, '0') }}</code><span>{{ t(event.label) }}</span><small>{{ t('loopPrototype.attempt', { count: event.attempt }) }}</small></li></ol></details>
      <pre v-if="jsonOpen" class="loop-plan-json">{{ JSON.stringify(snapshot, null, 2) }}</pre>
    </template>
    <PrototypeSwitcher :variant="variant" @change="changeVariant" />
  </section>
</template>
