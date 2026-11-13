<script setup lang="ts">
import { reactive, computed } from 'vue'
import { ShieldCheck, MessageCircleQuestion } from '@lucide/vue'
import type { Interaction, Reply } from '../types'
import { useI18n } from '../i18n'
import AnimatedDisclosure from './AnimatedDisclosure.vue'

const props = defineProps<{ interaction: Interaction; agentName?: string; executionDetail?: Record<string, unknown>; busy: boolean; online: boolean }>()
const emit = defineEmits<{ respond: [id: string, reply: Reply] }>()
const { t, stateLabel, message } = useI18n()
const answers = reactive<Record<string, string>>({})
const pending = computed(() => props.interaction.state === 'pending')
const canAnswer = computed(() => pending.value && !props.busy && props.online)
const complete = computed(() => (props.interaction.payload.questions ?? []).every(question => answers[question.id]?.trim()))
const answerText = computed(() => {
  const answer = props.interaction.answer
  if (answer?.decision === 'accept') return t('interaction.accepted')
  if (answer?.decision === 'decline') return t('interaction.declined')
  const responses = answer?.answers as Record<string, { answers?: string[] }> | undefined
  return (props.interaction.payload.questions ?? []).map(question => `${question.question}\n${responses?.[question.id]?.answers?.join('\n') ?? ''}`).join('\n\n')
})
function sendAnswers() {
  emit('respond', props.interaction.id, { answers: Object.fromEntries((props.interaction.payload.questions ?? []).map(question => [question.id, { answers: [answers[question.id] ?? ''] }])) })
}
</script>

<template>
  <article class="interaction" :class="{ closed: !pending }">
    <div class="interaction-heading"><component :is="interaction.kind === 'question' ? MessageCircleQuestion : ShieldCheck" :size="19" /><strong>{{ t(interaction.kind === 'question' ? 'interaction.question' : 'interaction.approval', { agent: agentName ?? 'Codex' }) }}</strong><span class="status">{{ pending ? t('interaction.pending') : stateLabel(interaction.state) }}</span></div>
    <p v-if="interaction.payload.reason">{{ message(interaction.payload.reason) }}</p>
    <pre v-if="interaction.payload.command">{{ interaction.payload.command }}</pre>
    <p v-if="interaction.payload.grantRoot">{{ t('interaction.writeAccess') }} <code>{{ interaction.payload.grantRoot }}</code></p>
    <AnimatedDisclosure v-if="executionDetail" class="interaction-details"><template #summary><span>{{ t('interaction.review') }}</span></template><pre>{{ JSON.stringify(executionDetail, null, 2) }}</pre></AnimatedDisclosure>
    <Transition name="interaction-content" mode="out-in">
      <div v-if="interaction.kind === 'question' && pending" key="question">
        <fieldset v-for="question in interaction.payload.questions" :key="question.id" :disabled="!canAnswer">
          <legend>{{ question.question }}</legend>
          <label v-for="option in question.options ?? []" :key="option.label" class="question-option">
            <input v-model="answers[question.id]" type="radio" :name="`${interaction.id}-${question.id}`" :value="option.label" />
            <span><strong>{{ option.label }}</strong><small>{{ option.description }}</small></span>
          </label>
          <label :for="`${interaction.id}-${question.id}-answer`" class="field-label">{{ t(question.options?.length ? 'interaction.otherAnswer' : 'interaction.answer') }}</label>
          <input :id="`${interaction.id}-${question.id}-answer`" v-model="answers[question.id]" :type="question.isSecret ? 'password' : 'text'" maxlength="8192" class="text-field" />
        </fieldset>
        <button class="button primary" :disabled="!canAnswer || !complete" @click="sendAnswers">{{ t('interaction.sendAnswers') }}</button>
      </div>
      <div v-else-if="pending" key="approval" class="interaction-actions">
        <button class="button primary" :disabled="!canAnswer" @click="emit('respond', interaction.id, { decision: 'accept' })">{{ t('interaction.allowOnce') }}</button>
        <button class="button" :disabled="!canAnswer" @click="emit('respond', interaction.id, { decision: 'decline' })">{{ t('interaction.decline') }}</button>
      </div>
      <p v-else-if="interaction.state === 'unknown'" class="subtle">{{ t('interaction.expired') }}</p>
      <pre v-else-if="interaction.answer">{{ answerText }}</pre>
    </Transition>
  </article>
</template>
