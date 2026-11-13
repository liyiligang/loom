<script setup lang="ts">
import { computed } from 'vue'
import { ArrowRight, Check, Circle, LoaderCircle, AlertCircle, GitBranch, ShieldCheck } from '@lucide/vue'
import { useI18n } from '../../i18n'
import type { LoopNode, NodeId, NodeState } from './demo'
const props = defineProps<{ nodes: LoopNode[]; states: Record<NodeId, NodeState>; selectedId: NodeId; attempt: number; readonly: boolean }>()
defineEmits<{ select: [id: NodeId]; updateTask: [value: string] }>()
const { t } = useI18n()
const selected = computed(() => props.nodes.find(node => node.id === props.selectedId)!)
</script>

<template>
  <div class="loop-focus-variant">
    <nav class="loop-focus-rail" :aria-label="t('loopPrototype.focusNavigation')">
      <div class="loop-focus-rail-title">{{ t('loopPrototype.executionOrder') }}</div>
      <button v-for="(node, index) in nodes" :key="node.id" :class="[states[node.id], { selected: selectedId === node.id }]" :aria-pressed="selectedId === node.id" @click="$emit('select', node.id)"><span><Check v-if="states[node.id] === 'passed'" :size="14" /><LoaderCircle v-else-if="states[node.id] === 'running'" :size="14" /><AlertCircle v-else-if="['failed', 'blocked'].includes(states[node.id])" :size="14" /><Circle v-else :size="10" /></span><div><small>{{ String(index + 1).padStart(2, '0') }}</small><strong>{{ t(node.title) }}</strong></div></button>
      <p>{{ t('loopPrototype.focusFeedback') }}</p>
    </nav>
    <section class="loop-focus-main">
      <div class="loop-focus-kicker"><span>{{ t(`loopPrototype.role.${selected.role}`) }}</span><span>{{ t('loopPrototype.attempt', { count: attempt }) }}</span></div>
      <h2>{{ t(selected.title) }}</h2>
      <p class="loop-focus-description">{{ t(selected.description) }}</p>
      <div class="loop-focus-branch"><GitBranch :size="15" /><code>{{ selected.branch }}</code><span class="loop-status" :class="states[selectedId]">{{ t(`loopPrototype.state.${states[selectedId]}`) }}</span></div>
      <div class="loop-focus-contract"><div><h4>{{ t('loopPrototype.inputs') }}</h4><p v-for="input in selected.inputs" :key="input">{{ input }}</p></div><ArrowRight :size="23" /><div><h4>{{ t('loopPrototype.outputs') }}</h4><p v-for="output in selected.outputs" :key="output">{{ output }}</p></div></div>
      <label class="loop-focus-task">{{ t('loopPrototype.task') }}<textarea :value="selected.task" :readonly="readonly" :aria-label="t('loopPrototype.editTask')" spellcheck="false" @input="$emit('updateTask', ($event.target as HTMLTextAreaElement).value)"></textarea></label>
      <div class="loop-focus-acceptance"><h4><ShieldCheck :size="16" />{{ t('loopPrototype.acceptance') }}</h4><p v-for="criterion in selected.criteria" :key="criterion"><Check :size="14" />{{ criterion }}</p></div>
    </section>
  </div>
</template>
