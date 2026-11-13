<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { Minus, Plus, Maximize, MousePointer2 } from '@lucide/vue'
import { useI18n } from '../../i18n'
import LoopNodeCard from './LoopNodeCard.vue'
import type { LoopNode, NodeId, NodeState } from './demo'
import { sampleEdges } from './demo'
const props = defineProps<{ nodes: LoopNode[]; states: Record<NodeId, NodeState>; selectedId: NodeId; attempt: number }>()
defineEmits<{ select: [id: NodeId] }>()
const { t } = useI18n()
const viewport = ref<HTMLDivElement>()
const scale = ref(1)
const offset = ref({ x: 0, y: 0 })
const dragging = ref(false)
const positions: Record<NodeId, { x: number; y: number }> = {
  sources: { x: 24, y: 42 }, plan: { x: 252, y: 42 }, build: { x: 480, y: 42 },
  gate: { x: 24, y: 228 }, review: { x: 252, y: 228 }, tests: { x: 480, y: 228 },
  delivery: { x: 24, y: 424 }, repair: { x: 252, y: 424 },
}
const paths: Record<string, string> = {
  'source-plan': 'M214 102 L252 102', 'plan-build': 'M442 102 L480 102',
  'build-tests': 'M575 162 L575 228', 'build-review': 'M555 162 C555 198 347 194 347 228',
  'review-gate': 'M252 288 L214 288', 'tests-gate': 'M575 348 L575 377 Q575 387 564 387 L130 387 Q119 387 119 377 L119 348',
  'gate-delivery': 'M94 348 L94 424', 'gate-repair': 'M214 302 C241 302 223 484 252 484',
  'repair-build': 'M442 484 L680 484 Q690 484 690 473 L690 113 Q690 102 679 102 L670 102',
}
const transform = computed(() => ({ transform: `translate(${offset.value.x}px, ${offset.value.y}px) scale(${scale.value})` }))
let observer: ResizeObserver | undefined
let dragStart = { x: 0, y: 0, offsetX: 0, offsetY: 0 }
function fit() {
  const bounds = viewport.value?.getBoundingClientRect()
  if (!bounds) return
  scale.value = Math.min(1, (bounds.width - 26) / 708, (bounds.height - 54) / 558)
  offset.value = { x: (bounds.width - 708 * scale.value) / 2, y: (bounds.height - 558 * scale.value) / 2 + 12 }
}
function zoom(amount: number) { scale.value = Math.min(1.7, Math.max(.45, scale.value + amount)) }
function wheel(event: WheelEvent) {
  if (!event.ctrlKey && !event.metaKey) return
  event.preventDefault()
  zoom(event.deltaY > 0 ? -.06 : .06)
}
function startDrag(event: PointerEvent) {
  if (event.button !== 0 || (event.target as Element).closest('button')) return
  dragging.value = true
  dragStart = { x: event.clientX, y: event.clientY, offsetX: offset.value.x, offsetY: offset.value.y }
  viewport.value?.setPointerCapture(event.pointerId)
}
function moveDrag(event: PointerEvent) {
  if (dragging.value) offset.value = { x: dragStart.offsetX + event.clientX - dragStart.x, y: dragStart.offsetY + event.clientY - dragStart.y }
}
function stopDrag() { dragging.value = false }
function edgeClass(from: NodeId, to: NodeId, feedback = false) {
  if (feedback && (props.states[from] === 'failed' || props.states[to] === 'running')) return 'feedback-lit'
  if (props.states[to] === 'running' && ['passed', 'failed'].includes(props.states[from])) return 'active'
  if (!feedback && props.states[from] === 'passed' && ['passed', 'waiting'].includes(props.states[to])) return 'complete'
  return ''
}
onMounted(() => { observer = new ResizeObserver(fit); if (viewport.value) observer.observe(viewport.value); fit() })
onBeforeUnmount(() => observer?.disconnect())
</script>

<template>
  <div class="loop-canvas-variant">
    <div ref="viewport" class="loop-canvas" :class="{ dragging }" :aria-label="t('loopPrototype.canvas')" @pointerdown="startDrag" @pointermove="moveDrag" @pointerup="stopDrag" @pointercancel="stopDrag" @wheel="wheel">
      <div class="loop-canvas-caption"><span class="loop-live-dot"></span>{{ t('loopPrototype.candidate') }}<code>{{ t('loopPrototype.attempt', { count: attempt }) }}</code></div>
      <div class="loop-graph" :style="transform">
        <svg class="loop-edges" width="708" height="558" viewBox="0 0 708 558" aria-hidden="true">
          <defs><marker id="loop-arrow" markerWidth="7" markerHeight="7" refX="5" refY="3" orient="auto"><path d="M0 0 L6 3 L0 6" fill="none" stroke="context-stroke" stroke-width="1.3" /></marker></defs>
          <g v-for="edge in sampleEdges" :key="edge.id" :class="[{ feedback: edge.feedback }, edgeClass(edge.from, edge.to, edge.feedback)]">
            <path :d="paths[edge.id]" class="loop-edge-base" marker-end="url(#loop-arrow)" />
            <path :d="paths[edge.id]" class="loop-edge-flow" />
          </g>
          <text x="102" y="401" class="loop-edge-label">{{ t('loopPrototype.edge.pass') }}</text>
          <text x="254" y="400" class="loop-edge-label feedback-label">{{ t('loopPrototype.edge.fail') }}</text>
          <text x="567" y="471" class="loop-edge-label feedback-label">{{ t('loopPrototype.edge.retry') }}</text>
          <text x="434" y="207" class="loop-edge-label">{{ t('loopPrototype.parallel') }}</text>
        </svg>
        <div v-for="(node, index) in nodes" :key="node.id" class="loop-node-position" :style="{ left: `${positions[node.id].x}px`, top: `${positions[node.id].y}px`, animationDelay: `${index * 45}ms` }"><LoopNodeCard :node="node" :state="states[node.id]" :selected="selectedId === node.id" @select="$emit('select', node.id)" /></div>
      </div>
      <div class="loop-canvas-tools"><button :aria-label="t('loopPrototype.zoomOut')" @click="zoom(-.12)"><Minus :size="15" /></button><code>{{ Math.round(scale * 100) }}%</code><button :aria-label="t('loopPrototype.zoomIn')" @click="zoom(.12)"><Plus :size="15" /></button><span></span><button :aria-label="t('loopPrototype.fit')" @click="fit"><Maximize :size="15" /></button></div>
      <div class="loop-canvas-hint"><MousePointer2 :size="12" />{{ t('loopPrototype.canvasHint') }}</div>
    </div>
  </div>
</template>
