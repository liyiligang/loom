<script setup lang="ts">
import { computed, onUnmounted } from 'vue'
import { ElConfigProvider } from 'element-plus'
import en from 'element-plus/es/locale/lang/en'
import zhCN from 'element-plus/es/locale/lang/zh-cn'
import { synchronizeDocumentLanguage, useI18n } from './i18n'
import { synchronizeDocumentTheme } from './composables/useTheme'

const { locale } = useI18n()
const elementLocale = computed(() => locale.value === 'zh-CN' ? zhCN : en)
const stopLanguageSync = synchronizeDocumentLanguage()
onUnmounted(stopLanguageSync)
const stopThemeSync = synchronizeDocumentTheme()
onUnmounted(stopThemeSync)
</script>

<template><ElConfigProvider :locale="elementLocale"><RouterView /></ElConfigProvider></template>
