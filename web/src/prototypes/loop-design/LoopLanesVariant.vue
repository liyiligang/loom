<script setup lang="ts">
import { GitBranch, Bot, ShieldCheck, UserRound } from '@lucide/vue'
import { useI18n } from '../../i18n'
import LoopNodeCard from './LoopNodeCard.vue'
import type { LoopNode, NodeId, NodeState } from './demo'
defineProps<{ nodes: LoopNode[]; states: Record<NodeId, NodeState>; selectedId: NodeId; attempt: number }>()
defineEmits<{ select: [id: NodeId] }>()
const { t } = useI18n()
const lanes = [
  { name: 'loopPrototype.lane.prepare' as const, icon: Bot, branch: 'main', ids: ['sources', 'plan'] },
  { name: 'loopPrototype.lane.implement' as const, icon: Bot, branch: 'implement-01 / optimize-01', ids: ['build', 'repair'] },
  { name: 'loopPrototype.lane.verify' as const, icon: ShieldCheck, branch: 'candidate revision', ids: ['tests', 'review', 'gate'] },
  { name: 'loopPrototype.lane.deliver' as const, icon: UserRound, branch: 'integrate-01', ids: ['delivery'] },
]
</script>

<template>
  <div class="loop-lanes-variant">
    <div class="loop-lanes-heading"><div><h3>{{ t('loopPrototype.lanesTitle') }}</h3><p>{{ t('loopPrototype.lanesHint') }}</p></div><span class="loop-attempt-pill">{{ t('loopPrototype.attempt', { count: attempt }) }}</span></div>
    <div v-for="lane in lanes" :key="lane.name" class="loop-lane">
      <div class="loop-lane-heading"><component :is="lane.icon" :size="17" /><strong>{{ t(lane.name) }}</strong><small><GitBranch :size="11" />{{ lane.branch }}</small></div>
      <div class="loop-lane-track"><div v-for="node in nodes.filter(value => lane.ids.includes(value.id))" :key="node.id"><LoopNodeCard :node="node" :state="states[node.id]" :selected="selectedId === node.id" @select="$emit('select', node.id)" /></div></div>
    </div>
    <div class="loop-lanes-feedback"><span></span>{{ t('loopPrototype.lanesFeedback') }}<span></span></div>
  </div>
</template>
