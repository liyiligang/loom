<script setup lang="ts">
import { computed } from 'vue'
import { Monitor, RefreshCw, Cable } from '@lucide/vue'
import { useI18n } from '../i18n'
import type { LocalStatus } from '../types'
import LanguagePicker from './LanguagePicker.vue'
import ThemePicker from './ThemePicker.vue'

const props = defineProps<{ status: LocalStatus; busy: boolean; open: boolean }>()
const emit = defineEmits<{ confirm: []; rescan: []; remote: [] }>()
const { t, message } = useI18n()
const available = computed(() => props.status.agents.some(agent => agent.state === 'available'))
const missing = computed(() => props.status.agents.filter(agent => agent.state === 'missing'))
</script>

<template>
  <section v-if="open" class="local-setup-page">
    <div class="local-setup-heading"><h1>{{ t('local.setupTitle') }}</h1><div class="appearance-controls"><ThemePicker /><LanguagePicker /></div></div>
    <p class="setup-intro">{{ t('local.setupIntro') }}</p>
    <p v-if="status.state === 'scanning'" role="status">{{ t('local.scanning') }}</p>
    <div v-for="agent in status.agents" :key="agent.provider" class="local-agent-card">
      <Monitor :size="18" />
      <div><strong>{{ agent.name }}</strong><p v-if="agent.error" role="status">{{ message(agent.error) }}</p></div>
      <span class="local-agent-state" :class="agent.state">{{ t(agent.state === 'available' ? 'local.available' : agent.state === 'missing' ? 'local.notInstalled' : 'local.unavailable') }}</span>
    </div>
    <details v-if="status.state === 'ready' && !available && missing.length" class="install-help">
      <summary>{{ t('local.installHelp') }}</summary>
      <p v-for="agent in missing" :key="agent.provider"><strong>{{ agent.name }}</strong><code>{{ agent.provider === 'codex' ? 'npm install -g @openai/codex' : 'npm install -g @anthropic-ai/claude-code' }}</code></p>
    </details>
    <div class="local-setup-actions"><button class="button primary" :disabled="busy || !available || status.state === 'scanning'" @click="emit('confirm')">{{ t(busy ? 'local.enabling' : 'local.confirm') }}</button></div>
    <div class="local-setup-links">
      <button class="text-button" :disabled="busy || status.state === 'scanning'" @click="emit('rescan')"><RefreshCw :size="14" />{{ t('local.detectAgain') }}</button>
      <button class="text-button" :disabled="busy" @click="emit('remote')"><Cable :size="14" />{{ t('local.useRemote') }}</button>
    </div>
  </section>
</template>
