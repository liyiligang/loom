<script setup lang="ts">
import { Languages } from '@lucide/vue'
import { useI18n } from '../i18n'
import type { Locale } from '../i18n/locale'

const { locale, setLanguage, t } = useI18n()
function change(event: Event) { setLanguage((event.target as HTMLSelectElement).value as Locale) }
</script>

<template>
  <label class="language-picker" :title="t('language.label')">
    <Languages :size="16" aria-hidden="true" />
    <span class="sr-only">{{ t('language.label') }}</span>
    <select :value="locale" :aria-label="t('language.label')" @change="change">
      <option value="en" lang="en">{{ t('language.english') }}</option>
      <option value="zh-CN" lang="zh-CN">{{ t('language.chinese') }}</option>
    </select>
  </label>
</template>
