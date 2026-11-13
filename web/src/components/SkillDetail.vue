<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { request } from '../api'
import { useI18n } from '../i18n'
import StreamingMarkdown from './StreamingMarkdown.vue'
import type { Project, ProjectSkill, ProjectSkillDetail, Session } from '../types'

const props = defineProps<{ project: Project; skill: ProjectSkill; session?: Session }>()
const { t, errorMessage } = useI18n()
const detail = ref<ProjectSkillDetail>()
const loading = ref(false)
const error = ref<unknown>()
let revision = 0
watch(() => [props.project.id, props.skill.id, props.session?.id], async () => {
  const version = ++revision
  loading.value = true
  detail.value = undefined
  error.value = undefined
  try {
    const query = props.session ? `?sessionId=${encodeURIComponent(props.session.id)}` : ''
    const value = await request<ProjectSkillDetail>(`/projects/${encodeURIComponent(props.project.id)}/skills/${encodeURIComponent(props.skill.id)}${query}`)
    if (version === revision) detail.value = value
  } catch (cause) { if (version === revision) error.value = cause }
  finally { if (version === revision) loading.value = false }
}, { immediate: true })
onBeforeUnmount(() => { ++revision })
</script>

<template>
  <section class="skill-detail">
    <p v-if="loading" class="subtle" role="status">{{ t('skills.loading') }}</p>
    <p v-else-if="error" class="form-error" role="alert">{{ errorMessage(error, 'skills.readError') }}</p>
    <template v-else-if="detail"><div class="skill-preview-meta"><p v-if="detail.description">{{ detail.description }}</p><code v-for="path in detail.paths" :key="path">{{ path }}</code></div><StreamingMarkdown :text="detail.content" status="completed" external-links-only /></template>
  </section>
</template>
