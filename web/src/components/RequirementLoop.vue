<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'
import { Workflow, Sparkles, ArrowRight, Minus, Plus, Crosshair, Check, Pause, AlertCircle } from '@lucide/vue'
import { useI18n } from '../i18n'
import { loopLayout } from '../composables/loopLayout'
import { loopProgress, preparationStages } from '../composables/loopProgress'
import type { PreparationStage, LoopVisualState } from '../composables/loopProgress'
import { useReducedMotion } from '../composables/useReducedMotion'
import type { LoopPlan, WorkflowDetail } from '../types'

const props = defineProps<{ detail: WorkflowDetail; sessionState?: string }>()
const emit = defineEmits<{ conversation: [] }>()
const { t } = useI18n()
const root = ref<HTMLElement>()
const canvas = ref<HTMLElement>()
const selected = ref('')
const mode = ref<'progress' | 'plan'>('progress')
const effects = ref(true)
const follow = ref(true)
const zoom = ref(1)
const reduced = useReducedMotion()
const inView = ref(false)
const pageVisible = ref(!document.hidden)
const motion = computed(() => effects.value && !reduced.value && inView.value && pageVisible.value)
const markerId = `loop-arrow-${useId().replaceAll(':', '')}`
const progress = computed(() => loopProgress(props.detail.workflow, props.sessionState))
const progressView = computed(() => mode.value === 'progress' || !props.detail.plan)
const titles = computed(() => ({ requirements: t('workflow.requirements'), design: t('workflow.design'), grill: t('requirement.decisions'), alignment: t('workflow.alignment'), confirmation: t('workflow.confirmation'), loop: t('requirement.loopDesign') }))
const progressPlan = computed<LoopPlan>(() => ({
  schemaVersion: 1, title: t('loop.progressTitle'), sourceHashes: {},
  nodes: preparationStages.map(stage => ({ id: stage, title: titles.value[stage], kind: stage === 'confirmation' ? 'check' : 'task', description: t(`loop.stage.${stage}`), acceptanceIds: [], outputs: [t(`loop.output.${stage}`)], checks: [], maxAttempts: 1 })),
  edges: preparationStages.slice(1).map((stage, index) => ({ from: preparationStages[index]!, to: stage, condition: 'success' })),
}))
const displayedPlan = computed(() => progressView.value ? progressPlan.value : props.detail.plan!)
const layout = computed(() => loopLayout(displayedPlan.value))
const node = computed(() => displayedPlan.value.nodes.find(node => node.id === selected.value))
const currentId = computed(() => progressView.value ? progress.value.current : null)
const currentTitle = computed(() => progress.value.current ? titles.value[progress.value.current] : t('loop.awaitingStart'))
function nodeState(id: string): LoopVisualState { return progressView.value ? progress.value.states.get(id as PreparationStage) ?? 'pending' : 'pending' }
function edgeState(from: string, to: string) {
  if (!progressView.value) return 'planned'
  if (nodeState(from) === 'completed' && nodeState(to) === 'completed') return 'completed'
  return to === currentId.value && nodeState(from) === 'completed' ? 'active' : 'pending'
}
function flowingEdge(to: string) { return motion.value && progressView.value && progress.value.flowing && to === currentId.value }
function centerCurrent(smooth = true) {
  const item = layout.value.nodes.find(node => node.id === currentId.value)
  if (!item || !canvas.value) return
  canvas.value.scrollTo({ left: Math.max(0, (item.x + 112) * zoom.value - canvas.value.clientWidth / 2), top: Math.max(0, (item.y + 71) * zoom.value - canvas.value.clientHeight / 2), behavior: smooth && motion.value ? 'smooth' : 'auto' })
}
function locateCurrent() {
  mode.value = 'progress'
  selected.value = progress.value.current ?? ''
  void nextTick(() => centerCurrent())
}
function selectMode(value: 'progress' | 'plan') { mode.value = value }
watch(() => props.detail.plan ? `${props.detail.workflow?.id}:${props.detail.workflow?.requestId}` : '', value => { if (value) mode.value = 'plan' }, { immediate: true })
watch([progressView, () => progress.value.current], ([isProgress, id], previous) => {
  if (isProgress && id) {
    if (!selected.value || !previous?.[0] || selected.value === previous?.[1]) selected.value = id
    if (follow.value) void nextTick(() => centerCurrent())
  } else if (!displayedPlan.value.nodes.some(node => node.id === selected.value)) selected.value = displayedPlan.value.nodes.find(node => node.kind === 'task')?.id ?? displayedPlan.value.nodes[0]?.id ?? ''
}, { immediate: true })
watch(zoom, () => { if (progressView.value && follow.value) void nextTick(() => centerCurrent(false)) })
function visibilityChanged() { pageVisible.value = !document.hidden }
let observer: IntersectionObserver | undefined
let canvasObserver: ResizeObserver | undefined
onMounted(() => {
  observer = new IntersectionObserver(entries => { inView.value = entries.some(entry => entry.isIntersecting) })
  if (root.value) observer.observe(root.value)
  canvasObserver = new ResizeObserver(() => { if (progressView.value && follow.value) centerCurrent(false) })
  if (canvas.value) canvasObserver.observe(canvas.value)
  document.addEventListener('visibilitychange', visibilityChanged)
  if (follow.value) centerCurrent(false)
})
onBeforeUnmount(() => { observer?.disconnect(); canvasObserver?.disconnect(); document.removeEventListener('visibilitychange', visibilityChanged) })
</script>

