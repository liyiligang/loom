<script setup lang="ts">
import { computed } from 'vue'
import { Terminal, Wrench, FileDiff, Brain, ChevronRight } from '@lucide/vue'
import MarkdownIt from 'markdown-it'
import DOMPurify from 'dompurify'
import type { Entry } from '../types'

const props = defineProps<{ entry: Entry }>()
const markdown = new MarkdownIt({ html: false, linkify: true, breaks: true })
const content = computed(() => DOMPurify.sanitize(markdown.render(props.entry.text), { FORBID_TAGS: ['img', 'style'] }))
const activityType = computed(() => String(props.entry.detail?.type ?? 'activity'))
const activityTitle = computed(() => {
  const detail = props.entry.detail
  if (detail?.command) return String(detail.command)
  if (activityType.value === 'fileChange') return 'File changes'
  if (activityType.value === 'reasoning') return 'Reasoning'
  if (activityType.value === 'plan') return 'Plan'
  if (detail?.tool) return String(detail.tool)
  return 'Agent activity'
})
const icon = computed(() => activityType.value === 'commandExecution' ? Terminal : activityType.value === 'fileChange' ? FileDiff : activityType.value === 'reasoning' ? Brain : Wrench)
</script>

<template>
  <article v-if="entry.kind === 'message'" class="message" :class="entry.role">
    <div class="message-heading">
      <span class="avatar" :class="entry.role">{{ entry.role === 'user' ? 'Y' : 'C' }}</span>
      <strong>{{ entry.role === 'user' ? 'You' : 'Codex' }}</strong>
      <time :datetime="entry.createdAt">{{ new Date(entry.createdAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) }}</time>
      <span v-if="entry.status !== 'completed'" class="stream-label">Writing</span>
    </div>
    <div v-if="entry.role === 'user'" class="user-text">{{ entry.text }}</div>
    <div v-else class="markdown" v-html="content"></div>
    <p v-if="entry.truncated" class="subtle">Output reached the 1 MiB display limit.</p>
  </article>
  <details v-else class="activity">
    <summary><component :is="icon" :size="16" /><span>{{ activityTitle }}</span><small>{{ entry.detail?.status ?? entry.status }}</small><ChevronRight :size="15" class="disclosure-icon" /></summary>
    <div class="activity-body">
      <pre v-if="entry.text">{{ entry.text }}</pre>
      <pre v-if="entry.detail">{{ JSON.stringify(entry.detail, null, 2) }}</pre>
      <p v-if="!entry.text && !entry.detail" class="subtle">Waiting for execution details.</p>
      <p v-if="entry.truncated" class="subtle">Output reached the 1 MiB display limit.</p>
    </div>
  </details>
</template>
