<script setup lang="ts">
import { computed, ref } from 'vue'
import { useWorkflowAnswers } from '../composables/workflowAnswers'
import { Check, LoaderCircle, Sparkles, ArrowRight } from '@lucide/vue'
import { useI18n } from '../i18n'
import type { MaterialKind, WorkflowAction, WorkflowDetail } from '../types'
import StreamingMarkdown from './StreamingMarkdown.vue'

const props = defineProps<{ detail: WorkflowDetail; busy: boolean; error?: unknown; act: (input: WorkflowAction) => Promise<boolean>; view: 'chat' | 'grill'; brief?: string; endpointId?: string; canStart?: boolean }>()
const emit = defineEmits<{ navigate: [value: MaterialKind | 'chat'] }>()
const { t, errorMessage } = useI18n()
const workflow = computed(() => props.detail.workflow)
const active = computed(() => ['pending', 'running'].includes(workflow.value?.state ?? ''))
const revision = ref('')
const { answers, save: saveAnswers, clear: clearAnswers } = useWorkflowAnswers(() => workflow.value?.id ?? '')
const steps = computed(() => [
  { stage: 'requirements', label: t('workflow.requirements'), tab: 'documents' as const },
  { stage: 'design', label: t('workflow.design'), tab: 'designDocuments' as const },
  { stage: 'grill', label: t('requirement.decisions'), tab: 'decisions' as const },
  { stage: 'alignment', label: t('workflow.alignment'), tab: 'chat' as const },
  { stage: 'confirmation', label: t('workflow.confirmation'), tab: 'chat' as const },
  { stage: 'loop', label: t('requirement.loopDesign'), tab: 'loopDesign' as const },
])
const currentStep = computed(() => workflow.value?.state === 'review' ? 4 : steps.value.findIndex(step => step.stage === workflow.value?.stage))
const answered = computed(() => workflow.value?.questions.every(question => answers.value[question.id]?.trim()))
async function submitAnswers() {
  const body: Record<string, string> = {}
  for (const question of workflow.value?.questions ?? []) body[question.id] = answers.value[question.id] ?? ''
  if (await props.act({ action: 'answers', answers: body })) clearAnswers()
}
async function revise() { if (await props.act({ action: 'revise', brief: revision.value })) revision.value = '' }
</script>

<template>
  <section class="preparation" :aria-label="t('workflow.title')" :aria-busy="busy">
    <header><div><span class="preparation-eyebrow"><Sparkles :size="13" aria-hidden="true" />{{ t('workflow.eyebrow') }}</span><h3>{{ t('workflow.title') }}</h3></div><span v-if="workflow" class="preparation-state">{{ t(`workflow.state.${workflow.state}`) }}</span></header>
    <ol class="preparation-steps"><li v-for="(step, index) in steps" :key="step.stage" :class="{ current: index === currentStep, done: index < currentStep || workflow?.state === 'preview' }"><button :aria-current="index === currentStep ? 'step' : undefined" @click="emit('navigate', step.tab)"><span><Check v-if="index < currentStep || workflow?.state === 'preview'" :size="12" /><template v-else>{{ index + 1 }}</template></span>{{ step.label }}</button></li></ol>
    <p v-if="!workflow || workflow.state === 'cancelled'" class="subtle">{{ t('workflow.startHint') }}</p>
    <p v-if="active" class="preparation-running" role="status"><LoaderCircle :size="15" class="preparation-spinner" aria-hidden="true" />{{ t('workflow.runningHint') }}</p>
    <p v-if="detail.stale && !active" class="form-error" role="status">{{ t('workflow.stale') }}</p>
    <p v-if="workflow?.error" class="form-error" role="alert">{{ t('workflow.failedHint', { reason: workflow.error }) }}</p>
    <p v-if="error" class="form-error" role="alert">{{ errorMessage(error, 'workflow.actionError') }}</p>
    <form v-if="workflow?.state === 'questions'" class="preparation-questions" @submit.prevent="submitAnswers">
      <p>{{ t('workflow.questionsHint') }}</p>
      <div v-for="(question, index) in workflow.questions" :key="question.id"><label :for="`grill-${view}-${workflow.id}-${question.id}`">{{ index + 1 }}. {{ question.question }}</label><small>{{ question.reason }}</small><textarea :id="`grill-${view}-${workflow.id}-${question.id}`" v-model="answers[question.id]" class="text-field" rows="3" maxlength="4000" :disabled="busy" required @input="saveAnswers"></textarea></div>
      <button type="submit" class="button primary" :disabled="busy || !answered || detail.stale">{{ t('workflow.submitAnswers') }}<ArrowRight :size="14" /></button>
    </form>
    <div v-if="workflow?.state === 'review'" class="preparation-review">
      <p>{{ t('workflow.reviewHint') }}</p>
      <details v-for="document in detail.documents" :key="document.path"><summary>{{ document.path }}</summary><StreamingMarkdown :text="document.content" status="completed" external-links-only /></details>
      <button class="button primary" :disabled="busy || detail.stale || detail.documents.length !== 3" @click="act({ action: 'confirm', reviewHash: workflow.reviewHash })">{{ t('workflow.confirm') }}<ArrowRight :size="14" /></button>
    </div>
    <div v-if="workflow?.state === 'preview'" class="preparation-preview"><p>{{ t('workflow.previewHint') }}</p><button class="button primary" @click="emit('navigate', 'loopDesign')">{{ t('workflow.openPreview') }}<ArrowRight :size="14" /></button></div>
    <form v-if="workflow && ['review', 'preview', 'failed'].includes(workflow.state) && view === 'chat'" class="preparation-revise" @submit.prevent="revise"><label :for="`revision-${workflow.id}`">{{ t('workflow.reviseLabel') }}</label><div><input :id="`revision-${workflow.id}`" v-model="revision" class="text-field" maxlength="32768" :placeholder="t('workflow.revisePlaceholder')" :disabled="busy" /><button class="button" :disabled="busy || !revision.trim()">{{ t('workflow.revise') }}</button></div></form>
    <footer>
      <button v-if="workflow?.state === 'cancelled' && workflow.pausedState" class="button primary" :disabled="busy || detail.stale" @click="act({ action: 'resume' })">{{ t('workflow.resume') }}<ArrowRight :size="14" /></button>
      <button v-if="view === 'chat' && !active && workflow?.state !== 'questions' && workflow?.state !== 'review'" class="button" :class="{ primary: !workflow || workflow.state === 'cancelled' }" :disabled="busy || !canStart" @click="act({ action: 'start', brief, endpointId })"><Sparkles :size="14" />{{ workflow && workflow.state !== 'cancelled' ? t('workflow.restart') : t('workflow.start') }}</button>
      <button v-if="workflow?.state === 'failed'" class="button" :disabled="busy || detail.stale" @click="act({ action: 'retry' })">{{ t('workflow.retry') }}</button>
      <button v-if="workflow && ['pending', 'running', 'questions', 'review'].includes(workflow.state)" class="button" :disabled="busy" @click="act({ action: 'cancel' })">{{ t('workflow.cancel') }}</button>
      <button v-if="view === 'grill' && (!workflow || workflow.state !== 'questions')" class="button" @click="emit('navigate', 'chat')">{{ t('workflow.openConversation') }}</button>
    </footer>
  </section>
