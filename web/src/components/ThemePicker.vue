<script setup lang="ts">
import { computed } from 'vue'
import { Moon, Sun } from '@lucide/vue'
import { useTheme } from '../composables/useTheme'
import { useI18n } from '../i18n'

const { theme, setTheme } = useTheme()
const { t } = useI18n()
const label = computed(() => t(theme.value === 'dark' ? 'theme.switchLight' : 'theme.switchDark'))
</script>

<template>
  <button type="button" class="icon-button theme-toggle" :aria-label="label" :title="label" @click="setTheme(theme === 'dark' ? 'light' : 'dark')">
    <component :is="theme === 'dark' ? Sun : Moon" :size="17" aria-hidden="true" />
  </button>
</template>
