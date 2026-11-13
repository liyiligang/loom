<script setup lang="ts">
import { computed } from 'vue'
import { Terminal, Wrench, FileDiff, Brain, Check, CircleAlert, LoaderCircle } from '@lucide/vue'
import AnimatedDisclosure from './AnimatedDisclosure.vue'
import StreamingMarkdown from './StreamingMarkdown.vue'
import type { Entry } from '../types'
import { useI18n } from '../i18n'

const props = withDefaults(defineProps<{ entry: Entry; agentName?: string }>(), { agentName: 'Codex' })
const { t, locale, stateLabel } = useI18n()
const activityType = computed(() => String(props.entry.detail?.type ?? 'activity'))
const activityTitle = computed(() => {
  const detail = props.entry.detail
  if (detail?.command) return String(detail.command)
  if (activityType.value === 'fileChange') return t('entry.fileChanges')
  if (activityType.value === 'reasoning') return t('entry.reasoning')
  if (activityType.value === 'plan') return t('entry.plan')
  if (detail?.tool) return String(detail.tool)
  return t('entry.activity')
})
const icon = computed(() => activityType.value === 'commandExecution' ? Terminal : activityType.value === 'fileChange' ? FileDiff : activityType.value === 'reasoning' ? Brain : Wrench)
const activityStatus = computed(() => String(props.entry.detail?.status ?? props.entry.status))
const activityState = computed(() => {
  if (['running', 'inProgress', 'streaming'].includes(activityStatus.value)) return 'running'
  if (['failed', 'error'].includes(activityStatus.value)) return 'failed'
  if (['completed', 'succeeded', 'success'].includes(activityStatus.value)) return 'completed'
  return 'waiting'
})
const writing = computed(() => props.entry.role !== 'user' && ['running', 'inProgress', 'streaming'].includes(props.entry.status))
</script>

<template>
  <article v-if="entry.kind === 'message'" class="message" :class="entry.role">
    <div class="message-heading">
      <span class="avatar" :class="entry.role">{{ entry.role === 'user' ? t('entry.youAvatar') : 'C' }}</span>
      <strong>{{ entry.role === 'user' ? t('entry.you') : agentName }}</strong>
      <time :datetime="entry.createdAt">{{ new Date(entry.createdAt).toLocaleTimeString(locale, { hour: '2-digit', minute: '2-digit' }) }}</time>
      <Transition name="fade"><span v-if="writing" class="stream-label"><span class="writing-dots" aria-hidden="true"><i></i><i></i><i></i></span>{{ t('entry.writing') }}</span></Transition>
    </div>
    <div v-if="entry.role === 'user'" class="user-text">{{ entry.text }}</div>
    <StreamingMarkdown v-else :text="entry.text" :status="entry.status" />
    <p v-if="entry.truncated" class="subtle">{{ t('entry.truncated') }}</p>
  </article>
  <AnimatedDisclosure v-else class="activity" :class="`activity-${activityState}`">
    <template #summary>
      <component :is="icon" :size="16" class="activity-type-icon" aria-hidden="true" />
      <span class="activity-title" :title="activityTitle">{{ activityTitle }}</span>
      <span class="activity-status" :class="activityState">
        <Transition name="status" mode="out-in">
          <component :is="activityState === 'running' ? LoaderCircle : activityState === 'completed' ? Check : CircleAlert" v-if="activityState !== 'waiting'" :key="activityState" :size="14" :class="{ spinning: activityState === 'running' }" aria-hidden="true" />
        </Transition>
        <small>{{ stateLabel(activityStatus) }}</small>
      </span>
    </template>
    <div class="activity-body">
      <pre v-if="entry.text">{{ entry.text }}</pre>
      <details v-if="entry.detail" class="raw-details"><summary>{{ t('entry.executionDetails') }}</summary><pre>{{ JSON.stringify(entry.detail, null, 2) }}</pre></details>
      <p v-if="!entry.text && !entry.detail" class="subtle">{{ t('entry.waitingDetails') }}</p>
      <p v-if="entry.truncated" class="subtle">{{ t('entry.truncated') }}</p>
    </div>
  </AnimatedDisclosure>
</template>
