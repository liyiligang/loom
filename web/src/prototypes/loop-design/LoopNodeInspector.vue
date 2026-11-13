<script setup lang="ts">
import { ArrowUpRight, GitBranch, Files, Target, ChevronDown, Check, ShieldCheck } from '@lucide/vue'
import { useI18n } from '../../i18n'
import type { LoopNode, NodeState } from './demo'
defineProps<{ node: LoopNode; state: NodeState; readonly: boolean }>()
defineEmits<{ updateTask: [value: string] }>()
const { t } = useI18n()
</script>

<template>
  <aside class="loop-inspector" :aria-label="t('loopPrototype.inspector')">
    <div class="loop-inspector-eyebrow"><span>{{ t('loopPrototype.nodeDetails') }}</span><ArrowUpRight :size="15" /></div>
    <h3>{{ t(node.title) }}</h3>
    <div class="loop-inspector-meta"><span class="loop-status" :class="state">{{ t(`loopPrototype.state.${state}`) }}</span><span>{{ t(`loopPrototype.role.${node.role}`) }}</span></div>
    <p class="loop-inspector-description">{{ t(node.description) }}</p>
    <div class="loop-detail-section"><h4><GitBranch :size="13" />{{ t('loopPrototype.proposedBranch') }}</h4><code>{{ node.branch }}</code></div>
    <div class="loop-detail-section"><h4><Files :size="13" />{{ t('loopPrototype.inputs') }}</h4><ul><li v-for="input in node.inputs" :key="input"><span class="loop-file-dot"></span>{{ input }}</li></ul></div>
    <div class="loop-detail-section"><h4><Target :size="13" />{{ t('loopPrototype.outputs') }}</h4><ul><li v-for="output in node.outputs" :key="output"><span class="loop-file-dot accent"></span>{{ output }}</li></ul></div>
    <details class="loop-task" open><summary>{{ t('loopPrototype.task') }}<ChevronDown :size="14" /></summary><textarea :value="node.task" :aria-label="t('loopPrototype.editTask')" :readonly="readonly" spellcheck="false" @input="$emit('updateTask', ($event.target as HTMLTextAreaElement).value)"></textarea><small>{{ t(readonly ? 'loopPrototype.frozen' : 'loopPrototype.editHint') }}</small></details>
    <div class="loop-detail-section loop-criteria"><h4><ShieldCheck :size="14" />{{ t('loopPrototype.acceptance') }}</h4><p v-for="criterion in node.criteria" :key="criterion"><Check :size="13" />{{ criterion }}</p></div>
  </aside>
</template>
