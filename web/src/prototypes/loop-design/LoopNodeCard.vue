<script setup lang="ts">
import { computed } from 'vue'
import { Files, Split, Code2, FlaskConical, ScanSearch, GitMerge, Wrench, Flag, Check, LoaderCircle, Circle, AlertCircle, Pause } from '@lucide/vue'
import { useI18n } from '../../i18n'
import type { LoopNode, NodeState } from './demo'
const props = defineProps<{ node: LoopNode; state: NodeState; selected: boolean; compact?: boolean }>()
defineEmits<{ select: [] }>()
const { t } = useI18n()
const icons = { sources: Files, plan: Split, build: Code2, tests: FlaskConical, review: ScanSearch, gate: GitMerge, repair: Wrench, delivery: Flag }
const statusIcon = computed(() => props.state === 'passed' ? Check : props.state === 'running' ? LoaderCircle : props.state === 'failed' || props.state === 'blocked' ? AlertCircle : props.state === 'waiting' ? Pause : Circle)
</script>

<template>
  <button class="loop-node" :class="[state, { selected, compact }]" :aria-label="`${t(node.title)} · ${t(`loopPrototype.state.${state}`)}`" :aria-pressed="selected" @click="$emit('select')" @pointerdown.stop>
    <div class="loop-node-top"><span class="loop-node-icon"><component :is="icons[node.id]" :size="17" /></span><span class="loop-node-role">{{ t(`loopPrototype.role.${node.role}`) }}</span><component :is="statusIcon" class="loop-node-status-icon" :size="14" /></div>
    <strong>{{ t(node.title) }}</strong>
    <p v-if="!compact">{{ t(node.description) }}</p>
    <div class="loop-node-bottom"><span class="loop-state-dot"></span>{{ t(`loopPrototype.state.${state}`) }}</div>
  </button>
</template>