<template>
  <section ref="root" class="real-loop" :class="{ 'loop-effects': motion, 'progress-waiting': ['waiting', 'approval', 'answers', 'confirmation'].includes(progress.status), 'progress-failed': progress.status === 'failed', 'progress-paused': ['paused', 'pausing', 'unknown'].includes(progress.status) }" :aria-label="t('requirement.loopDesign')">
    <header><div><span class="loop-eyebrow"><Workflow :size="14" />{{ progressView ? t('loop.liveBadge') : t('loop.previewBadge') }}</span><h2>{{ displayedPlan.title }}</h2><p>{{ progressView ? t('loop.liveHint') : t('loop.planNotStarted') }}</p></div><button class="button" @click="emit('conversation')">{{ t('workflow.openConversation') }}<ArrowRight :size="14" /></button></header>
    <div class="loop-live" :class="progress.status" role="status" aria-live="polite"><div><span class="loop-live-mark"></span><div><strong>{{ t('loop.currentStep', { step: currentTitle }) }}</strong><small>{{ t(`loop.status.${progress.status}`) }}</small></div></div><div class="loop-completion"><span>{{ t('loop.completedSteps', { count: progress.completed, total: preparationStages.length }) }}</span><div class="loop-progress-track" role="progressbar" :aria-label="t('loop.progressTitle')" :aria-valuenow="progress.completed" :aria-valuemin="0" :aria-valuemax="preparationStages.length"><i :style="{ width: `${progress.completed / preparationStages.length * 100}%` }"></i></div></div></div>
    <p v-if="detail.stale && !['pending', 'running'].includes(detail.workflow?.state ?? '')" class="form-error" role="status">{{ t('workflow.stale') }}</p>
    <p v-if="detail.warning" class="form-error" role="status">{{ t('loop.unavailable') }}</p>
    <div v-if="detail.plan" class="loop-view-switch" role="group" :aria-label="t('loop.view')"><button :aria-pressed="progressView" @click="selectMode('progress')">{{ t('loop.progressView') }}</button><button :aria-pressed="!progressView" @click="selectMode('plan')">{{ t('loop.planView') }}</button></div>
    <div class="loop-body">
      <div class="loop-canvas-wrap"><div class="loop-toolbar"><span>{{ t('loop.nodeCount', { count: layout.nodes.length }) }}</span><div><button class="button loop-locate" :disabled="!progress.current" @click="locateCurrent"><Crosshair :size="13" />{{ t('loop.locate') }}</button><button class="icon-button" :aria-label="t('loop.zoomOut')" :disabled="zoom <= .5" @click="zoom = Math.max(.5, zoom - .1)"><Minus :size="15" /></button><span>{{ Math.round(zoom * 100) }}%</span><button class="icon-button" :aria-label="t('loop.zoomIn')" :disabled="zoom >= 1.5" @click="zoom = Math.min(1.5, zoom + .1)"><Plus :size="15" /></button><label v-if="progressView"><input v-model="follow" type="checkbox" @change="follow && centerCurrent()" />{{ t('loop.follow') }}</label><label><input v-model="effects" type="checkbox" />{{ t('loop.effects') }}</label></div></div>
        <div ref="canvas" class="loop-canvas" :aria-label="t('loop.canvas')"><div :style="{ width: `${layout.width * zoom}px`, height: `${layout.height * zoom}px` }"><div class="loop-world" :style="{ width: `${layout.width}px`, height: `${layout.height}px`, transform: `scale(${zoom})` }">
          <svg :width="layout.width" :height="layout.height" aria-hidden="true">
            <defs>
              <marker :id="markerId" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto"><path d="M0,0 L8,4 L0,8" fill="context-stroke" /></marker>
              <template v-for="(edge, index) in layout.edges" :key="index">
                <linearGradient v-if="flowingEdge(edge.to)" :id="`${markerId}-reflection-${index}`" gradientUnits="userSpaceOnUse" :x1="edge.startX - 64" :y1="edge.startY" :x2="edge.startX" :y2="edge.startY">
                  <stop offset="0" stop-color="#8d98a8" stop-opacity="0" />
                  <stop offset=".25" stop-color="#b7c1ce" stop-opacity=".35" />
                  <stop offset=".5" stop-color="#e2e8f0" stop-opacity=".9" />
                  <stop offset=".65" stop-color="#ffffff" />
                  <stop offset=".78" stop-color="#d6dfe9" stop-opacity=".85" />
                  <stop offset="1" stop-color="#8d98a8" stop-opacity="0" />
                  <animateTransform attributeName="gradientTransform" type="translate" from="0 0" :to="`${edge.endX - edge.startX + 64} ${edge.endY - edge.startY}`" dur="2.8s" repeatCount="indefinite" />
                </linearGradient>
              </template>
            </defs>
            <g v-for="(edge, index) in layout.edges" :key="`${edge.from}-${edge.to}-${edge.condition}`">
              <path class="loop-route" :d="edge.path" :class="[edgeState(edge.from, edge.to), { feedback: edge.condition === 'failure', flowing: flowingEdge(edge.to), inspected: !progressView && (edge.from === selected || edge.to === selected) }]" :marker-end="`url(#${markerId})`" />
              <template v-if="flowingEdge(edge.to)">
                <path class="loop-reflection bloom" :d="edge.path" :stroke="`url(#${markerId}-reflection-${index})`" />
                <path class="loop-reflection" :d="edge.path" :stroke="`url(#${markerId}-reflection-${index})`" />
              </template>
            </g>
          </svg>
          <button v-for="(item, index) in layout.nodes" :key="item.id" class="loop-node" :class="[item.kind, nodeState(item.id), { selected: selected === item.id, 'is-current': currentId === item.id }]" :style="{ left: `${item.x}px`, top: `${item.y}px` }" :aria-pressed="selected === item.id" :aria-current="currentId === item.id ? 'step' : undefined" @click="selected = item.id"><span v-if="currentId === item.id" class="loop-current-label">{{ progress.status === 'ready' ? t('loop.ready') : t('loop.current') }}</span><span class="loop-node-kind">{{ String(index + 1).padStart(2, '0') }} · {{ progressView ? t('loop.preparationNode') : t(`loop.kind.${item.kind}`) }}</span><strong>{{ item.title }}</strong><small>{{ progressView ? t(`loop.output.${item.id as PreparationStage}`) : item.acceptanceIds?.join(' · ') || t('loop.flowControl') }}</small><span class="loop-node-state"><Check v-if="nodeState(item.id) === 'completed'" :size="12" /><Pause v-else-if="nodeState(item.id) === 'paused'" :size="12" /><AlertCircle v-else-if="['waiting', 'failed', 'unknown'].includes(nodeState(item.id))" :size="12" /><i v-else></i>{{ currentId === item.id ? t(`loop.status.${progress.status}`) : t(`loop.nodeState.${nodeState(item.id)}`) }}</span></button>
        </div></div></div>
        <div class="loop-legend"><span v-if="progressView"><i class="active" />{{ t('loop.current') }}</span><span><i class="completed" />{{ progressView ? t('loop.completed') : t('loop.success') }}</span><span><i :class="progressView ? 'pending' : 'feedback'" />{{ progressView ? t('loop.notStarted') : t('loop.feedback') }}</span><span>{{ t('loop.scrollHint') }}</span></div>
      </div>
      <aside v-if="node" class="loop-inspector"><span class="loop-eyebrow"><Sparkles :size="13" />{{ t('loop.nodeDetails') }}</span><h3>{{ node.title }}</h3><span class="loop-inspector-state" :class="nodeState(node.id)">{{ currentId === node.id ? t(`loop.status.${progress.status}`) : t(`loop.nodeState.${nodeState(node.id)}`) }}</span><p>{{ node.description }}</p><template v-if="!progressView"><h4>{{ t('loop.acceptance') }}</h4><p>{{ node.acceptanceIds?.join(' · ') || t('loop.flowControl') }}</p></template><h4>{{ t('loop.outputs') }}</h4><ul><li v-for="output in node.outputs" :key="output">{{ output }}</li></ul><template v-if="!progressView"><h4>{{ t('loop.checks') }}</h4><ul><li v-for="check in node.checks" :key="check">{{ check }}</li></ul><p class="loop-budget">{{ t('loop.attempts', { count: node.maxAttempts }) }}</p><p v-for="edge in displayedPlan.edges.filter(edge => edge.from === node?.id && edge.condition === 'failure')" :key="edge.to" class="loop-budget">{{ t('loop.retryBudget', { target: displayedPlan.nodes.find(item => item.id === edge.to)?.title ?? edge.to, count: edge.maxTraversals ?? 0 }) }}</p></template><button v-if="progressView && currentId === node.id && ['waiting', 'failed', 'paused', 'unknown'].includes(nodeState(node.id))" class="button" @click="emit('conversation')">{{ t('loop.handleInChat') }}<ArrowRight :size="14" /></button></aside>
    </div>
  </section>