</template>

<style scoped>
.preparation { margin: 0 0 20px; padding: 20px; border: 1px solid var(--border); border-radius: 14px; background: var(--surface-subtle); font-size: 13px; }
.preparation header { display: flex; gap: 12px; align-items: center; justify-content: space-between; }
.preparation-eyebrow { display: flex; align-items: center; gap: 6px; color: var(--muted); font-size: 10px; letter-spacing: .12em; }
.preparation h3 { margin: 8px 0 0; font-size: 17px; font-weight: 600; }
.preparation-state { padding: 5px 10px; border: 1px solid var(--border); border-radius: 20px; color: var(--muted); font-size: 11px; white-space: nowrap; }
.preparation-steps { display: flex; flex-wrap: wrap; gap: 6px 14px; padding: 0; margin: 18px 0; list-style: none; }
.preparation-steps button { display: flex; align-items: center; gap: 6px; background: transparent; border: 0; padding: 4px 0; font: inherit; font-size: 11px; color: var(--muted); }
.preparation-steps span { display: grid; place-items: center; width: 21px; height: 21px; border: 1px solid var(--border); border-radius: 50%; }
.preparation-steps .current button { color: var(--text); font-weight: 600; }
.preparation-steps .current span { background: var(--accent); color: var(--surface); border-color: var(--accent); }
.preparation-steps .done span { color: #22856c; border-color: #77b7a0; }
.preparation p { line-height: 1.7; }
.preparation-running { display: flex; align-items: center; gap: 8px; color: var(--muted); }
.preparation-spinner { flex-shrink: 0; animation: preparation-spin 1.5s linear infinite; }
.preparation footer { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 14px; }
.preparation-questions { display: grid; gap: 16px; }
.preparation-questions label, .preparation-revise label { display: block; line-height: 1.6; font-weight: 600; }
.preparation-questions small { display: block; margin: 6px 0 10px; line-height: 1.6; color: var(--muted); }
.preparation-questions .button { justify-self: start; }
.preparation-review details { margin: 10px 0; padding: 12px; border: 1px solid var(--border); border-radius: 8px; background: var(--surface); }
.preparation-review summary { cursor: pointer; overflow-wrap: anywhere; }
.preparation-review .markdown { padding: 12px 0 0; max-height: 360px; overflow: auto; }
.preparation-revise { margin-top: 16px; }
.preparation-revise > div { display: flex; gap: 8px; margin-top: 8px; }
.preparation-revise input { flex: 1; min-width: 0; }
@keyframes preparation-spin { to { transform: rotate(360deg); } }
@media (prefers-reduced-motion: reduce) { .preparation-spinner { animation: none; } }
</style>
