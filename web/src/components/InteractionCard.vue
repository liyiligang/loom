<script setup lang="ts">
import { reactive, computed } from 'vue'
import { ShieldCheck, MessageCircleQuestion } from '@lucide/vue'
import type { Interaction, Reply } from '../types'

const props = defineProps<{ interaction: Interaction; executionDetail?: Record<string, unknown>; busy: boolean; online: boolean }>()
const emit = defineEmits<{ respond: [id: string, reply: Reply] }>()
const answers = reactive<Record<string, string>>({})
const pending = computed(() => props.interaction.state === 'pending')
const canAnswer = computed(() => pending.value && !props.busy && props.online)
const complete = computed(() => (props.interaction.payload.questions ?? []).every(question => answers[question.id]?.trim()))
function sendAnswers() {
  emit('respond', props.interaction.id, { answers: Object.fromEntries((props.interaction.payload.questions ?? []).map(question => [question.id, { answers: [answers[question.id] ?? ''] }])) })
}
</script>

<template>
  <article class="interaction" :class="{ closed: !pending }">
    <div class="interaction-heading"><component :is="interaction.kind === 'question' ? MessageCircleQuestion : ShieldCheck" :size="19" /><strong>{{ interaction.kind === 'question' ? 'Codex needs your input' : 'Approval requested' }}</strong><span class="status">{{ interaction.state }}</span></div>
    <p v-if="interaction.payload.reason">{{ interaction.payload.reason }}</p>
    <pre v-if="interaction.payload.command">{{ interaction.payload.command }}</pre>
    <p v-if="interaction.payload.grantRoot">Requested write access: <code>{{ interaction.payload.grantRoot }}</code></p>
    <details v-if="executionDetail"><summary>Review execution details</summary><pre>{{ JSON.stringify(executionDetail, null, 2) }}</pre></details>
    <template v-if="interaction.kind === 'question' && pending">
      <fieldset v-for="question in interaction.payload.questions" :key="question.id" :disabled="!canAnswer">
        <legend>{{ question.question }}</legend>
        <label v-for="option in question.options ?? []" :key="option.label" class="question-option">
          <input v-model="answers[question.id]" type="radio" :name="`${interaction.id}-${question.id}`" :value="option.label" />
          <span><strong>{{ option.label }}</strong><small>{{ option.description }}</small></span>
        </label>
        <label :for="`${interaction.id}-${question.id}-answer`" class="field-label">{{ question.options?.length ? 'Or enter an answer' : 'Your answer' }}</label>
        <input :id="`${interaction.id}-${question.id}-answer`" v-model="answers[question.id]" :type="question.isSecret ? 'password' : 'text'" maxlength="8192" class="text-field" />
      </fieldset>
      <button class="button primary" :disabled="!canAnswer || !complete" @click="sendAnswers">Send answers</button>
    </template>
    <div v-else-if="pending" class="interaction-actions">
      <button class="button primary" :disabled="!canAnswer" @click="emit('respond', interaction.id, { decision: 'accept' })">Allow once</button>
      <button class="button" :disabled="!canAnswer" @click="emit('respond', interaction.id, { decision: 'decline' })">Decline</button>
    </div>
    <p v-else-if="interaction.state === 'unknown'" class="subtle">This request's connection expired. Synchronize the session to check its current state.</p>
    <pre v-else-if="interaction.answer">{{ JSON.stringify(interaction.answer, null, 2) }}</pre>
  </article>
</template>