</template>

<style scoped>
.real-loop { padding: 18px; border: 1px solid var(--border); border-radius: 12px; margin-bottom: 24px; background: var(--surface-subtle); --loop-active: #626d7a; --loop-metal: #a5afbc; --loop-metal-soft: #e8edf3; --loop-done: var(--success); --loop-wait: var(--warning); --loop-error: var(--error); }
:global(:root[data-theme='dark'] .real-loop) { --loop-active: #c1c9d3; --loop-metal: #c1c9d3; --loop-metal-soft: #e8edf3; }
.real-loop > header { display: flex; align-items: center; justify-content: space-between; gap: 18px; margin-bottom: 18px; }
.loop-eyebrow { display: flex; align-items: center; gap: 7px; font-size: 12px; color: var(--muted); }
.real-loop h2 { margin: 10px 0 8px; font-size: 22px; font-weight: 600; }
.real-loop header p { max-width: 600px; margin: 0; font-size: 12px; line-height: 1.7; color: var(--muted); }
.loop-live { display: flex; align-items: center; justify-content: space-between; gap: 18px; padding: 14px 16px; margin-bottom: 16px; border: 1px solid var(--border); border-radius: 10px; background: var(--surface); }
.loop-live > div:first-child { display: flex; align-items: center; gap: 11px; }
.loop-live strong, .loop-live small { display: block; }
.loop-live strong { font-size: 13px; font-weight: 600; }
.loop-live small { margin-top: 5px; color: var(--muted); font-size: 11px; }
.loop-live-mark { width: 3px; height: 24px; flex-shrink: 0; border-radius: 3px; background: var(--muted); }
.loop-live.running .loop-live-mark, .loop-live.validating .loop-live-mark { background: linear-gradient(180deg, var(--loop-metal-soft), var(--loop-metal) 50%, var(--loop-active)); box-shadow: 0 0 10px color-mix(in srgb, var(--loop-metal) 22%, transparent); }
.loop-live.ready .loop-live-mark { background: var(--loop-done); }
.loop-live.waiting .loop-live-mark, .loop-live.answers .loop-live-mark, .loop-live.approval .loop-live-mark, .loop-live.confirmation .loop-live-mark { background: var(--loop-wait); }
.loop-live.failed .loop-live-mark { background: var(--loop-error); }
.loop-completion { width: 160px; flex-shrink: 0; color: var(--muted); font-size: 12px; }
.loop-progress-track { height: 4px; border-radius: 4px; background: var(--border); margin-top: 9px; overflow: hidden; }
.loop-progress-track i { display: block; height: 100%; background: var(--loop-done); transition: width .35s ease; }
.loop-view-switch { display: inline-flex; gap: 4px; padding: 4px; margin-bottom: 14px; border: 1px solid var(--border); border-radius: 9px; }
.loop-view-switch button { background: transparent; border: 0; border-radius: 6px; font: inherit; font-size: 12px; padding: 7px 10px; color: var(--muted); }
.loop-view-switch button[aria-pressed='true'] { background: var(--surface-raised); color: var(--text); box-shadow: 0 1px 5px #00000012; }
.loop-body { display: grid; grid-template-columns: minmax(0, 1fr) 260px; border: 1px solid var(--border); border-radius: 12px; overflow: hidden; background: var(--surface); }
.loop-canvas-wrap { min-width: 0; }
.loop-toolbar { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: 10px; padding: 10px 14px; border-bottom: 1px solid var(--border); font-size: 12px; color: var(--muted); }
.loop-toolbar > div { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.loop-toolbar label { display: flex; align-items: center; gap: 5px; white-space: nowrap; }
.loop-locate { padding: 6px 8px; font-size: 12px; }
.loop-canvas { height: 360px; overflow: auto; background-image: radial-gradient(var(--border-strong) 1px, transparent 1px); background-size: 20px 20px; }
.loop-world { position: relative; transform-origin: 0 0; }
.loop-world svg { position: absolute; top: 0; left: 0; overflow: visible; }
.loop-route { fill: none; stroke: var(--border-strong); stroke-width: 1.5; }
.loop-route.pending { stroke-dasharray: 4 6; }
.loop-route.planned { stroke: #91a59e; }
.loop-route.completed { stroke: var(--loop-done); stroke-width: 2; }
.loop-route.active { stroke: var(--loop-active); stroke-width: 3; stroke-dasharray: none; }
.progress-waiting .loop-route.active { stroke: var(--loop-wait); }
.progress-failed .loop-route.active { stroke: var(--loop-error); }
.progress-paused .loop-route.active { stroke: #899ab0; }
.loop-route.feedback { stroke: #c39462; stroke-dasharray: 5 5; }
.loop-route.inspected { stroke-width: 2.5; }
.loop-route.flowing { filter: drop-shadow(0 0 3px color-mix(in srgb, var(--loop-metal) 35%, transparent)); }
.loop-reflection { fill: none; stroke-width: 3; pointer-events: none; }
.loop-reflection.bloom { stroke-width: 9; opacity: .45; filter: blur(3px); }
.loop-node { position: absolute; display: flex; flex-direction: column; width: 224px; height: 142px; text-align: left; border: 1px solid var(--border-strong); border-radius: 12px; padding: 14px; background: var(--surface-raised); box-shadow: 0 5px 16px #00000008; color: var(--text); cursor: pointer; transition: border-color .25s, box-shadow .25s; }
.loop-node.pending { color: var(--muted); }
.loop-node.selected { outline: 2px dashed color-mix(in srgb, var(--text) 55%, transparent); outline-offset: 4px; }
.loop-node:focus-visible { outline: 3px solid var(--focus); outline-offset: 5px; }
.loop-node.is-current { border: 2px solid var(--loop-active); box-shadow: 0 0 0 2px color-mix(in srgb, var(--loop-metal) 10%, transparent); z-index: 2; }
.loop-node.is-current.waiting { border-color: var(--loop-wait); }
.loop-node.is-current.failed { border-color: var(--loop-error); }
.loop-node.is-current.paused, .loop-node.is-current.unknown { border-color: var(--muted); }
.loop-node.completed { border-color: color-mix(in srgb, var(--loop-done) 65%, var(--border)); }
.loop-current-label { position: absolute; top: -29px; left: 50%; transform: translateX(-50%); padding: 4px 10px; border: 1px solid currentColor; border-radius: 6px; background: var(--surface-raised); color: var(--loop-active); font-size: 12px; font-weight: 600; white-space: nowrap; }
.loop-node.waiting .loop-current-label { color: var(--loop-wait); }
.loop-node.failed .loop-current-label { color: var(--loop-error); }
.loop-node.paused .loop-current-label, .loop-node.unknown .loop-current-label { color: var(--muted); }
.loop-node strong { font-size: 14px; line-height: 1.4; margin: 9px 0 7px; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
.loop-node-kind, .loop-node small { color: var(--muted); font-size: 12px; }
.loop-node small { overflow: hidden; white-space: nowrap; text-overflow: ellipsis; max-width: 100%; }
.loop-node-state { display: flex; align-items: center; gap: 6px; margin-top: auto; padding-top: 7px; color: var(--muted); font-size: 12px; }
.loop-node-state i { width: 5px; height: 5px; border-radius: 50%; background: currentColor; }
.loop-node.running .loop-node-state { color: var(--loop-active); }
.loop-node.completed .loop-node-state { color: var(--loop-done); }
.loop-node.waiting .loop-node-state { color: var(--loop-wait); }
.loop-node.failed .loop-node-state { color: var(--loop-error); }
.loop-legend { display: flex; flex-wrap: wrap; gap: 16px; padding: 12px 14px; border-top: 1px solid var(--border); font-size: 12px; color: var(--muted); }
.loop-legend span { display: flex; align-items: center; gap: 6px; }
.loop-legend i { display: inline-block; width: 18px; border-top: 2px solid var(--border-strong); }
.loop-legend i.active { border-color: var(--loop-active); }
.loop-legend i.completed { border-color: var(--loop-done); }
.loop-legend i.feedback { border-color: #c39462; border-top-style: dashed; }
.loop-legend i.pending { border-top-style: dashed; }
.loop-inspector { padding: 20px; border-left: 1px solid var(--border); max-height: 490px; overflow: auto; }
.loop-inspector h3 { font-size: 16px; margin: 12px 0; }
.loop-inspector-state { display: inline-block; padding: 4px 8px; border: 1px solid var(--border); border-radius: 20px; font-size: 12px; color: var(--muted); }
.loop-inspector-state.running { color: var(--loop-active); }
.loop-inspector-state.completed { color: var(--loop-done); }
.loop-inspector-state.waiting { color: var(--loop-wait); }
.loop-inspector h4 { margin: 20px 0 8px; font-size: 12px; color: var(--muted); }
.loop-inspector p, .loop-inspector li { font-size: 13px; line-height: 1.7; overflow-wrap: anywhere; }
.loop-inspector ul { margin: 0; padding-left: 16px; }
.loop-inspector > .button { margin-top: 18px; }
.loop-budget { padding-top: 12px; border-top: 1px solid var(--border); color: var(--muted); }
@media (prefers-reduced-motion: reduce) { .loop-progress-track i, .loop-node { transition: none; } }
@media (max-width: 1100px) { .loop-body { grid-template-columns: minmax(0, 1fr); } .loop-inspector { border-left: 0; border-top: 1px solid var(--border); max-height: 280px; } }
@media (max-width: 700px) { .real-loop { padding: 14px; } .real-loop > header { flex-direction: column; align-items: flex-start; } .loop-live { flex-wrap: wrap; } .loop-completion { width: 100%; } .loop-canvas { height: 310px; } }
</style>
